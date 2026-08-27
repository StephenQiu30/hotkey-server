package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Environment               string
	Role                      string
	HTTPAddr                  string
	RequestTimeout            time.Duration
	ShutdownTimeout           time.Duration
	WorkerPollInterval        time.Duration
	WorkerConcurrency         int
	WorkerLeaseTimeout        time.Duration
	CronInterval              time.Duration
	DatabaseURL               string
	OTLPHTTPEndpoint          string
	SourceDNSOverHTTPSURL     string
	SourceCredentialMasterKey string
	BilibiliWebhookSecret     string
	MinIO                     MinIOConfig
	VaultPath                 string
	Authentication            AuthenticationConfig
	AI                        AIConfig
	Agent                     AgentConfig
	Notification              NotificationConfig
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type NotificationConfig struct {
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	MaxConnections    int
	WebOrigin         string
}

// ValidateRuntime 校验运行进程创建唯一 MinIO 客户端所需的最小配置。
// 错误只标识字段，不回显任何已配置的凭据。
func (c MinIOConfig) ValidateRuntime() error {
	switch {
	case strings.TrimSpace(c.Endpoint) == "":
		return errors.New("MinIO endpoint is required")
	case strings.TrimSpace(c.Bucket) == "":
		return errors.New("MinIO bucket is required")
	case strings.TrimSpace(c.AccessKey) == "":
		return errors.New("MinIO access key is required")
	case strings.TrimSpace(c.SecretKey) == "":
		return errors.New("MinIO secret key is required")
	default:
		return nil
	}
}

type AuthenticationConfig struct {
	JWTSecret              string
	JWTIssuer              string
	JWTAudience            string
	VerificationHMACSecret string
	RedisURL               string
	SMTP                   SMTPConfig
	AllowedOrigins         []string
	RefreshCookieSecure    bool
}

type SMTPConfig struct {
	Enabled   bool
	Host      string
	Port      int
	TLSMode   string
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

// ValidateRuntime 在 SMTP 启用时校验所有角色共享的连接配置。
// 禁用时允许保留空值，错误只标识字段，不回显主机、账号或凭据。
func (c SMTPConfig) ValidateRuntime() error {
	if !c.Enabled {
		return nil
	}
	switch {
	case strings.TrimSpace(c.Host) == "":
		return errors.New("SMTP host is required when SMTP is enabled")
	case c.Port < 1 || c.Port > 65535:
		return errors.New("SMTP port must be between 1 and 65535")
	case c.TLSMode != "tls" && c.TLSMode != "starttls":
		return errors.New("SMTP TLS mode must be tls or starttls")
	case !validMailbox(c.FromEmail):
		return errors.New("SMTP from email must be a valid mailbox")
	case (strings.TrimSpace(c.Username) == "") != (strings.TrimSpace(c.Password) == ""):
		return errors.New("SMTP username and password must be configured together")
	default:
		return nil
	}
}

func validMailbox(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return false
	}
	address, err := mail.ParseAddress(trimmed)
	return err == nil && address.Name == "" && address.Address == trimmed
}

// AIConfig 只保存 Provider 显式凭据和 PLAN-008 适配器使用的本地 ONNX 产物路径。
// 启动时不会隐式选择模型配置，因此这些字段均可留空。
type AIConfig struct {
	OpenAIAPIKey       string
	DeepSeekAPIKey     string
	OllamaEnabled      bool
	OllamaBaseURL      string
	ONNXRuntimeLibrary string
	ONNXModelPath      string
	ONNXTokenizerPath  string
	ONNXManifestPath   string
}

type AgentConfig struct {
	URL              string
	AuthToken        string
	MaxResponseBytes int64
	ShadowEnabled    bool
}

func (c AgentConfig) Enabled() bool {
	return c.ShadowEnabled && strings.TrimSpace(c.URL) != "" && strings.TrimSpace(c.AuthToken) != ""
}

