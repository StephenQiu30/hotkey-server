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

	// Obsidian / daily digest
	ObsidianVaultPath  string `mapstructure:"OBSIDIAN_VAULT_PATH"`
	DailyDigestTime    string `mapstructure:"DAILY_DIGEST_TIME"`
	DailyDigestTimezone string `mapstructure:"DAILY_DIGEST_TIMEZONE"`
	DailyDigestTarget  string `mapstructure:"DAILY_DIGEST_TARGET"`
	DailyDigestTopN    int    `mapstructure:"DAILY_DIGEST_TOP_N"`

	// LLM
	LLMProvider string `mapstructure:"LLM_PROVIDER"`
	LLMAPIKey   string `mapstructure:"LLM_API_KEY"`
	LLMBaseURL  string `mapstructure:"LLM_BASE_URL"`
	LLMModel    string `mapstructure:"LLM_MODEL"`
}

// Load reads configuration from .env file and environment variables using Viper.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	v.AutomaticEnv()

	// Set defaults
	v.SetDefault("HTTP_ADDR", ":8080")
	v.SetDefault("DAILY_DIGEST_TIME", "08:00")
	v.SetDefault("DAILY_DIGEST_TIMEZONE", "Asia/Shanghai")
	v.SetDefault("DAILY_DIGEST_TARGET", "yesterday")
	v.SetDefault("DAILY_DIGEST_TOP_N", 20)
	v.SetDefault("LLM_PROVIDER", "openai")
	v.SetDefault("LLM_BASE_URL", "https://api.openai.com/v1")
	v.SetDefault("LLM_MODEL", "gpt-4o-mini")

	// Try to read .env file (ignore if not found)
	_ = v.ReadInConfig()

	// Bind environment variables explicitly
	_ = v.BindEnv("DATABASE_URL")
	_ = v.BindEnv("JWT_SECRET")
	_ = v.BindEnv("HTTP_ADDR")
	_ = v.BindEnv("REDIS_ADDR")
	_ = v.BindEnv("X_BEARER_TOKEN")
	_ = v.BindEnv("X_BASE_URL")
	_ = v.BindEnv("OBSIDIAN_VAULT_PATH")
	_ = v.BindEnv("DAILY_DIGEST_TIME")
	_ = v.BindEnv("DAILY_DIGEST_TIMEZONE")
	_ = v.BindEnv("DAILY_DIGEST_TARGET")
	_ = v.BindEnv("DAILY_DIGEST_TOP_N")
	_ = v.BindEnv("LLM_PROVIDER")
	_ = v.BindEnv("LLM_API_KEY")
	_ = v.BindEnv("LLM_BASE_URL")
	_ = v.BindEnv("LLM_MODEL")

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
