package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	// Drivers
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	// Instrumentation
	"github.com/exaring/otelpgx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	// Interne
	"github.com/jupiterclapton/cenackle/services/post-service/config"
	grpc_adapter "github.com/jupiterclapton/cenackle/services/post-service/internal/adapters/primary/grpc"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/adapters/secondary/eventbroker"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/adapters/secondary/repository"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/services"
)

func main() {
	// 1. Config & Logger
	cfg := config.Load()
	initLogger(cfg)
	slog.Info("🚀 Starting Post Service", "config", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Télémétrie (Tracing)
	tp, err := initTracer(ctx, cfg)
	if err != nil {
		slog.Error("Failed to init tracer", "error", err)
	} else {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	// 3. Infrastructure: Base de données (Postgres)
	dbConfig, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		slog.Error("Unable to parse DB config", "error", err)
		os.Exit(1)
	}
	// Instrumentation SQL (Pour voir les requêtes dans Jaeger)
	dbConfig.ConnConfig.Tracer = otelpgx.NewTracer()

	dbPool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		slog.Error("Unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	slog.Info("✅ Connected to Postgres")

	// 4. Infrastructure: Event Broker (NATS)
	nc, err := nats.Connect(cfg.NatsUrl)
	if err != nil {
		slog.Error("Unable to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	slog.Info("✅ Connected to NATS")

	// 5. Initialisation des Adapters (Driven)
	postRepo := repository.NewPostgresRepo(dbPool)
	eventPub := eventbroker.NewNatsPublisher(nc)

	// 6. Initialisation du Core (Domain Logic)
	postService := services.NewPostService(postRepo, eventPub)

	// 7. Initialisation du Primary Adapter (gRPC)
	// Ajout de l'intercepteur OTEL pour propager le contexte de trace
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)

	serverAdapter := grpc_adapter.NewServer(postService)
	serverAdapter.Register(grpcServer)

	// Health Check standard pour K8s/Docker
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Active la reflection pour grpcurl / Postman
	reflection.Register(grpcServer)

	// 8. Démarrage
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}

	slog.Info("📡 Post Service listening", "port", cfg.GRPCPort)

	// Graceful Shutdown
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1) // Fatal en prod
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("🛑 Shutting down server...")

	grpcServer.GracefulStop()
	slog.Info("👋 Server exited")
}

// --- Helpers (À déplacer un jour dans pkg/telemetry et pkg/logger) ---

func initLogger(cfg config.Config) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if cfg.Env == "local" {
		opts.Level = slog.LevelDebug
	}
	var handler slog.Handler
	if cfg.Env == "local" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func initTracer(ctx context.Context, cfg config.Config) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OtelEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("post-service"),
			semconv.DeploymentEnvironmentKey.String(cfg.Env),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}