func (c AgentConfig) Validate() error {
	urlValue := strings.TrimSpace(c.URL)
	token := strings.TrimSpace(c.AuthToken)
	if urlValue == "" && token == "" {
		if c.ShadowEnabled {
			return errors.New("Agent Shadow requires URL and auth token")
		}
		return nil
	}
	if urlValue == "" || token == "" {
		return errors.New("Agent URL and auth token must be configured together")
	}
	if len([]byte(token)) < 32 {
		return errors.New("Agent auth token must be at least 32 bytes")
	}
	parsed, err := url.Parse(urlValue)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("Agent URL must be an absolute HTTP(S) origin without credentials, path, query, or fragment")
	}
	if c.MaxResponseBytes <= 0 || c.MaxResponseBytes > 8<<20 {
		return errors.New("Agent max response bytes must be between 1 and 8388608")
	}
	return nil
}

func Default() Config {
	return Config{
		Environment:        "development",
		Role:               "all",
		HTTPAddr:           ":8866",
		RequestTimeout:     15 * time.Second,
		ShutdownTimeout:    15 * time.Second,
		WorkerPollInterval: time.Second,
		WorkerConcurrency:  1,
		WorkerLeaseTimeout: 5 * time.Minute,
		CronInterval:       time.Minute,
		VaultPath:          "./var/vault",
		MinIO: MinIOConfig{
			Endpoint: "localhost:9000",
			Bucket:   "hotkey-evidence",
		},
		Authentication: AuthenticationConfig{
			JWTIssuer:   "hotkey",
			JWTAudience: "hotkey-web",
			SMTP: SMTPConfig{
				Port:    465,
				TLSMode: "tls",
			},
		},
		AI: AIConfig{OllamaBaseURL: "http://127.0.0.1:11434"},
		Agent: AgentConfig{
			MaxResponseBytes: 1 << 20,
		},
		Notification: NotificationConfig{
			PollInterval:      time.Second,
			HeartbeatInterval: 10 * time.Second,
			MaxConnections:    100,
			WebOrigin:         "http://127.0.0.1:8010",
		},
	}
}

