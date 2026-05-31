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
}

func Load() Config {
	return Config{
		HTTPAddr:                   envOrDefault("HOTKEY_HTTP_ADDR", ":8080"),
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
