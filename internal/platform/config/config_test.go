package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultIsValid(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default().Validate() error = %v", err)
	}
	if cfg.Role != "all" {
		t.Fatalf("Default().Role = %q, want all", cfg.Role)
	}
}

func TestValidateRejectsInvalidRuntimeConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "role", mutate: func(c *Config) { c.Role = "scheduler" }},
		{name: "http address", mutate: func(c *Config) { c.HTTPAddr = "" }},
		{name: "shutdown timeout", mutate: func(c *Config) { c.ShutdownTimeout = 0 }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := Default()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}

func TestValidateAcceptsWorkerWithoutListeningAddress(t *testing.T) {
	t.Parallel()

	cfg := Config{Role: "worker", ShutdownTimeout: time.Second}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRuntimeRequiresCompleteMinIOConfigurationWithoutLeakingSecret(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "endpoint", mutate: func(config *Config) { config.MinIO.Endpoint = "" }},
		{name: "bucket", mutate: func(config *Config) { config.MinIO.Bucket = "" }},
		{name: "access key", mutate: func(config *Config) { config.MinIO.AccessKey = "" }},
		{name: "secret key", mutate: func(config *Config) { config.MinIO.SecretKey = "" }},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config := Default()
			config.Role = "worker"
			config.DatabaseURL = "postgres://fixture"
			config.MinIO.AccessKey = "fixture-access"
			config.MinIO.SecretKey = "fixture-secret-must-not-appear"
			test.mutate(&config)

			err := config.ValidateRuntime()
			if err == nil {
				t.Fatal("ValidateRuntime() error = nil, want incomplete MinIO rejection")
			}
			if strings.Contains(err.Error(), "fixture-secret-must-not-appear") {
				t.Fatalf("ValidateRuntime() leaked MinIO secret: %v", err)
			}
		})
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("HOTKEY_ROLE", "worker")
	t.Setenv("HOTKEY_HTTP_ADDR", "")
	t.Setenv("HOTKEY_SHUTDOWN_TIMEOUT", "3s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Role != "worker" {
		t.Errorf("Role = %q, want worker", cfg.Role)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 3s", cfg.ShutdownTimeout)
	}
}

func TestValidateAuthenticationRuntimeRejectsUnsafeConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing JWT secret", mutate: func(c *Config) { c.Authentication.JWTSecret = "" }},
		{name: "short JWT secret", mutate: func(c *Config) { c.Authentication.JWTSecret = "short" }},
		{name: "production insecure cookie", mutate: func(c *Config) {
			c.Environment = "production"
			c.Authentication.RefreshCookieSecure = false
		}},
		{name: "production wildcard CORS", mutate: func(c *Config) {
			c.Environment = "production"
			c.Authentication.RefreshCookieSecure = true
			c.Authentication.AllowedOrigins = []string{"*"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAuthenticationConfig()
			tt.mutate(&cfg)
			if err := cfg.ValidateAuthenticationRuntime(); err == nil {
				t.Fatal("ValidateAuthenticationRuntime() error = nil, want an error")
			}
		})
	}
}

func TestValidateAuthenticationRuntimeRequiresVerificationHMACSecret(t *testing.T) {
	t.Parallel()

	for _, secret := range []string{"", "too-short"} {
		cfg := validAuthenticationConfig()
		cfg.Authentication.VerificationHMACSecret = secret
		if err := cfg.ValidateAuthenticationRuntime(); err == nil {
			t.Fatalf("ValidateAuthenticationRuntime() with HMAC secret %q error = nil, want rejection", secret)
		}
	}
}

func TestValidateAuthenticationRuntimeAcceptsExplicitSafeConfiguration(t *testing.T) {
	t.Parallel()

	cfg := validAuthenticationConfig()
	if err := cfg.ValidateAuthenticationRuntime(); err != nil {
		t.Fatalf("ValidateAuthenticationRuntime() error = %v", err)
	}
}