func Load() (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("HOTKEY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	defaults := Default()
	setDefaults(v, defaults)
	for _, key := range configKeys() {
		if err := v.BindEnv(key); err != nil {
			return Config{}, fmt.Errorf("bind environment key %s: %w", key, err)
		}
	}
	if err := loadEnvironmentFile(v, ".env"); err != nil {
		return Config{}, err
	}
	if configString(v, "env") == "production" {
		if err := loadEnvironmentFile(v, ".env.prod"); err != nil {
			return Config{}, err
		}
	}

	cfg := Config{
		Environment:               configString(v, "env"),
		Role:                      configString(v, "role"),
		HTTPAddr:                  configString(v, "http_addr"),
		RequestTimeout:            configDuration(v, "request_timeout"),
		ShutdownTimeout:           configDuration(v, "shutdown_timeout"),
		WorkerPollInterval:        configDuration(v, "worker_poll_interval"),
		WorkerConcurrency:         configInt(v, "worker_concurrency"),
		WorkerLeaseTimeout:        configDuration(v, "worker_lease_timeout"),
		CronInterval:              configDuration(v, "cron_interval"),
		DatabaseURL:               configString(v, "database_url"),
		OTLPHTTPEndpoint:          configString(v, "otlp_http_endpoint"),
		SourceDNSOverHTTPSURL:     configString(v, "source_doh_url"),
		SourceCredentialMasterKey: configString(v, "source_credential_master_key"),
		BilibiliWebhookSecret:     configString(v, "bilibili_webhook_secret"),
		VaultPath:                 configString(v, "vault_path"),
		MinIO: MinIOConfig{
			Endpoint:  configString(v, "minio_endpoint"),
			AccessKey: configString(v, "minio_access_key"),
			SecretKey: configString(v, "minio_secret_key"),
			Bucket:    configString(v, "minio_bucket"),
			UseSSL:    configBool(v, "minio_use_ssl"),
		},
		Authentication: AuthenticationConfig{
			JWTSecret:              configString(v, "jwt_secret"),
			JWTIssuer:              configString(v, "jwt_issuer"),
			JWTAudience:            configString(v, "jwt_audience"),
			VerificationHMACSecret: configString(v, "verification_hmac_secret"),
			RedisURL:               configString(v, "redis_url"),
			AllowedOrigins:         parseCSV(configString(v, "cors_allowed_origins")),
			RefreshCookieSecure:    configBool(v, "refresh_cookie_secure"),
			SMTP: SMTPConfig{
				Enabled:   configBool(v, "smtp_enabled"),
				Host:      configString(v, "smtp_host"),
				Port:      configInt(v, "smtp_port"),
				TLSMode:   configString(v, "smtp_tls_mode"),
				Username:  configString(v, "smtp_username"),
				Password:  configString(v, "smtp_password"),
				FromEmail: configString(v, "smtp_from_email"),
				FromName:  configString(v, "smtp_from_name"),
			},
		},
		AI: AIConfig{
			OpenAIAPIKey:       configString(v, "openai_api_key"),
			DeepSeekAPIKey:     configString(v, "deepseek_api_key"),
			OllamaEnabled:      configBool(v, "ollama_enabled"),
			OllamaBaseURL:      configString(v, "ollama_base_url"),
			ONNXRuntimeLibrary: configString(v, "onnx_runtime_library"),
			ONNXModelPath:      configString(v, "onnx_model_path"),
			ONNXTokenizerPath:  configString(v, "onnx_tokenizer_path"),
			ONNXManifestPath:   configString(v, "onnx_manifest_path"),
		},
		Agent: AgentConfig{
			URL:              configString(v, "agent_url"),
			AuthToken:        configString(v, "agent_auth_token"),
			MaxResponseBytes: int64(configInt(v, "agent_max_response_bytes")),
			ShadowEnabled:    configBool(v, "agent_shadow_enabled"),
		},
		Notification: NotificationConfig{
			PollInterval:      configDuration(v, "notification_poll_interval"),
			HeartbeatInterval: configDuration(v, "notification_heartbeat_interval"),
			MaxConnections:    configInt(v, "notification_max_connections"),
			WebOrigin:         configString(v, "notification_web_origin"),
		},
	}
	return cfg, cfg.Validate()
}

// loadEnvironmentFile 在文件存在时读取约定的 dotenv 配置。
// .env 是默认配置；只有解析后的环境为 production 时才叠加 .env.prod。
func loadEnvironmentFile(v *viper.Viper, path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect environment file %s: %w", path, err)
	}
	v.SetConfigFile(path)
	v.SetConfigType("env")
	if err := v.MergeInConfig(); err != nil {
		return fmt.Errorf("read environment file %s: %w", path, err)
	}
	return nil
}

func configKey(v *viper.Viper, key string) string {
	if _, present := os.LookupEnv("HOTKEY_" + strings.ToUpper(key)); present {
		return key
	}
	prefixedKey := "hotkey_" + key
	if v.InConfig(prefixedKey) {
		return prefixedKey
	}
	return key
}

func configString(v *viper.Viper, key string) string { return v.GetString(configKey(v, key)) }
func configBool(v *viper.Viper, key string) bool     { return v.GetBool(configKey(v, key)) }
func configInt(v *viper.Viper, key string) int       { return v.GetInt(configKey(v, key)) }
func configDuration(v *viper.Viper, key string) time.Duration {
	return v.GetDuration(configKey(v, key))
}

