package config

import "os"

const (
	defaultHTTPAddr    = ":8080"
	defaultDatabaseURL = "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable"
	defaultRedisURL    = "redis://localhost:6379/0"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	RedisURL        string
	InternalAPIKey  string
	DefaultTenantID string
}

func Load() Config {
	return Config{
		HTTPAddr:        envOrDefault("HOTKEY_HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:     envOrDefault("HOTKEY_DATABASE_URL", defaultDatabaseURL),
		RedisURL:        envOrDefault("HOTKEY_REDIS_URL", defaultRedisURL),
		InternalAPIKey:  os.Getenv("HOTKEY_INTERNAL_API_KEY"),
		DefaultTenantID: envOrDefault("HOTKEY_DEFAULT_TENANT_ID", ""),
	}
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
