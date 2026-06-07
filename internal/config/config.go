package config

import (
	"os"
	"strconv"
	"time"
)

type RuntimeMode string

const (
	RuntimeModeAll    RuntimeMode = "all"
	RuntimeModeAPI    RuntimeMode = "api"
	RuntimeModeWorker RuntimeMode = "worker"
)

type Config struct {
	HTTPAddr                   string
	DatabaseURL                string
	AuthTokenSecret            string
	AccessTokenTTL             time.Duration
	RefreshTokenTTL            time.Duration
	RedisURL                   string
	RuntimeMode                RuntimeMode
	CollectSourceID            string
	DashScopeAPIKey            string
	EmbeddingModel             string
	HotspotSimilarityThreshold float64
	HotspotWindow              time.Duration
	SMTPHost                   string
	SMTPPort                   int
	SMTPUsername               string
	SMTPPassword               string
	SMTPFrom                   string
	SMTPTLS                    bool
	SMTPStartTLS               bool
	XClientID                  string
	XClientSecret              string
	XRedirectURL               string
	MinIOEndpoint              string
	MinIOAccessKey             string
	MinIOSecretKey             string
	MinIOBucket                string
	MinIOUseSSL                bool
	MinIOLocation              string
	ContentRetentionDays       int
}

func Load() Config {
	return Config{
		HTTPAddr:                   envOrDefault("HOTKEY_HTTP_ADDR", ":8080"),
		DatabaseURL:                envOrDefault("HOTKEY_DATABASE_URL", "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable"),
		AuthTokenSecret:            os.Getenv("HOTKEY_AUTH_TOKEN_SECRET"),
		AccessTokenTTL:             durationOrDefault("HOTKEY_AUTH_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:            durationOrDefault("HOTKEY_AUTH_REFRESH_TOKEN_TTL", 30*24*time.Hour),
		RedisURL:                   envOrDefault("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/0"),
		RuntimeMode:                parseRuntimeMode(os.Getenv("HOTKEY_RUNTIME_MODE")),
		CollectSourceID:            envOrDefault("HOTKEY_COLLECT_SOURCE_ID", "default"),
		DashScopeAPIKey:            os.Getenv("HOTKEY_DASHSCOPE_API_KEY"),
		EmbeddingModel:             envOrDefault("HOTKEY_EMBEDDING_MODEL", "text-embedding-v2"),
		HotspotSimilarityThreshold: floatOrDefault("HOTKEY_HOTSPOT_SIMILARITY_THRESHOLD", 0.82),
		HotspotWindow:              durationOrDefault("HOTKEY_HOTSPOT_WINDOW", 24*time.Hour),
		SMTPHost:                   os.Getenv("HOTKEY_SMTP_HOST"),
		SMTPPort:                   intOrDefault("HOTKEY_SMTP_PORT", 587),
		SMTPUsername:               os.Getenv("HOTKEY_SMTP_USERNAME"),
		SMTPPassword:               os.Getenv("HOTKEY_SMTP_PASSWORD"),
		SMTPFrom:                   os.Getenv("HOTKEY_SMTP_FROM"),
		SMTPTLS:                    boolOrDefault("HOTKEY_SMTP_TLS", false),
		SMTPStartTLS:               boolOrDefault("HOTKEY_SMTP_STARTTLS", true),
		XClientID:                  os.Getenv("HOTKEY_X_CLIENT_ID"),
		XClientSecret:              os.Getenv("HOTKEY_X_CLIENT_SECRET"),
		XRedirectURL:               envOrDefault("HOTKEY_X_REDIRECT_URL", "http://localhost:8080/api/v1/admin/x/auth/callback"),
		MinIOEndpoint:              envOrDefault("HOTKEY_MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:             os.Getenv("HOTKEY_MINIO_ACCESS_KEY"),
		MinIOSecretKey:             os.Getenv("HOTKEY_MINIO_SECRET_KEY"),
		MinIOBucket:                envOrDefault("HOTKEY_MINIO_BUCKET", "hotkey-snapshots"),
		MinIOUseSSL:                boolOrDefault("HOTKEY_MINIO_USE_SSL", false),
		MinIOLocation:              envOrDefault("HOTKEY_MINIO_LOCATION", "us-east-1"),
		ContentRetentionDays:       intOrDefault("HOTKEY_CONTENT_RETENTION_DAYS", 30),
	}
}

func parseRuntimeMode(value string) RuntimeMode {
	switch RuntimeMode(value) {
	case RuntimeModeAPI:
		return RuntimeModeAPI
	case RuntimeModeWorker:
		return RuntimeModeWorker
	case RuntimeModeAll:
		return RuntimeModeAll
	default:
		return RuntimeModeAll
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

func floatOrDefault(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func intOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 65535 {
		return fallback
	}
	return parsed
}

func boolOrDefault(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
