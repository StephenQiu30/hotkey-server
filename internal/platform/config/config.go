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
	Environment      string
	Role             string
	HTTPAddr         string
	RequestTimeout   time.Duration
	ShutdownTimeout  time.Duration
	DatabaseURL      string
	OTLPHTTPEndpoint string
	MinIO            MinIOConfig
	VaultPath        string
	Authentication   AuthenticationConfig
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
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

func Default() Config {
	return Config{
		Environment:     "development",
		Role:            "all",
		HTTPAddr:        ":8080",
		RequestTimeout:  15 * time.Second,
		ShutdownTimeout: 15 * time.Second,
		VaultPath:       "./var/vault",
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
	if _, err := os.Stat(".env"); err == nil {
		v.SetConfigFile(".env")
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read .env: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("inspect .env: %w", err)
	}

	cfg := Config{
		Environment:      v.GetString("env"),
		Role:             v.GetString("role"),
		HTTPAddr:         v.GetString("http_addr"),
		RequestTimeout:   v.GetDuration("request_timeout"),
		ShutdownTimeout:  v.GetDuration("shutdown_timeout"),
		DatabaseURL:      v.GetString("database_url"),
		OTLPHTTPEndpoint: v.GetString("otlp_http_endpoint"),
		VaultPath:        v.GetString("vault_path"),
		MinIO: MinIOConfig{
			Endpoint:  v.GetString("minio_endpoint"),
			AccessKey: v.GetString("minio_access_key"),
			SecretKey: v.GetString("minio_secret_key"),
			Bucket:    v.GetString("minio_bucket"),
			UseSSL:    v.GetBool("minio_use_ssl"),
		},
		Authentication: AuthenticationConfig{
			JWTSecret:              v.GetString("jwt_secret"),
			JWTIssuer:              v.GetString("jwt_issuer"),
			JWTAudience:            v.GetString("jwt_audience"),
			VerificationHMACSecret: v.GetString("verification_hmac_secret"),
			RedisURL:               v.GetString("redis_url"),
			AllowedOrigins:         parseCSV(v.GetString("cors_allowed_origins")),
			RefreshCookieSecure:    v.GetBool("refresh_cookie_secure"),
			BootstrapAdminEmail:    v.GetString("bootstrap_admin_email"),
			BootstrapAdminPassword: v.GetString("bootstrap_admin_password"),
			SMTP: SMTPConfig{
				Enabled:   v.GetBool("smtp_enabled"),
				Host:      v.GetString("smtp_host"),
				Port:      v.GetInt("smtp_port"),
				TLSMode:   v.GetString("smtp_tls_mode"),
				Username:  v.GetString("smtp_username"),
				Password:  v.GetString("smtp_password"),
				FromEmail: v.GetString("smtp_from_email"),
				FromName:  v.GetString("smtp_from_name"),
			},
		},
	}
	return cfg, cfg.Validate()
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
		"env", "role", "http_addr", "request_timeout", "shutdown_timeout", "database_url", "otlp_http_endpoint",
		"minio_endpoint", "minio_access_key", "minio_secret_key", "minio_bucket",
		"minio_use_ssl", "vault_path",
		"jwt_secret", "jwt_issuer", "jwt_audience", "verification_hmac_secret", "redis_url", "smtp_enabled", "smtp_host", "smtp_port", "smtp_tls_mode", "smtp_username", "smtp_password", "smtp_from_email", "smtp_from_name", "cors_allowed_origins", "refresh_cookie_secure", "bootstrap_admin_email", "bootstrap_admin_password",
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
