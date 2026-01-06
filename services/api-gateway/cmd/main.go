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

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ravilushqa/otelgqlgen"
	"github.com/rs/cors"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	feedv1 "github.com/jupiterclapton/cenackle/gen/feed/v1"
	identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"
	postv1 "github.com/jupiterclapton/cenackle/gen/post/v1"
	"github.com/jupiterclapton/cenackle/services/api-gateway/config"
	"github.com/jupiterclapton/cenackle/services/api-gateway/graph"
	"github.com/jupiterclapton/cenackle/services/api-gateway/internal/auth"
)

func main() {
	// 1. Configuration
	cfg := config.Load()

	// 2. Logger
	initLogger(cfg)
	slog.Info("🚀 Starting API Gateway", "config", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Télémétrie (Tracing)
	tp, err := initTracer(ctx, cfg)
	if err != nil {
		slog.Error("Failed to init tracer", "error", err)
	} else {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	// 4. Clients gRPC (Identity, Post, Feed)
	// On utilise une fonction helper pour éviter de dupliquer le code de connexion
	identityConn := mustConnectGrpc(cfg.IdentityURL, "Identity Service")
	defer identityConn.Close()
	identityClient := identityv1.NewIdentityServiceClient(identityConn)

	postConn := mustConnectGrpc(cfg.PostURL, "Post Service")
	defer postConn.Close()
	postClient := postv1.NewPostServiceClient(postConn)

	feedConn := mustConnectGrpc(cfg.FeedURL, "Feed Service")
	defer feedConn.Close()
	feedClient := feedv1.NewFeedServiceClient(feedConn)

	// 5. Création du Serveur GraphQL
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{
			IdentityClient: identityClient,
			PostClient:     postClient,
			FeedClient:     feedClient,
		},
	}))

	// Instrumentation GraphQL (Expert)
	srv.Use(otelgqlgen.Middleware())

	// 6. Chaîne de Middlewares HTTP
	var h http.Handler = srv

	// A. Auth (Injecte UserID)
	h = auth.Middleware(identityClient)(h)

	// B. CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:19006"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "baggage", "sentry-trace"},
		AllowCredentials: true,
	})
	h = c.Handler(h)

	// C. OTEL HTTP (Racine)
	h = otelhttp.NewHandler(h, "GraphQL-Gateway", otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
		return fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
	}))

	// 7. Routage
	mux := http.NewServeMux()
	mux.Handle("/query", h)

	if cfg.Env != "prod" {
		mux.Handle("/", playground.Handler("GraphQL playground", "/query"))
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	// 8. Démarrage Graceful
	srvHTTP := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	go func() {
		slog.Info("📡 Gateway listening", "port", cfg.Port)
		if err := srvHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

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

// --- HELPERS ---

// Helper pour initier les connexions gRPC avec Tracing activé
func mustConnectGrpc(url string, serviceName string) *grpc.ClientConn {
	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()), // Injection Trace Context
	)
	if err != nil {
		slog.Error("Failed to connect to microservice", "service", serviceName, "url", url, "error", err)
		os.Exit(1)
	}
	return conn
}

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
			semconv.ServiceNameKey.String("api-gateway"),
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
