package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config holds application configuration loaded from environment variables.
//
// Domain-specific fields are declared in sub-files as embedded sub-configs
// with mapstructure:",squash" so that cfg.HTTPAddr (promoted from ServerConfig)
// works identically to a flat struct — no call-site changes needed.
type Config struct {
	AppEnv string `mapstructure:"APP_ENV"`

	ServerConfig   `mapstructure:",squash"`
	DatabaseConfig `mapstructure:",squash"`
	KafkaConfig    `mapstructure:",squash"`
	LLMConfig      `mapstructure:",squash"`
	ObsidianConfig `mapstructure:",squash"`
	AuthConfig     `mapstructure:",squash"`
	SMTPConfig     `mapstructure:",squash"`

	LogLevel  string `mapstructure:"LOG_LEVEL"`
	LogFormat string `mapstructure:"LOG_FORMAT"`
	LogOutput string `mapstructure:"LOG_OUTPUT"`
}

// Load reads configuration from .env file and environment variables using Viper.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	v.AutomaticEnv()

	v.SetDefault("HTTP_ADDR", ":8080")
	v.SetDefault("SWAGGER_ENABLED", true)
	v.SetDefault("DAILY_DIGEST_TIME", "08:00")
	v.SetDefault("DAILY_DIGEST_TIMEZONE", "Asia/Shanghai")
	v.SetDefault("DAILY_DIGEST_TARGET", "yesterday")
	v.SetDefault("DAILY_DIGEST_TOP_N", 20)
	v.SetDefault("LLM_PROVIDER", "openai")
	v.SetDefault("LLM_BASE_URL", "https://api.openai.com/v1")
	v.SetDefault("LLM_MODEL", "gpt-4o-mini")
	v.SetDefault("LLM_MAX_TOKENS", 4096)
	v.SetDefault("LLM_TEMPERATURE", 0.7)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "json")
	v.SetDefault("LOG_OUTPUT", "stdout")
	v.SetDefault("REDIS_ADDR", "localhost:6379")
	v.SetDefault("KAFKA_BROKERS", []string{"localhost:9092"})
	v.SetDefault("KAFKA_CONSUMER_GROUP", "hotkey-workers")
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("JWT_ISSUER", "hotkey-server")
	v.SetDefault("JWT_AUDIENCE", "hotkey-web")
	v.SetDefault("AUTH_COOKIE_SECURE", false)
	v.SetDefault("SMTP_PORT", 587)

	if err := v.ReadInConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to read .env config file: %v\n", err)
	}

	_ = v.BindEnv("APP_ENV")
	_ = v.BindEnv("DATABASE_URL")
	_ = v.BindEnv("JWT_SECRET")
	_ = v.BindEnv("JWT_ISSUER")
	_ = v.BindEnv("JWT_AUDIENCE")
	_ = v.BindEnv("AUTH_VERIFICATION_PEPPER")
	_ = v.BindEnv("WEB_ALLOWED_ORIGINS")
	_ = v.BindEnv("AUTH_COOKIE_SECURE")
	_ = v.BindEnv("AUTH_COOKIE_DOMAIN")
	_ = v.BindEnv("SMTP_HOST")
	_ = v.BindEnv("SMTP_PORT")
	_ = v.BindEnv("SMTP_USERNAME")
	_ = v.BindEnv("SMTP_AUTH_CODE")
	_ = v.BindEnv("SMTP_FROM_EMAIL")
	_ = v.BindEnv("SMTP_FROM_NAME")
	_ = v.BindEnv("HTTP_ADDR")
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
	_ = v.BindEnv("LLM_MAX_TOKENS")
	_ = v.BindEnv("LLM_TEMPERATURE")
	_ = v.BindEnv("LOG_LEVEL")
	_ = v.BindEnv("LOG_FORMAT")
	_ = v.BindEnv("LOG_OUTPUT")
	_ = v.BindEnv("KAFKA_BROKERS")
	_ = v.BindEnv("KAFKA_CONSUMER_GROUP")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if cfg.AppEnv == "production" && len(cfg.JWTSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET must be at least 32 characters in production")
	}
	if cfg.XToken == "" {
		return Config{}, errors.New("X_BEARER_TOKEN is required")
	}
	if cfg.AppEnv == "production" && cfg.VerificationPepper == "" {
		return Config{}, errors.New("AUTH_VERIFICATION_PEPPER is required in production environment")
	}
	if cfg.SMTPHost != "" && cfg.SMTPAuthCode == "" {
		return Config{}, errors.New("SMTP_AUTH_CODE is required when SMTP_HOST is set")
	}
	for _, origin := range cfg.WebAllowedOrigins {
		if origin == "*" && !cfg.CookieSecure {
			return Config{}, errors.New("WEB_ALLOWED_ORIGINS cannot include wildcard origin when AUTH_COOKIE_SECURE is false")
		}
	}

	if cfg.XBaseURL == "" {
		cfg.XBaseURL = "https://api.x.com"
	}

	if cfg.DailyDigestTime == "" {
		cfg.DailyDigestTime = "08:00"
	}
	if cfg.DailyDigestTimezone == "" {
		cfg.DailyDigestTimezone = "Asia/Shanghai"
	}
	if cfg.DailyDigestTarget == "" {
		cfg.DailyDigestTarget = "yesterday"
	}
	if cfg.DailyDigestTopN == 0 {
		cfg.DailyDigestTopN = 20
	}
	if cfg.LLMBaseURL == "" {
		cfg.LLMBaseURL = "https://api.openai.com/v1"
	}
	if cfg.LLMModel == "" {
		cfg.LLMModel = "gpt-4o-mini"
	}
	if cfg.LLMMaxTokens <= 0 {
		cfg.LLMMaxTokens = 4096
	}
	if cfg.LLMTemperature <= 0 {
		cfg.LLMTemperature = 0.7
	}

	return cfg, nil
}
