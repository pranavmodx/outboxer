package config

import (
	"os"
	"time"
)

// Config holds all application configuration, sourced from environment variables.
type Config struct {
	DatabaseURL  string
	BrokerType   string        // "kafka" or "redis"
	KafkaBrokers string        // comma-separated, e.g. "localhost:9092"
	RedisAddr    string        // e.g. "localhost:6379"
	PollInterval time.Duration // how often the relay polls for new outbox events
	HTTPPort     string        // port for the order service HTTP server
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		DatabaseURL:  envOrDefault("DATABASE_URL", "postgres://outbox:outbox@localhost:5432/outbox?sslmode=disable"),
		BrokerType:   envOrDefault("BROKER_TYPE", "kafka"),
		KafkaBrokers: envOrDefault("KAFKA_BROKERS", "127.0.0.1:9092"),
		RedisAddr:    envOrDefault("REDIS_ADDR", "localhost:6379"),
		PollInterval: parseDuration(envOrDefault("POLL_INTERVAL", "1s")),
		HTTPPort:     envOrDefault("HTTP_PORT", "8080"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Second
	}
	return d
}
