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
	JWTSecret   string
	XToken      string
	XBaseURL    string
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

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	xBaseURL := os.Getenv("X_BASE_URL")
	if xBaseURL == "" {
		xBaseURL = "https://api.x.com"
	}

	return Config{
		HTTPAddr:    httpAddr,
		DatabaseURL: dbURL,
		RedisAddr:   os.Getenv("REDIS_ADDR"),
		JWTSecret:   jwtSecret,
		XToken:      os.Getenv("X_BEARER_TOKEN"),
		XBaseURL:    xBaseURL,
	}, nil
}