func (c Config) Validate() error {
	switch c.Environment {
	case "development", "testing", "production":
	default:
		return errors.New("environment must be development, testing, or production")
	}
	switch c.Role {
	case "all", "api", "worker":
	default:
		return fmt.Errorf("role must be all, api, or worker, got %q", c.Role)
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}
	if c.WorkerPollInterval < 0 || c.WorkerConcurrency < 0 || c.WorkerLeaseTimeout < 0 || c.CronInterval < 0 {
		return errors.New("worker runtime settings cannot be negative")
	}
	if c.WorkerConcurrency > 64 {
		return errors.New("worker concurrency must not exceed 64")
	}
	if c.Role != "worker" {
		if c.Notification.PollInterval <= 0 || c.Notification.HeartbeatInterval <= 0 {
			return errors.New("notification intervals must be positive")
		}
		if c.Notification.MaxConnections <= 0 || c.Notification.MaxConnections > 10000 {
			return errors.New("notification max connections must be between 1 and 10000")
		}
		if strings.TrimSpace(c.HTTPAddr) == "" {
			return errors.New("HTTP address is required for all and api roles")
		}
		if _, _, err := net.SplitHostPort(c.HTTPAddr); err != nil {
			return fmt.Errorf("invalid HTTP address %q: %w", c.HTTPAddr, err)
		}
		if c.RequestTimeout <= 0 {
			return errors.New("request timeout must be positive")
		}
	}
	if strings.TrimSpace(c.Notification.WebOrigin) != "" && !validCORSOrigin(c.Notification.WebOrigin) {
		return errors.New("notification web origin must be an absolute HTTP(S) origin")
	}
	if value := strings.TrimSpace(c.SourceCredentialMasterKey); value != "" {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			return errors.New("source credential master key must be Base64-encoded 32 bytes")
		}
	}
	if err := c.Agent.Validate(); err != nil {
		return err
	}
	return c.Authentication.SMTP.ValidateRuntime()
}

// ValidateRuntime 为所有运行角色补充显式数据库 URL 要求。
// Validate 仍可供不启动数据库生命周期的轻量构造测试使用。
func (c Config) ValidateRuntime() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("database URL is required for a running role")
	}
	return nil
}

// ValidateAuthenticationRuntime 与 ValidateRuntime 保持独立：数据库命令不需要身份认证，
// 提供身份接口的 API 则必须在装配路由前拒绝不安全密钥和携带凭据的 CORS 配置。
func (c Config) ValidateAuthenticationRuntime() error {
	if err := c.Validate(); err != nil {
		return err
	}
	auth := c.Authentication
	if len([]byte(strings.TrimSpace(auth.JWTSecret))) < 32 {
		return errors.New("JWT secret must be at least 32 bytes")
	}
	if knownAuthenticationPlaceholder(auth.JWTSecret) {
		return errors.New("JWT secret must not use a known placeholder")
	}
	if strings.TrimSpace(auth.JWTIssuer) == "" {
		return errors.New("JWT issuer is required")
	}
	if strings.TrimSpace(auth.JWTAudience) == "" {
		return errors.New("JWT audience is required")
	}
	if len([]byte(strings.TrimSpace(auth.VerificationHMACSecret))) < 32 {
		return errors.New("verification HMAC secret must be at least 32 bytes")
	}
	if knownAuthenticationPlaceholder(auth.VerificationHMACSecret) {
		return errors.New("verification HMAC secret must not use a known placeholder")
	}
	if len(auth.AllowedOrigins) == 0 {
		return errors.New("at least one allowed CORS origin is required for authentication")
	}
	for _, origin := range auth.AllowedOrigins {
		if !validCORSOrigin(origin) {
			return errors.New("authentication CORS origins must be absolute HTTP or HTTPS origins without credentials, paths, queries, or fragments")
		}
	}
	if c.Environment == "production" && !auth.RefreshCookieSecure {
		return errors.New("production refresh cookie must be secure")
	}
	return nil
}

func knownAuthenticationPlaceholder(value string) bool {
	switch strings.TrimSpace(value) {
	case "change-me-with-at-least-32-characters",
		"replace-with-your-random-secret-at-least-32-bytes",
		"replace-with-another-random-secret-32-bytes":
		return true
	default:
		return false
	}
}

