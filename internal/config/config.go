package config

import (
	"errors"

	"github.com/spf13/viper"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	HTTPAddr    string `mapstructure:"HTTP_ADDR"`
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	RedisAddr   string `mapstructure:"REDIS_ADDR"`
	JWTSecret   string `mapstructure:"JWT_SECRET"`
	XToken      string `mapstructure:"X_BEARER_TOKEN"`
	XBaseURL    string `mapstructure:"X_BASE_URL"`
}

// Load reads configuration from .env file and environment variables using Viper.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault("HTTP_ADDR", ":8080")

	// Try to read .env file (ignore if not found)
	_ = v.ReadInConfig()

	// Bind environment variables explicitly
	_ = v.BindEnv("DATABASE_URL")
	_ = v.BindEnv("JWT_SECRET")
	_ = v.BindEnv("HTTP_ADDR")
	_ = v.BindEnv("REDIS_ADDR")
	_ = v.BindEnv("X_BEARER_TOKEN")
	_ = v.BindEnv("X_BASE_URL")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}

	// Validate required fields
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	// X config defaults
	if cfg.XBaseURL == "" {
		cfg.XBaseURL = "https://api.x.com"
	}

	return cfg, nil
}
