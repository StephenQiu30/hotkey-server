package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// RuntimeMode 控制服务启动后运行哪些组件。
type RuntimeMode string

const (
	RuntimeModeAll    RuntimeMode = "all"
	RuntimeModeAPI    RuntimeMode = "api"
	RuntimeModeWorker RuntimeMode = "worker"
)

// Capabilities 标识各可选子系统是否已配置，供调用方做降级决策。
type Capabilities struct {
	SMTPEnabled      bool
	MinIOEnabled     bool
	DashScopeEnabled bool
	XEnabled         bool
}

// Config 是 hotkey-server 统一配置结构体。
// 加载顺序：默认值 → .env 文件 → 系统环境变量（系统环境变量优先级最高）。
type Config struct {
	// --- 应用基础 ---
	HTTPAddr     string
	AppEnv       string // local / development / test / staging / production
	AppName      string
	AppTimezone  string
	AppSecretKey string

	// --- 数据库 ---
	DatabaseURL string

	// --- 认证 ---
	AuthTokenSecret      string
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	JWTSecretKey         string
	JWTSessionExpireDays int

	// --- Redis ---
	RedisURL string

	// --- 运行模式 ---
	RuntimeMode     RuntimeMode
	CollectSourceID string

	// --- DashScope ---
	DashScopeAPIKey            string
	DashScopeBaseURL           string
	DashScopeChatModel         string
	EmbeddingModel             string
	HotspotSimilarityThreshold float64
	HotspotWindow              time.Duration

	// --- SMTP ---
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPTLS      bool
	SMTPStartTLS bool

	// --- 加密 ---
	EncryptionKey string

	// --- X 平台 OAuth ---
	XClientID     string
	XClientSecret string
	XRedirectURL  string

	// --- MinIO ---
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool
	MinIOLocation  string

	// --- 内容留存 ---
	ContentRetentionDays int

	// --- 调度 ---
	SchedulerEnabled bool

	// --- 日志 ---
	LogLevel  string // debug / info / warn / error
	LogFormat string // text / json

	// --- 可观测性 ---
	PprofEnabled             bool
	OTelExporterOTLPEndpoint string
}

// Load 从系统环境变量加载配置（向后兼容，不读取 .env 文件）。
func Load() Config {
	return loadConfig()
}

// LoadFromFile 先加载指定 .env 文件，再从系统环境变量读取配置。
// 系统环境变量始终覆盖 .env 文件中的值。
func LoadFromFile(path string) (Config, error) {
	if err := godotenv.Load(path); err != nil {
		return Config{}, fmt.Errorf("加载 env 文件失败 %s: %w", path, err)
	}
	return loadConfig(), nil
}