func validCORSOrigin(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || value == "*" || strings.Contains(value, "#") {
		return false
	}
	origin, err := url.Parse(value)
	if err != nil || origin.Opaque != "" || origin.User != nil {
		return false
	}
	if origin.Scheme != "http" && origin.Scheme != "https" {
		return false
	}
	return origin.Host != "" && origin.Hostname() != "" && origin.Path == "" && origin.RawPath == "" &&
		!origin.ForceQuery && origin.RawQuery == "" && origin.Fragment == ""
}

func setDefaults(v *viper.Viper, cfg Config) {
	v.SetDefault("env", cfg.Environment)
	v.SetDefault("role", cfg.Role)
	v.SetDefault("http_addr", cfg.HTTPAddr)
	v.SetDefault("request_timeout", cfg.RequestTimeout)
	v.SetDefault("shutdown_timeout", cfg.ShutdownTimeout)
	v.SetDefault("worker_poll_interval", cfg.WorkerPollInterval)
	v.SetDefault("worker_concurrency", cfg.WorkerConcurrency)
	v.SetDefault("worker_lease_timeout", cfg.WorkerLeaseTimeout)
	v.SetDefault("cron_interval", cfg.CronInterval)
	v.SetDefault("vault_path", cfg.VaultPath)
	v.SetDefault("minio_endpoint", cfg.MinIO.Endpoint)
	v.SetDefault("minio_bucket", cfg.MinIO.Bucket)
	v.SetDefault("jwt_issuer", cfg.Authentication.JWTIssuer)
	v.SetDefault("jwt_audience", cfg.Authentication.JWTAudience)
	v.SetDefault("smtp_port", cfg.Authentication.SMTP.Port)
	v.SetDefault("smtp_tls_mode", cfg.Authentication.SMTP.TLSMode)
	v.SetDefault("smtp_from_name", "HotKey")
	v.SetDefault("ollama_enabled", cfg.AI.OllamaEnabled)
	v.SetDefault("ollama_base_url", cfg.AI.OllamaBaseURL)
	v.SetDefault("agent_max_response_bytes", cfg.Agent.MaxResponseBytes)
	v.SetDefault("agent_shadow_enabled", cfg.Agent.ShadowEnabled)
	v.SetDefault("notification_poll_interval", cfg.Notification.PollInterval)
	v.SetDefault("notification_heartbeat_interval", cfg.Notification.HeartbeatInterval)
	v.SetDefault("notification_max_connections", cfg.Notification.MaxConnections)
	v.SetDefault("notification_web_origin", cfg.Notification.WebOrigin)
}

func configKeys() []string {
	return []string{
		"env", "role", "http_addr", "request_timeout", "shutdown_timeout", "worker_poll_interval", "worker_concurrency", "worker_lease_timeout", "cron_interval", "database_url", "otlp_http_endpoint",
		"source_doh_url", "source_credential_master_key", "bilibili_webhook_secret",
		"minio_endpoint", "minio_access_key", "minio_secret_key", "minio_bucket",
		"minio_use_ssl", "vault_path",
		"jwt_secret", "jwt_issuer", "jwt_audience", "verification_hmac_secret", "redis_url", "smtp_enabled", "smtp_host", "smtp_port", "smtp_tls_mode", "smtp_username", "smtp_password", "smtp_from_email", "smtp_from_name", "cors_allowed_origins", "refresh_cookie_secure",
		"openai_api_key", "deepseek_api_key", "ollama_enabled", "ollama_base_url", "onnx_runtime_library", "onnx_model_path", "onnx_tokenizer_path", "onnx_manifest_path",
		"agent_url", "agent_auth_token", "agent_max_response_bytes", "agent_shadow_enabled",
		"notification_poll_interval", "notification_heartbeat_interval", "notification_max_connections", "notification_web_origin",
	}
}

func parseCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	items := strings.Split(value, ",")
	values := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}
