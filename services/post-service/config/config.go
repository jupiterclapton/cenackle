package config

import (
	"os"
	"strings"
)

type Config struct {
	GRPCPort     string
	DBUrl        string
	NatsUrl      string
	OtelEndpoint string
	Env          string // "local" or "prod"
}

func Load() Config {
	return Config{
		GRPCPort:     getEnv("GRPC_PORT", "50053"), // Identité=50051, Graph=50052, Post=50053
		DBUrl:        getEnv("DB_URL", "postgres://user:password@localhost:5432/post_db?sslmode=disable"),
		NatsUrl:      getEnv("NATS_URL", "nats://localhost:4222"),
		OtelEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		Env:          getEnv("APP_ENV", "local"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
