package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Env         string // "local", "dev", "prod"
	ServiceName string
	GRPCPort    string

	// Infrastructure
	DBUrl   string // Connection string Postgres
	NatsUrl string

	// Sécurité
	RSAPrivateKeyPath string
	RSAPublicKeyPath  string

	// Telemetry
	OtelEndpoint string // URL du collecteur (Jaeger/Tempo)
}

// Load charge la configuration depuis l'ENV ou utilise des défauts
func Load() (*Config, error) {
	cfg := &Config{
		Env:               getEnv("APP_ENV", "local"),
		ServiceName:       getEnv("SERVICE_NAME", "identity-service"),
		GRPCPort:          getEnv("GRPC_PORT", "50051"),
		DBUrl:             getEnv("DB_URL", "postgres://user:password@localhost:5432/identity_db?sslmode=disable"),
		NatsUrl:           getEnv("NATS_URL", "nats://localhost:4222"),
		RSAPrivateKeyPath: getEnv("RSA_PRIVATE_KEY_PATH", "./keys/private.pem"),
		RSAPublicKeyPath:  getEnv("RSA_PUBLIC_KEY_PATH", "./keys/public.pem"),
		OtelEndpoint:      getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
	}

	// Validation basique pour éviter de démarrer avec une config cassée
	if cfg.Env == "prod" && cfg.DBUrl == "" {
		return nil, fmt.Errorf("DB_URL is required in production")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}
