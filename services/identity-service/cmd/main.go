package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	// OpenTelemetry
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	// PostgreSQL Driver
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"

	// Interne
	"github.com/jupiterclapton/cenackle/services/identity-service/config"
	grpc_adapter "github.com/jupiterclapton/cenackle/services/identity-service/internal/adapters/primary/grpc"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/adapters/secondary/eventbroker"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/adapters/secondary/repository"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/adapters/secondary/security"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/services"
)

func main() {
	// 1. Charger la Config
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	// 2. Initialiser le Logger (slog JSON pour la prod, Text pour le dev)
	initLogger(cfg)
	slog.Info("🚀 Starting Identity Service", "env", cfg.Env, "port", cfg.GRPCPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Initialiser le Tracing (OpenTelemetry)
	tp, err := initTracer(ctx, cfg)
	if err != nil {
		slog.Error("Failed to init tracer", "error", err)
	} else {
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				slog.Error("Error shutting down tracer", "error", err)
			}
		}()
	}

	// 4. Infrastructure : Base de données (Postgres)
	// 1. On parse la config d'abord
	dbConfig, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		slog.Error("Unable to parse DB config", "error", err)
		os.Exit(1)
	}

	// 2. MAGIE ICI : On injecte le tracer OpenTelemetry
	dbConfig.ConnConfig.Tracer = otelpgx.NewTracer()

	// 3. On crée le pool avec cette config enrichie
	dbPool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		slog.Error("Unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// Vérification connectivité immédiate (Fail Fast)
	if err := dbPool.Ping(ctx); err != nil {
		slog.Error("Database ping failed", "error", err)
		os.Exit(1)
	}
	slog.Info("✅ Database connected")

	// 5. Infrastructure : Event Broker (Nats JetStream)
	broker, err := eventbroker.NewNatsBroker(cfg.NatsUrl)
	if err != nil {
		// En prod, on pourrait décider de retry ou panic selon la stratégie
		slog.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	slog.Info("✅ NATS JetStream connected")

	// 6. Infrastructure : Sécurité (Clés RSA & Argon2)
	privKey, pubKey, err := loadKeys(cfg.RSAPrivateKeyPath, cfg.RSAPublicKeyPath)
	if err != nil {
		slog.Error("Failed to load RSA keys", "error", err)
		os.Exit(1)
	}

	jwtProvider, err := security.NewJWTProvider(privKey, pubKey)
	if err != nil {
		slog.Error("Failed to init JWT provider", "error", err)
		os.Exit(1)
	}

	hasher := security.NewArgon2Hasher(nil) // Params par défaut

	// 7. Wiring (Injection de dépendances) - Adapters -> Service
	repo := repository.NewPostgresRepo(dbPool)

	// Orchestration du cœur
	identityService := services.NewIdentityService(repo, hasher, jwtProvider, broker)

	// Adapter Primaire (gRPC Handler)
	grpcHandler := grpc_adapter.NewAuthGrpcServer(identityService)

	// 8. Configuration du Serveur gRPC
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("Failed to listen", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	// Options gRPC : Interceptors pour le Tracing et Logging
	opts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()), // Auto-tracing des requêtes
		// On pourrait ajouter ici un interceptor de logging custom ou un RecoveryInterceptor
	}

	grpcServer := grpc.NewServer(opts...)

	// Enregistrement des services
	// Note: grpcHandler.Listen() dans votre code précédent faisait tout ça,
	// mais ici on le décompose pour avoir accès au 'grpcServer' pour le HealthCheck et Reflection.
	// Il faudrait adapter grpcHandler pour exposer juste une méthode Register(s *grpc.Server)
	// Pour l'instant, supposons que grpcHandler implémente l'interface protobuf :
	// identityv1.RegisterIdentityServiceServer(grpcServer, grpcHandler) <- Voir note en bas

	// Astuce: Modifiez légèrement votre server.go pour exposer une méthode d'enregistrement explicite
	// ou faites-le ici si 'grpcHandler' est exporté.
	// Supposons que vous ayez accès à l'interface générée :
	// identityv1.RegisterIdentityServiceServer(grpcServer, grpcHandler)
	// (Voir note explicative à la fin)

	// Appelons la méthode Listen modifiée ou enregistrons manuellement :
	grpcHandler.RegisterTo(grpcServer) // <= J'explique cette modif en bas

	// Health Check (Standard K8s)
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(cfg.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// Reflection (Pour tester avec Postman/GRPCCurl en dev)
	if cfg.Env != "prod" {
		reflection.Register(grpcServer)
		slog.Info("🔍 gRPC Reflection enabled")
	}

	// 9. Démarrage du serveur (Goroutine)
	go func() {
		slog.Info("🚀 gRPC Server listening", "address", lis.Addr())
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("Failed to serve", "error", err)
			os.Exit(1) // Fatal
		}
	}()

	// 10. Graceful Shutdown (Attente des signaux OS)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	sig := <-quit // Bloquant
	slog.Info("⚠️  Signal received, shutting down...", "signal", sig)

	// Création d'un timeout pour l'arrêt
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Arrêt de gRPC (finit les requêtes en cours)
	// Note: GracefulStop ne prend pas de contexte, mais on peut forcer Stop() après timeout
	done := make(chan struct{})
	go func() {
		healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("✅ gRPC Server stopped gracefully")
	case <-shutdownCtx.Done():
		slog.Warn("⏳ Timeout reached, forcing server stop")
		grpcServer.Stop()
	}

	slog.Info("👋 Service stopped")
}

// --- HELPERS ---

func initLogger(cfg *config.Config) {
	var handler slog.Handler
	if cfg.Env == "local" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	slog.SetDefault(slog.New(handler))
}

func initTracer(ctx context.Context, cfg *config.Config) (*sdktrace.TracerProvider, error) {
	// Création de l'exporteur OTLP (gRPC)
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OtelEndpoint),
		otlptracegrpc.WithInsecure(), // En prod, gérez le TLS
	)
	if err != nil {
		return nil, err
	}

	// Ressource (Nom du service, version...)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String("1.0.0"),
			semconv.DeploymentEnvironmentKey.String(cfg.Env),
		),
	)
	if err != nil {
		return nil, err
	}

	// Provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// Set global propagator (Important pour propager le trace-id entre microservices)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}

func loadKeys(privPath, pubPath string) ([]byte, []byte, error) {
	priv, err := os.ReadFile(privPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading private key: %w", err)
	}
	pub, err := os.ReadFile(pubPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading public key: %w", err)
	}
	return priv, pub, nil
}
