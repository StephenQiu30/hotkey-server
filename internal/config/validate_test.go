package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- RED: Validate() returns error when required fields are missing ---

func TestValidate_MissingDatabaseURL_ReturnsError(t *testing.T) {
	cfg := Config{
		DatabaseURL: "",
		RedisURL:    "redis://localhost:6379/0",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when DatabaseURL is empty")
	}
	if !strings.Contains(err.Error(), "HOTKEY_DATABASE_URL") {
		t.Fatalf("error should mention HOTKEY_DATABASE_URL, got: %s", err.Error())
	}
}

func TestValidate_MissingRedisURL_ReturnsError(t *testing.T) {
	cfg := Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable",
		RedisURL:    "",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when RedisURL is empty")
	}
	if !strings.Contains(err.Error(), "HOTKEY_REDIS_URL") {
		t.Fatalf("error should mention HOTKEY_REDIS_URL, got: %s", err.Error())
	}
}

func TestValidate_MultipleMissing_ReturnsCombinedError(t *testing.T) {
	cfg := Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when both DatabaseURL and RedisURL are empty")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "HOTKEY_DATABASE_URL") || !strings.Contains(errStr, "HOTKEY_REDIS_URL") {
		t.Fatalf("error should mention both fields, got: %s", errStr)
	}
}

func TestValidate_AllRequiredPresent_ReturnsNil(t *testing.T) {
	cfg := Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable",
		RedisURL:    "redis://localhost:6379/0",
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error, got: %s", err.Error())
	}
}

// --- RED: Capability flags for optional features ---

func TestCapabilityFlags_AllDisabled_WhenOptionalFieldsEmpty(t *testing.T) {
	cfg := Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable",
		RedisURL:    "redis://localhost:6379/0",
		// SMTP, MinIO, DashScope, X all empty
	}
	caps := cfg.Capabilities()

	if caps.SMTPEnabled {
		t.Error("expected SMTPEnabled=false when SMTPHost is empty")
	}
	if caps.MinIOEnabled {
		t.Error("expected MinIOEnabled=false when MinIOAccessKey is empty")
	}
	if caps.DashScopeEnabled {
		t.Error("expected DashScopeEnabled=false when DashScopeAPIKey is empty")
	}
	if caps.XEnabled {
		t.Error("expected XEnabled=false when XClientID is empty")
	}
}

func TestCapabilityFlags_SMTPEnabled_WhenHostSet(t *testing.T) {
	cfg := Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable",
		RedisURL:    "redis://localhost:6379/0",
		SMTPHost:    "smtp.example.com",
	}
	caps := cfg.Capabilities()
	if !caps.SMTPEnabled {
		t.Error("expected SMTPEnabled=true when SMTPHost is set")
	}
}

func TestCapabilityFlags_MinIOEnabled_WhenAccessKeySet(t *testing.T) {
	cfg := Config{
		DatabaseURL:    "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable",
		RedisURL:       "redis://localhost:6379/0",
		MinIOAccessKey: "mykey",
	}
	caps := cfg.Capabilities()
	if !caps.MinIOEnabled {
		t.Error("expected MinIOEnabled=true when MinIOAccessKey is set")
	}
}

func TestCapabilityFlags_DashScopeEnabled_WhenAPIKeySet(t *testing.T) {
	cfg := Config{
		DatabaseURL:     "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable",
		RedisURL:        "redis://localhost:6379/0",
		DashScopeAPIKey: "dashscope-key",
	}
	caps := cfg.Capabilities()
	if !caps.DashScopeEnabled {
		t.Error("expected DashScopeEnabled=true when DashScopeAPIKey is set")
	}
}

func TestCapabilityFlags_XEnabled_WhenClientIDSet(t *testing.T) {
	cfg := Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable",
		RedisURL:    "redis://localhost:6379/0",
		XClientID:   "x-client-id",
	}
	caps := cfg.Capabilities()
	if !caps.XEnabled {
		t.Error("expected XEnabled=true when XClientID is set")
	}
}

// --- RED: LoadFromFile loads .env file and populates config ---

func TestLoadFromFile_LoadsEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env.test")
	content := `HOTKEY_HTTP_ADDR=:9090
HOTKEY_DATABASE_URL=postgres://test:test@db:5432/testdb?sslmode=disable
HOTKEY_REDIS_URL=redis://redis:6379/1
HOTKEY_DASHSCOPE_API_KEY=test-dashscope-key
HOTKEY_SMTP_HOST=smtp.test.com
`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test env file: %s", err)
	}

	cfg, err := LoadFromFile(envFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %s", err)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":9090")
	}
	if cfg.DatabaseURL != "postgres://test:test@db:5432/testdb?sslmode=disable" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://test:test@db:5432/testdb?sslmode=disable")
	}
	if cfg.RedisURL != "redis://redis:6379/1" {
		t.Errorf("RedisURL = %q, want %q", cfg.RedisURL, "redis://redis:6379/1")
	}
	if cfg.DashScopeAPIKey != "test-dashscope-key" {
		t.Errorf("DashScopeAPIKey = %q, want %q", cfg.DashScopeAPIKey, "test-dashscope-key")
	}
	if cfg.SMTPHost != "smtp.test.com" {
		t.Errorf("SMTPHost = %q, want %q", cfg.SMTPHost, "smtp.test.com")
	}
}