func TestLoadReadsNamedAuthenticationSettings(t *testing.T) {
	t.Setenv("HOTKEY_JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("HOTKEY_JWT_ISSUER", "hotkey-test")
	t.Setenv("HOTKEY_JWT_AUDIENCE", "hotkey-web")
	t.Setenv("HOTKEY_VERIFICATION_HMAC_SECRET", "verification-hmac-secret-for-tests-32-bytes")
	t.Setenv("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/15")
	t.Setenv("HOTKEY_SMTP_HOST", "smtp.163.com")
	t.Setenv("HOTKEY_SMTP_PORT", "465")
	t.Setenv("HOTKEY_SMTP_TLS_MODE", "tls")
	t.Setenv("HOTKEY_SMTP_USERNAME", "sender@163.com")
	t.Setenv("HOTKEY_SMTP_PASSWORD", "app-password")
	t.Setenv("HOTKEY_SMTP_FROM_EMAIL", "sender@163.com")
	t.Setenv("HOTKEY_SMTP_FROM_NAME", "HotKey")
	t.Setenv("HOTKEY_CORS_ALLOWED_ORIGINS", "https://app.example.test,https://admin.example.test")
	t.Setenv("HOTKEY_REFRESH_COOKIE_SECURE", "true")
	t.Setenv("HOTKEY_BOOTSTRAP_ADMIN_EMAIL", "admin@example.test")
	t.Setenv("HOTKEY_BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-password")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Authentication.JWTIssuer != "hotkey-test" || cfg.Authentication.VerificationHMACSecret != "verification-hmac-secret-for-tests-32-bytes" || cfg.Authentication.SMTP.Host != "smtp.163.com" {
		t.Errorf("Load() authentication = %#v, want named values", cfg.Authentication)
	}
	if got, want := strings.Join(cfg.Authentication.AllowedOrigins, ","), "https://app.example.test,https://admin.example.test"; got != want {
		t.Errorf("AllowedOrigins = %q, want %q", got, want)
	}
}

func TestLoadUsesDefaultEnvironmentFileAndProcessOverrides(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte(strings.Join([]string{
		"HOTKEY_ROLE=worker",
		"HOTKEY_JWT_SECRET=default-development-secret-with-more-than-32-bytes",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(directory)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() default environment: %v", err)
	}
	if cfg.Role != "worker" || cfg.Authentication.JWTSecret != "default-development-secret-with-more-than-32-bytes" {
		t.Fatalf("Load() default environment = %#v", cfg)
	}

	t.Setenv("HOTKEY_ROLE", "api")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() process override: %v", err)
	}
	if cfg.Role != "api" {
		t.Fatalf("Load() process role = %q, want api", cfg.Role)
	}
}

func TestLoadUsesProductionEnvironmentFile(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte(strings.Join([]string{
		"HOTKEY_ENV=production",
		"HOTKEY_ROLE=worker",
		"HOTKEY_JWT_SECRET=default-development-secret-with-more-than-32-bytes",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".env.prod"), []byte(strings.Join([]string{
		"HOTKEY_JWT_SECRET=production-secret-with-more-than-32-bytes",
		"HOTKEY_VERIFICATION_HMAC_SECRET=production-hmac-secret-with-more-than-32-bytes",
		"HOTKEY_CORS_ALLOWED_ORIGINS=https://app.example.test",
		"HOTKEY_REFRESH_COOKIE_SECURE=true",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(directory)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() production environment: %v", err)
	}
	if cfg.Environment != "production" || cfg.Authentication.JWTSecret != "production-secret-with-more-than-32-bytes" || !cfg.Authentication.RefreshCookieSecure {
		t.Fatalf("Load() production environment = %#v", cfg)
	}
}

func validAuthenticationConfig() Config {
	cfg := Default()
	cfg.Authentication = AuthenticationConfig{
		JWTSecret:              "0123456789abcdef0123456789abcdef",
		JWTIssuer:              "hotkey",
		JWTAudience:            "hotkey-web",
		VerificationHMACSecret: "verification-hmac-secret-for-tests-32-bytes",
		AllowedOrigins:         []string{"http://localhost:3000"},
		RefreshCookieSecure:    false,
	}
	return cfg
}
