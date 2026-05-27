package config

import (
	"os"
	"strconv"
)

const (
	defaultHTTPAddr    = ":8080"
	defaultDatabaseURL = "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable"
	defaultRedisURL    = "redis://localhost:6379/0"

	defaultDashScopeBaseURL        = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	defaultDashScopeChatModel      = "qwen-plus"
	defaultDashScopeEmbeddingModel = "text-embedding-v2"
	defaultEmbeddingDimension      = 1536
	defaultAIErrorStrategy         = "fallback"
)

type Config struct {
	HTTPAddr                string
	DatabaseURL             string
	RedisURL                string
	InternalAPIKey          string
	DefaultTenantID         string
	DashScopeAPIKey         string
	DashScopeBaseURL        string
	DashScopeChatModel      string
	DashScopeEmbeddingModel string
	EmbeddingDimension      int
	AIProviderErrorStrategy string
}

func Load() Config {
	embeddingDim := defaultEmbeddingDimension
	if v := os.Getenv("EMBEDDING_DIMENSION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			embeddingDim = n
		}
	}

	return Config{
		HTTPAddr:                envOrDefault("HOTKEY_HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:             envOrDefault("HOTKEY_DATABASE_URL", defaultDatabaseURL),
		RedisURL:                envOrDefault("HOTKEY_REDIS_URL", defaultRedisURL),
		InternalAPIKey:          os.Getenv("HOTKEY_INTERNAL_API_KEY"),
		DefaultTenantID:         envOrDefault("HOTKEY_DEFAULT_TENANT_ID", ""),
		DashScopeAPIKey:         os.Getenv("DASHSCOPE_API_KEY"),
		DashScopeBaseURL:        envOrDefault("DASHSCOPE_BASE_URL", defaultDashScopeBaseURL),
		DashScopeChatModel:      envOrDefault("DASHSCOPE_CHAT_MODEL", defaultDashScopeChatModel),
		DashScopeEmbeddingModel: envOrDefault("DASHSCOPE_EMBEDDING_MODEL", defaultDashScopeEmbeddingModel),
		EmbeddingDimension:      embeddingDim,
		AIProviderErrorStrategy: envOrDefault("AI_PROVIDER_ERROR_STRATEGY", defaultAIErrorStrategy),
	}
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
