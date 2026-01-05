package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/jupiterclapton/cenackle/services/graph-service/config"
	grpc_adapter "github.com/jupiterclapton/cenackle/services/graph-service/internal/adapters/primary/grpc"
	"github.com/jupiterclapton/cenackle/services/graph-service/internal/adapters/secondary/repository"
	"github.com/jupiterclapton/cenackle/services/graph-service/internal/core/services"
)

func main() {
	cfg := config.Load()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// 1. Connexion Neo4j
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		slog.Error("Failed to create neo4j driver", "error", err)
		os.Exit(1)
	}
	defer driver.Close(context.Background())

	// Vérification connectivité
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		slog.Error("Failed to connect to Neo4j", "error", err)
		os.Exit(1)
	}
	slog.Info("✅ Connected to Neo4j")

	// 2. Wiring
	repo := repository.NewNeo4jRepo(driver)

	// Init Schema (Indexes)
	if err := repo.EnsureSchema(context.Background()); err != nil {
		slog.Warn("Schema init failed (might be fine if already exists)", "error", err)
	}

	svc := services.NewGraphService(repo)
	handler := grpc_adapter.NewServer(svc)

	// 3. gRPC Server
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		panic(err)
	}

	grpcServer := grpc.NewServer()
	handler.Register(grpcServer)

	// Health Check
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	slog.Info("🚀 Graph Service (Neo4j) listening", "port", cfg.GRPCPort)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			panic(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	grpcServer.GracefulStop()
}
