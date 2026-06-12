package config

import (
	"errors"
	"os"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
	RedisAddr   string
}

// Load reads configuration from environment variables and validates required fields.
func Load() (Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	return Config{
		HTTPAddr:    httpAddr,
		DatabaseURL: dbURL,
		RedisAddr:   os.Getenv("REDIS_ADDR"),
	}, nil
}
