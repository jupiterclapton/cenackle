package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	// Imports OpenTelemetry (Indispensables pour que initTracer compile)
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/jupiterclapton/cenackle/services/feed-service/config"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/adapters/primary/events"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/adapters/secondary/clients"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/adapters/secondary/repository"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/services"
)

func main() {
	// 1. Chargement de la Config
	cfg := config.Load()

	// 2. Logger
	initLogger(cfg)
	slog.Info("🚀 Starting Feed Service", "config", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Télémétrie (Tracing)
	tp, err := initTracer(ctx, cfg)
	if err != nil {
		slog.Error("Failed to init tracer", "error", err)
	} else {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	// 4. Redis (Repository)
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})

	// Instrumentation Redis (Pour voir les écritures dans Jaeger)
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		panic(err)
	}

	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		panic(err)
	}
	slog.Info("✅ Connected to Redis")

	feedRepo := repository.NewRedisFeedRepo(rdb)

	// 5. Graph Client (gRPC)
	graphClient, err := clients.NewGraphClient(cfg.GraphUrl)
	if err != nil {
		slog.Error("Failed to connect to Graph Service", "error", err)
		panic(err)
	}
	defer graphClient.Close()
	slog.Info("✅ Connected to Graph Service")

	// 6. Core Service
	feedService := services.NewFeedService(feedRepo, graphClient)

	// 7. NATS Consumer (Primary Adapter)
	nc, err := nats.Connect(cfg.NatsUrl)
	if err != nil {
		slog.Error("Failed to connect to NATS", "error", err)
		panic(err)
	}
	defer nc.Close()
	slog.Info("✅ Connected to NATS")

	// Abonnement à l'événement
	handler := events.NewEventHandler(feedService)
	_, err = nc.Subscribe("post.created", handler.HandlePostCreated)
	if err != nil {
		panic(err)
	}

	slog.Info("👂 Feed Service listening for events...")

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("👋 Feed Service shutting down")
}

// --- Helpers ---

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
			// ⚠️ CORRECTION ICI : "feed-service" et non "post-service"
			semconv.ServiceNameKey.String("feed-service"),
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
