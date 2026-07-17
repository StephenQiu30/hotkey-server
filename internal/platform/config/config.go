package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Environment        string
	Role               string
	HTTPAddr           string
	RequestTimeout     time.Duration
	ShutdownTimeout    time.Duration
	WorkerPollInterval time.Duration
	WorkerConcurrency  int
	WorkerLeaseTimeout time.Duration
	CronInterval       time.Duration
	DatabaseURL        string
	OTLPHTTPEndpoint   string
	MinIO              MinIOConfig
	VaultPath          string
	Authentication     AuthenticationConfig
	AI                 AIConfig
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// ValidateRuntime verifies the minimum credentials required to construct the
// single MinIO client used by a running process. It intentionally names only
// missing fields so an error can never echo a configured secret.
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
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
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

// AIConfig contains only the explicit provider credentials and local ONNX
// artifact locations required by the PLAN-008 adapters. All values remain
// optional at process startup because no profile is selected implicitly.
type AIConfig struct {
	OpenAIAPIKey       string
	ONNXRuntimeLibrary string
	ONNXModelPath      string
	ONNXTokenizerPath  string
	ONNXManifestPath   string
}

func Default() Config {
	return Config{
		Environment:        "development",
		Role:               "all",
		HTTPAddr:           ":8080",
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
		Environment:        configString(v, "env"),
		Role:               configString(v, "role"),
		HTTPAddr:           configString(v, "http_addr"),
		RequestTimeout:     configDuration(v, "request_timeout"),
		ShutdownTimeout:    configDuration(v, "shutdown_timeout"),
		WorkerPollInterval: configDuration(v, "worker_poll_interval"),
		WorkerConcurrency:  configInt(v, "worker_concurrency"),
		WorkerLeaseTimeout: configDuration(v, "worker_lease_timeout"),
		CronInterval:       configDuration(v, "cron_interval"),
		DatabaseURL:        configString(v, "database_url"),
		OTLPHTTPEndpoint:   configString(v, "otlp_http_endpoint"),
		VaultPath:          configString(v, "vault_path"),
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
			BootstrapAdminEmail:    configString(v, "bootstrap_admin_email"),
			BootstrapAdminPassword: configString(v, "bootstrap_admin_password"),
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
			ONNXRuntimeLibrary: configString(v, "onnx_runtime_library"),
			ONNXModelPath:      configString(v, "onnx_model_path"),
			ONNXTokenizerPath:  configString(v, "onnx_tokenizer_path"),
			ONNXManifestPath:   configString(v, "onnx_manifest_path"),
		},
	}
	return cfg, cfg.Validate()
}

// loadEnvironmentFile reads one conventional dotenv file when it exists.
// .env is the default development configuration; .env.prod is loaded only
// when the resolved environment is production.
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
	return nil
}

// ValidateRuntime adds the production requirement that every running role has
// an explicit database URL. Validate stays usable for lightweight constructor
// tests that intentionally do not start a database lifecycle.
func (c Config) ValidateRuntime() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("database URL is required for a running role")
	}
	return nil
}

// ValidateAuthenticationRuntime is intentionally separate from ValidateRuntime:
// database commands do not need email authentication, while an API runtime that
// serves identity endpoints must reject unsafe credential and credentialed CORS
// configuration before wiring those endpoints.
func (c Config) ValidateAuthenticationRuntime() error {
	if err := c.Validate(); err != nil {
		return err
	}
	auth := c.Authentication
	if len([]byte(strings.TrimSpace(auth.JWTSecret))) < 32 {
		return errors.New("JWT secret must be at least 32 bytes")
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
	if len(auth.AllowedOrigins) == 0 {
		return errors.New("at least one allowed CORS origin is required for authentication")
	}
	for _, origin := range auth.AllowedOrigins {
		if strings.TrimSpace(origin) == "" || origin == "*" {
			return errors.New("authentication CORS origins must be explicit")
		}
	}
	if c.Environment == "production" && !auth.RefreshCookieSecure {
		return errors.New("production refresh cookie must be secure")
	}
	return nil
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
}

func configKeys() []string {
	return []string{
		"env", "role", "http_addr", "request_timeout", "shutdown_timeout", "worker_poll_interval", "worker_concurrency", "worker_lease_timeout", "cron_interval", "database_url", "otlp_http_endpoint",
		"minio_endpoint", "minio_access_key", "minio_secret_key", "minio_bucket",
		"minio_use_ssl", "vault_path",
		"jwt_secret", "jwt_issuer", "jwt_audience", "verification_hmac_secret", "redis_url", "smtp_enabled", "smtp_host", "smtp_port", "smtp_tls_mode", "smtp_username", "smtp_password", "smtp_from_email", "smtp_from_name", "cors_allowed_origins", "refresh_cookie_secure", "bootstrap_admin_email", "bootstrap_admin_password",
		"openai_api_key", "onnx_runtime_library", "onnx_model_path", "onnx_tokenizer_path", "onnx_manifest_path",
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
