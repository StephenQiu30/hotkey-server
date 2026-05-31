package config

import "os"

type RuntimeMode string

const (
	RuntimeModeAll    RuntimeMode = "all"
	RuntimeModeAPI    RuntimeMode = "api"
	RuntimeModeWorker RuntimeMode = "worker"
)

type Config struct {
	HTTPAddr        string
	RedisURL        string
	RuntimeMode     RuntimeMode
	CollectSourceID string
}

func Load() Config {
	return Config{
		HTTPAddr:        envOrDefault("HOTKEY_HTTP_ADDR", ":8080"),
		RedisURL:        envOrDefault("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/0"),
		RuntimeMode:     parseRuntimeMode(os.Getenv("HOTKEY_RUNTIME_MODE")),
		CollectSourceID: envOrDefault("HOTKEY_COLLECT_SOURCE_ID", "default"),
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
