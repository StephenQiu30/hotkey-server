package config

import (
	"os"
	"time"
)

type Config struct {
	HTTPAddr        string
	AuthTokenSecret string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:        envOrDefault("HOTKEY_HTTP_ADDR", ":8080"),
		AuthTokenSecret: os.Getenv("HOTKEY_AUTH_TOKEN_SECRET"),
		AccessTokenTTL:  durationOrDefault("HOTKEY_AUTH_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: durationOrDefault("HOTKEY_AUTH_REFRESH_TOKEN_TTL", 30*24*time.Hour),
	}
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}
