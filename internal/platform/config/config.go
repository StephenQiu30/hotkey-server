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
	Environment     string
	Role            string
	HTTPAddr        string
	ShutdownTimeout time.Duration
	DatabaseURL     string
	MinIO           MinIOConfig
	VaultPath       string
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

func Default() Config {
	return Config{
		Environment:     "development",
		Role:            "all",
		HTTPAddr:        ":8080",
		ShutdownTimeout: 15 * time.Second,
		VaultPath:       "./var/vault",
		MinIO: MinIOConfig{
			Endpoint: "localhost:9000",
			Bucket:   "hotkey-evidence",
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
		Environment:     v.GetString("env"),
		Role:            v.GetString("role"),
		HTTPAddr:        v.GetString("http_addr"),
		ShutdownTimeout: v.GetDuration("shutdown_timeout"),
		DatabaseURL:     v.GetString("database_url"),
		VaultPath:       v.GetString("vault_path"),
		MinIO: MinIOConfig{
			Endpoint:  v.GetString("minio_endpoint"),
			AccessKey: v.GetString("minio_access_key"),
			SecretKey: v.GetString("minio_secret_key"),
			Bucket:    v.GetString("minio_bucket"),
			UseSSL:    v.GetBool("minio_use_ssl"),
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
	}
	return nil
}

func setDefaults(v *viper.Viper, cfg Config) {
	v.SetDefault("env", cfg.Environment)
	v.SetDefault("role", cfg.Role)
	v.SetDefault("http_addr", cfg.HTTPAddr)
	v.SetDefault("shutdown_timeout", cfg.ShutdownTimeout)
	v.SetDefault("vault_path", cfg.VaultPath)
	v.SetDefault("minio_endpoint", cfg.MinIO.Endpoint)
	v.SetDefault("minio_bucket", cfg.MinIO.Bucket)
}

func configKeys() []string {
	return []string{
		"env", "role", "http_addr", "shutdown_timeout", "database_url",
		"minio_endpoint", "minio_access_key", "minio_secret_key", "minio_bucket",
		"minio_use_ssl", "vault_path",
	}
}
