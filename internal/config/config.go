package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	HTTPAddr    string `mapstructure:"HTTP_ADDR"`
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	RedisAddr   string `mapstructure:"REDIS_ADDR"`
	JWTSecret   string `mapstructure:"JWT_SECRET"`
}

// Load reads configuration from .env / environment variables via Viper
// and validates required fields.
func Load() (Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("HTTP_ADDR", ":8080")

	// Bind environment variables explicitly
	_ = v.BindEnv("HTTP_ADDR")
	_ = v.BindEnv("DATABASE_URL")
	_ = v.BindEnv("REDIS_ADDR")
	_ = v.BindEnv("JWT_SECRET")

	// Try .env file (non-fatal if missing)
	v.SetConfigFile(".env")
	_ = v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}
