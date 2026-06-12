package config

import (
	"errors"
	"os"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	RedisAddr   string
	JWTSecret   string
}

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
		JWTSecret:   os.Getenv("JWT_SECRET"),
	}, nil
}
