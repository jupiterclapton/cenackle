package config

import (
	"os"
	"strings"
)

type Config struct {
	Port         string
	IdentityURL  string
	PostURL      string // Ajouté pour l'agrégation
	FeedURL      string // Ajouté pour l'agrégation
	OtelEndpoint string
	Env          string // "local" ou "prod"
}

func Load() Config {
	return Config{
		Port:         getEnv("PORT", "8080"),
		IdentityURL:  getEnv("IDENTITY_SERVICE_URL", "localhost:50051"),
		PostURL:      getEnv("POST_SERVICE_URL", "localhost:50053"), // Port par défaut du Post Service
		FeedURL:      getEnv("FEED_SERVICE_URL", "localhost:50054"), // Port par défaut du Feed Service
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
