package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// GraphQL
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ravilushqa/otelgqlgen" // Tracing spécifique GraphQL

	// HTTP & CORS
	"github.com/rs/cors"

	// gRPC
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// OpenTelemetry
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	// Interne
	identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"
	"github.com/jupiterclapton/cenackle/services/api-gateway/graph"
	"github.com/jupiterclapton/cenackle/services/api-gateway/internal/auth"
)

// Config simple (idéalement dans un package config/)
type Config struct {
	Port         string
	IdentityURL  string
	OtelEndpoint string
	Env          string
}

func main() {
	// 1. Configuration
	cfg := loadConfig()

	// Logger JSON structuré
	initLogger(cfg)
	slog.Info("🚀 Starting API Gateway", "config", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialisation Télémétrie (Tracing)
	// C'est vital de le faire avant tout le reste
	tp, err := initTracer(ctx, cfg)
	if err != nil {
		slog.Error("Failed to init tracer", "error", err)
	} else {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	// 3. Connexion aux Microservices (avec Tracing gRPC)
	// L'intercepteur otelgrpc va injecter le TraceID dans les headers gRPC automatiquement
	conn, err := grpc.NewClient(
		cfg.IdentityURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()), // <--- MAGIE ICI
	)
	if err != nil {
		slog.Error("Failed to connect to identity service", "url", cfg.IdentityURL, "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	identityClient := identityv1.NewIdentityServiceClient(conn)

	// 4. Création du Serveur GraphQL
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{
			IdentityClient: identityClient,
		},
	}))

	// INSTRUMENTATION GRAPHQL (Expert)
	// Trace chaque champ résolu (ex: query { me { username } })
	srv.Use(otelgqlgen.Middleware())

	// 5. Construction de la chaîne de Middlewares HTTP (L'oignon)
	// L'ordre d'exécution est : OTEL -> CORS -> AUTH -> GRAPHQL

	// A. Handler GraphQL de base
	var h http.Handler = srv

	// B. Middleware Auth (Injecte UserID)
	h = auth.Middleware(identityClient)(h)

	// C. Middleware CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:19006"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "baggage", "sentry-trace"}, // baggage est utile pour OTEL
		AllowCredentials: true,
	})
	h = c.Handler(h)

	// D. Middleware OTEL HTTP (Le plus à l'extérieur)
	// Il crée le Span racine "HTTP POST /query"
	h = otelhttp.NewHandler(h, "GraphQL-Gateway", otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
		return fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
	}))

	// 6. Routage
	mux := http.NewServeMux()
	mux.Handle("/query", h)
	// Le playground ne doit pas être tracé ou sécurisé, c'est du dev tools
	if cfg.Env != "prod" {
		mux.Handle("/", playground.Handler("GraphQL playground", "/query"))
	}
	// Health check pour K8s (sans middleware lourd)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	// 7. Démarrage Graceful
	srvHTTP := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	go func() {
		slog.Info("📡 Server listening", "url", "http://localhost:"+cfg.Port)
		if err := srvHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	// Attente signal d'arrêt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("🛑 Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srvHTTP.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	slog.Info("👋 Server exited")
}

// --- HELPERS (Similaires à Identity-Service) ---

func loadConfig() Config {
	return Config{
		Port:         getEnv("PORT", "8080"),
		IdentityURL:  getEnv("IDENTITY_SERVICE_URL", "localhost:50051"),
		OtelEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Env:          getEnv("APP_ENV", "local"),
	}
}

func initLogger(cfg Config) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if cfg.Env == "local" {
		opts.Level = slog.LevelDebug
	}
	// En prod -> JSON, En local -> Texte
	var handler slog.Handler
	if cfg.Env == "local" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func initTracer(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OtelEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("api-gateway"),
			semconv.DeploymentEnvironmentKey.String(cfg.Env),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	// CRUCIAL : Permet de propager le contexte (TraceID) via les headers HTTP et gRPC
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