// --- RED: System env vars override .env file values ---

func TestLoadFromFile_SystemEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env.test")
	content := `HOTKEY_HTTP_ADDR=:9090
HOTKEY_DATABASE_URL=postgres://file:file@localhost:5432/filedb?sslmode=disable
`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test env file: %s", err)
	}

	// System env should take precedence over .env file
	t.Setenv("HOTKEY_HTTP_ADDR", ":7070")

	cfg, err := LoadFromFile(envFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %s", err)
	}

	if cfg.HTTPAddr != ":7070" {
		t.Errorf("HTTPAddr = %q, want %q (system env should override .env file)", cfg.HTTPAddr, ":7070")
	}
}

// --- RED: LoadFromFile returns error for non-existent file ---

func TestLoadFromFile_NonExistentFile_ReturnsError(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/.env")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// --- RED: Load() continues to work without .env file ---

func TestLoad_BackwardCompatible_WithoutEnvFile(t *testing.T) {
	t.Setenv("HOTKEY_HTTP_ADDR", ":3030")
	t.Setenv("HOTKEY_DATABASE_URL", "postgres://compat:compat@localhost:5432/compatdb?sslmode=disable")
	t.Setenv("HOTKEY_REDIS_URL", "redis://localhost:6379/2")

	cfg := Load()

	if cfg.HTTPAddr != ":3030" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":3030")
	}
	if cfg.DatabaseURL != "postgres://compat:compat@localhost:5432/compatdb?sslmode=disable" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://compat:compat@localhost:5432/compatdb?sslmode=disable")
	}
}

// --- RED: Config has new fields for APP, LOG, JWT, Scheduler ---

func TestLoad_NewFields_HaveDefaults(t *testing.T) {
	// Clear any env vars that might interfere
	for _, key := range []string{
		"HOTKEY_APP_ENV", "HOTKEY_APP_NAME", "HOTKEY_APP_TIMEZONE",
		"HOTKEY_LOG_LEVEL", "HOTKEY_LOG_FORMAT",
		"HOTKEY_JWT_SECRET_KEY", "HOTKEY_JWT_SESSION_EXPIRE_DAYS",
		"HOTKEY_SCHEDULER_ENABLED",
	} {
		t.Setenv(key, "")
	}

	cfg := Load()

	if cfg.AppEnv != "local" {
		t.Errorf("AppEnv = %q, want %q", cfg.AppEnv, "local")
	}
	if cfg.AppName != "hotkey-server" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "hotkey-server")
	}
	if cfg.AppTimezone != "Asia/Shanghai" {
		t.Errorf("AppTimezone = %q, want %q", cfg.AppTimezone, "Asia/Shanghai")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
	if cfg.JWTSessionExpireDays != 7 {
		t.Errorf("JWTSessionExpireDays = %d, want %d", cfg.JWTSessionExpireDays, 7)
	}
	if cfg.SchedulerEnabled {
		t.Errorf("SchedulerEnabled = %v, want false", cfg.SchedulerEnabled)
	}
}

// --- RED: .env.test file provides complete config ---

func TestLoadFromFile_EnvTestFile_LoadsCompletely(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env.test")
	content := `HOTKEY_HTTP_ADDR=:8080
HOTKEY_DATABASE_URL=postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable
HOTKEY_REDIS_URL=redis://localhost:6379/0
HOTKEY_DASHSCOPE_API_KEY=test-key
HOTKEY_SMTP_HOST=smtp.test.com
HOTKEY_SMTP_PORT=587
HOTKEY_SMTP_USERNAME=test
HOTKEY_SMTP_PASSWORD=test
HOTKEY_SMTP_FROM=Test <test@test.com>
HOTKEY_MINIO_ENDPOINT=localhost:9000
HOTKEY_MINIO_ACCESS_KEY=minioadmin
HOTKEY_MINIO_SECRET_KEY=minioadmin
HOTKEY_X_CLIENT_ID=x-client
HOTKEY_X_CLIENT_SECRET=x-secret
HOTKEY_APP_ENV=test
HOTKEY_APP_NAME=hotkey-server-test
HOTKEY_APP_TIMEZONE=Asia/Shanghai
`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test env file: %s", err)
	}

	cfg, err := LoadFromFile(envFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %s", err)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate failed on complete config: %s", err)
	}

	caps := cfg.Capabilities()
	if !caps.SMTPEnabled {
		t.Error("expected SMTPEnabled=true with complete .env.test")
	}
	if !caps.MinIOEnabled {
		t.Error("expected MinIOEnabled=true with complete .env.test")
	}
	if !caps.DashScopeEnabled {
		t.Error("expected DashScopeEnabled=true with complete .env.test")
	}
	if !caps.XEnabled {
		t.Error("expected XEnabled=true with complete .env.test")
	}
}
