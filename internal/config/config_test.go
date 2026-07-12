package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

func TestLoad_DailyDigestConfigDefaults(t *testing.T) {
	// Ensure required fields are present so Load() succeeds.
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("X_BEARER_TOKEN", "test-token")

	// Clear all digest-related and LLM env vars to test defaults.
	for _, k := range []string{
		"OBSIDIAN_VAULT_PATH",
		"DAILY_DIGEST_TIME",
		"DAILY_DIGEST_TIMEZONE",
		"DAILY_DIGEST_TARGET",
		"DAILY_DIGEST_TOP_N",
		"LLM_PROVIDER",
		"LLM_API_KEY",
		"LLM_BASE_URL",
		"LLM_MODEL",
		"LLM_MAX_TOKENS",
		"LLM_TEMPERATURE",
	} {
		os.Unsetenv(k)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// ObsidianVaultPath has no default; should be empty when unset.
	if cfg.ObsidianVaultPath != "" {
		t.Errorf("ObsidianVaultPath = %q, want empty", cfg.ObsidianVaultPath)
	}

	if cfg.DailyDigestTime != "08:00" {
		t.Errorf("DailyDigestTime = %q, want %q", cfg.DailyDigestTime, "08:00")
	}

	if cfg.DailyDigestTimezone != "Asia/Shanghai" {
		t.Errorf("DailyDigestTimezone = %q, want %q", cfg.DailyDigestTimezone, "Asia/Shanghai")
	}

	if cfg.DailyDigestTarget != "yesterday" {
		t.Errorf("DailyDigestTarget = %q, want %q", cfg.DailyDigestTarget, "yesterday")
	}

	if cfg.DailyDigestTopN != 20 {
		t.Errorf("DailyDigestTopN = %d, want %d", cfg.DailyDigestTopN, 20)
	}

	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "openai")
	}

	if cfg.LLMAPIKey != "" {
		t.Errorf("LLMAPIKey = %q, want empty", cfg.LLMAPIKey)
	}

	if cfg.LLMBaseURL != "https://api.openai.com/v1" {
		t.Errorf("LLMBaseURL = %q, want %q", cfg.LLMBaseURL, "https://api.openai.com/v1")
	}

	if cfg.LLMModel != "gpt-4o-mini" {
		t.Errorf("LLMModel = %q, want %q", cfg.LLMModel, "gpt-4o-mini")
	}

	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("LLMMaxTokens = %d, want %d", cfg.LLMMaxTokens, 4096)
	}

	if cfg.LLMTemperature != 0.7 {
		t.Errorf("LLMTemperature = %f, want %f", cfg.LLMTemperature, 0.7)
	}
}

func TestLoad_RejectsShortJWTSecretInProd(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "short")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	t.Setenv("APP_ENV", "production")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for short JWT_SECRET in production")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error should mention 32-character minimum, got: %v", err)
	}
}

func TestLoad_AcceptsShortJWTSecretInDev(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "short")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	t.Setenv("APP_ENV", "development")

	_, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error for short JWT_SECRET in dev: %v", err)
	}
}

func TestLoad_RejectsEmptyVerificationPepperInProd(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-32-byte-secret-key-for-testing!!")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	t.Setenv("APP_ENV", "production")
	// Explicitly leave AUTH_VERIFICATION_PEPPER unset / empty

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for empty AUTH_VERIFICATION_PEPPER in production")
	}
	if !strings.Contains(err.Error(), "VERIFICATION_PEPPER") {
		t.Errorf("error should mention AUTH_VERIFICATION_PEPPER, got: %v", err)
	}
}

func TestLoad_RejectsWildcardOriginInsecure(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-32-byte-secret-key-for-testing!!")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	t.Setenv("WEB_ALLOWED_ORIGINS", "*")
	t.Setenv("AUTH_COOKIE_SECURE", "false")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for wildcard origin with CookieSecure=false")
	}
	if !strings.Contains(err.Error(), "WEB_ALLOWED_ORIGINS") {
		t.Errorf("error should mention WEB_ALLOWED_ORIGINS, got: %v", err)
	}
}

func TestLoad_RejectsEmptySMTPCodeWhenHostSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-32-byte-secret-key-for-testing!!")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	// SMTP_AUTH_CODE left empty

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for empty SMTP_AUTH_CODE when SMTP_HOST is set")
	}
	if !strings.Contains(err.Error(), "SMTP_AUTH_CODE") {
		t.Errorf("error should mention SMTP_AUTH_CODE, got: %v", err)
	}
}

func TestLoad_AcceptsNonProductionValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "short") // short is OK in dev
	t.Setenv("X_BEARER_TOKEN", "test-token")
	t.Setenv("APP_ENV", "development")
	// AUTH_VERIFICATION_PEPPER empty is OK in dev
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_AUTH_CODE", "smtp-auth-code")

	_, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error for non-production config: %v", err)
	}
}

func TestLoad_AuthDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-32-byte-secret-key-for-testing!!")
	t.Setenv("X_BEARER_TOKEN", "test-token")

	// Clear auth-related env vars to test defaults.
	for _, k := range []string{
		"APP_ENV",
		"JWT_ISSUER",
		"JWT_AUDIENCE",
		"AUTH_COOKIE_SECURE",
		"AUTH_COOKIE_DOMAIN",
		"SMTP_HOST",
		"SMTP_PORT",
		"SMTP_USERNAME",
		"SMTP_AUTH_CODE",
		"SMTP_FROM_EMAIL",
		"SMTP_FROM_NAME",
	} {
		os.Unsetenv(k)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv = %q, want %q", cfg.AppEnv, "development")
	}
	if cfg.JWTIssuer != "hotkey-server" {
		t.Errorf("JWTIssuer = %q, want %q", cfg.JWTIssuer, "hotkey-server")
	}
	if cfg.JWTAudience != "hotkey-web" {
		t.Errorf("JWTAudience = %q, want %q", cfg.JWTAudience, "hotkey-web")
	}
	if cfg.CookieSecure {
		t.Errorf("CookieSecure = true, want false (dev default)")
	}
	if cfg.CookieDomain != "" {
		t.Errorf("CookieDomain = %q, want empty", cfg.CookieDomain)
	}
	if cfg.SMTPPort != 465 {
		t.Errorf("SMTPPort = %d, want %d", cfg.SMTPPort, 465)
	}
}
