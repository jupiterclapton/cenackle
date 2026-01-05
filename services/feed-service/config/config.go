package config

import (
	"os"
	"strings"
)

type Config struct {
	RedisAddr    string
	NatsUrl      string
	GraphUrl     string
	OtelEndpoint string
	Env          string // "local" ou "prod"
}

func Load() Config {
	return Config{
		RedisAddr:    getEnv("REDIS_ADDR", "redis:6379"),
		NatsUrl:      getEnv("NATS_URL", "nats://nats:4222"),
		GraphUrl:     getEnv("GRAPH_SERVICE_URL", "graph-service:50052"),
		OtelEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "jaeger:4317"),
		Env:          getEnv("APP_ENV", "local"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