// Validate 校验必填配置项。返回的 error 包含所有缺失字段的人类可读信息。
func (c Config) Validate() error {
	var missing []string

	if c.DatabaseURL == "" {
		missing = append(missing, "HOTKEY_DATABASE_URL")
	}
	if c.RedisURL == "" {
		missing = append(missing, "HOTKEY_REDIS_URL")
	}

	if len(missing) > 0 {
		return fmt.Errorf("缺少必填配置项: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Capabilities 返回各可选子系统的启用状态。
func (c Config) Capabilities() Capabilities {
	return Capabilities{
		SMTPEnabled:      strings.TrimSpace(c.SMTPHost) != "",
		MinIOEnabled:     strings.TrimSpace(c.MinIOAccessKey) != "",
		DashScopeEnabled: strings.TrimSpace(c.DashScopeAPIKey) != "",
		XEnabled:         strings.TrimSpace(c.XClientID) != "",
	}
}

// loadConfig 统一从当前进程环境变量读取所有配置字段。
func loadConfig() Config {
	return Config{
		// --- 应用基础 ---
		HTTPAddr:     envOrDefault("HOTKEY_HTTP_ADDR", ":8080"),
		AppEnv:       envOrDefault("HOTKEY_APP_ENV", "local"),
		AppName:      envOrDefault("HOTKEY_APP_NAME", "hotkey-server"),
		AppTimezone:  envOrDefault("HOTKEY_APP_TIMEZONE", "Asia/Shanghai"),
		AppSecretKey: os.Getenv("HOTKEY_APP_SECRET_KEY"),

		// --- 数据库 ---
		DatabaseURL: envOrDefault("HOTKEY_DATABASE_URL", "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable"),

		// --- 认证 ---
		AuthTokenSecret:      os.Getenv("HOTKEY_AUTH_TOKEN_SECRET"),
		AccessTokenTTL:       durationOrDefault("HOTKEY_AUTH_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:      durationOrDefault("HOTKEY_AUTH_REFRESH_TOKEN_TTL", 30*24*time.Hour),
		JWTSecretKey:         os.Getenv("HOTKEY_JWT_SECRET_KEY"),
		JWTSessionExpireDays: intOrDefaultAllowZero("HOTKEY_JWT_SESSION_EXPIRE_DAYS", 7),

		// --- Redis ---
		RedisURL: envOrDefault("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/0"),

		// --- 运行模式 ---
		RuntimeMode:     parseRuntimeMode(os.Getenv("HOTKEY_RUNTIME_MODE")),
		CollectSourceID: envOrDefault("HOTKEY_COLLECT_SOURCE_ID", "default"),

		// --- DashScope ---
		DashScopeAPIKey:            os.Getenv("HOTKEY_DASHSCOPE_API_KEY"),
		DashScopeBaseURL:           envOrDefault("HOTKEY_DASHSCOPE_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
		DashScopeChatModel:         envOrDefault("HOTKEY_DASHSCOPE_CHAT_MODEL", "qwen-plus"),
		EmbeddingModel:             envOrDefault("HOTKEY_EMBEDDING_MODEL", "text-embedding-v2"),
		HotspotSimilarityThreshold: floatOrDefault("HOTKEY_HOTSPOT_SIMILARITY_THRESHOLD", 0.82),
		HotspotWindow:              durationOrDefault("HOTKEY_HOTSPOT_WINDOW", 24*time.Hour),

		// --- SMTP ---
		SMTPHost:     os.Getenv("HOTKEY_SMTP_HOST"),
		SMTPPort:     intOrDefault("HOTKEY_SMTP_PORT", 587),
		SMTPUsername: os.Getenv("HOTKEY_SMTP_USERNAME"),
		SMTPPassword: os.Getenv("HOTKEY_SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("HOTKEY_SMTP_FROM"),
		SMTPTLS:      boolOrDefault("HOTKEY_SMTP_TLS", false),
		SMTPStartTLS: boolOrDefault("HOTKEY_SMTP_STARTTLS", true),

		// --- 加密 ---
		EncryptionKey: os.Getenv("HOTKEY_ENCRYPTION_KEY"),

		// --- X 平台 OAuth ---
		XClientID:     os.Getenv("HOTKEY_X_CLIENT_ID"),
		XClientSecret: os.Getenv("HOTKEY_X_CLIENT_SECRET"),
		XRedirectURL:  envOrDefault("HOTKEY_X_REDIRECT_URL", "http://localhost:8080/api/v1/admin/x/auth/callback"),

		// --- MinIO ---
		MinIOEndpoint:  envOrDefault("HOTKEY_MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey: os.Getenv("HOTKEY_MINIO_ACCESS_KEY"),
		MinIOSecretKey: os.Getenv("HOTKEY_MINIO_SECRET_KEY"),
		MinIOBucket:    envOrDefault("HOTKEY_MINIO_BUCKET", "hotkey-snapshots"),
		MinIOUseSSL:    boolOrDefault("HOTKEY_MINIO_USE_SSL", false),
		MinIOLocation:  envOrDefault("HOTKEY_MINIO_LOCATION", "us-east-1"),

		// --- 内容留存 ---
		ContentRetentionDays: intOrDefaultAllowZero("HOTKEY_CONTENT_RETENTION_DAYS", 30),

		// --- 调度 ---
		SchedulerEnabled: boolOrDefault("HOTKEY_SCHEDULER_ENABLED", false),

		// --- 日志 ---
		LogLevel:  envOrDefault("HOTKEY_LOG_LEVEL", "info"),
		LogFormat: envOrDefault("HOTKEY_LOG_FORMAT", "text"),

		// --- 可观测性 ---
		PprofEnabled:             boolOrDefault("HOTKEY_PPROF_ENABLED", false),
		OTelExporterOTLPEndpoint: os.Getenv("HOTKEY_OTEL_EXPORTER_OTLP_ENDPOINT"),
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

func intOrDefaultAllowZero(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
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
