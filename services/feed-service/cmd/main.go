package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	// Drivers
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	// Instrumentation
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	// Interne
	"github.com/jupiterclapton/cenackle/services/feed-service/config"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/adapters/primary/events"
	grpc_adapter "github.com/jupiterclapton/cenackle/services/feed-service/internal/adapters/primary/grpc"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/adapters/secondary/clients"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/adapters/secondary/repository"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/services"
)

func main() {
	// 1. Config & Logger
	cfg := config.Load()
	initLogger(cfg)
	slog.Info("🚀 Starting Feed Service", "config", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Télémétrie (Tracing)
	tp, err := initTracer(ctx, cfg)
	if err != nil {
		slog.Error("Failed to init tracer", "error", err)
	} else {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	// 3. Infrastructure: Redis (Driven Adapter)
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	// Instrumentation Redis
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		panic(err)
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("Unable to connect to Redis", "error", err)
		os.Exit(1)
	}
	slog.Info("✅ Connected to Redis")

	feedRepo := repository.NewRedisFeedRepo(rdb)

	// 4. Infrastructure: Graph Client (Driven Adapter)
	graphClient, err := clients.NewGraphClient(cfg.GraphUrl)
	if err != nil {
		slog.Error("Unable to connect to Graph Service", "error", err)
		os.Exit(1)
	}
	defer graphClient.Close()
	slog.Info("✅ Connected to Graph Service")

	// 5. Infrastructure: Event Broker NATS (Driven Adapter)
	nc, err := nats.Connect(cfg.NatsUrl)
	if err != nil {
		slog.Error("Unable to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	slog.Info("✅ Connected to NATS")

	// 6. Initialisation du Core
	feedService := services.NewFeedService(feedRepo, graphClient)

	// 7. Initialisation du Consumer NATS (Driving Adapter - Async)
	handler := events.NewEventHandler(feedService)
	_, err = nc.Subscribe("post.created", handler.HandlePostCreated)
	if err != nil {
		slog.Error("Failed to subscribe to NATS", "error", err)
		os.Exit(1)
	}
	slog.Info("👂 Listening for events (NATS)")

	// 8. Initialisation du Serveur gRPC (Driving Adapter - Sync)
	// C'est ici qu'on permet la LECTURE du feed
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)

	serverAdapter := grpc_adapter.NewServer(feedService)
	serverAdapter.Register(grpcServer)

	// Health Check & Reflection
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	reflection.Register(grpcServer)

	slog.Info("📡 Feed Service gRPC listening", "port", cfg.GRPCPort)

	// On lance le serveur gRPC dans une goroutine pour ne pas bloquer
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("🛑 Shutting down server...")

	grpcServer.GracefulStop()
	// On pourrait fermer rdb et nc proprement ici aussi
	slog.Info("👋 Server exited")
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
			semconv.ServiceNameKey.String("feed-service"), // ⚠️ Important: bien nommer le service
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
