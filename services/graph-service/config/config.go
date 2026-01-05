package config

import "os"

type Config struct {
	GRPCPort  string
	Neo4jURI  string // ex: bolt://localhost:7687
	Neo4jUser string
	Neo4jPass string
}

func Load() Config {
	return Config{
		GRPCPort:  getEnv("GRPC_PORT", "50052"),
		Neo4jURI:  getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser: getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass: getEnv("NEO4J_PASSWORD", "password"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
