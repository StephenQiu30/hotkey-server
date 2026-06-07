package config

import (
	"testing"
	"time"
)

func TestLoadRuntimeMode(t *testing.T) {
	tests := []struct {
		env  string
		want RuntimeMode
	}{
		{"", RuntimeModeAll},
		{"all", RuntimeModeAll},
		{"api", RuntimeModeAPI},
		{"worker", RuntimeModeWorker},
		{"bogus", RuntimeModeAll},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("HOTKEY_RUNTIME_MODE", tt.env)
			got := Load()
			if got.RuntimeMode != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got.RuntimeMode)
			}
		})
	}
}

func TestLoadEmbeddingAndHotspotDefaults(t *testing.T) {
	t.Setenv("HOTKEY_DASHSCOPE_API_KEY", "")
	t.Setenv("HOTKEY_EMBEDDING_MODEL", "")
	t.Setenv("HOTKEY_HOTSPOT_SIMILARITY_THRESHOLD", "")
	t.Setenv("HOTKEY_HOTSPOT_WINDOW", "")

	got := Load()

	if got.DashScopeAPIKey != "" {
		t.Fatalf("expected empty DashScope API key, got %q", got.DashScopeAPIKey)
	}
	if got.EmbeddingModel != "text-embedding-v2" {
		t.Fatalf("expected default embedding model, got %q", got.EmbeddingModel)
	}
	if got.HotspotSimilarityThreshold != 0.82 {
		t.Fatalf("expected default hotspot threshold, got %f", got.HotspotSimilarityThreshold)
	}
	if got.HotspotWindow != 24*time.Hour {
		t.Fatalf("expected default hotspot window, got %s", got.HotspotWindow)
	}
}

func TestLoadEmbeddingAndHotspotOverrides(t *testing.T) {
	t.Setenv("HOTKEY_DASHSCOPE_API_KEY", "dashscope-key")
	t.Setenv("HOTKEY_EMBEDDING_MODEL", "custom-model")
	t.Setenv("HOTKEY_HOTSPOT_SIMILARITY_THRESHOLD", "0.91")
	t.Setenv("HOTKEY_HOTSPOT_WINDOW", "12h")

	got := Load()

	if got.DashScopeAPIKey != "dashscope-key" || got.EmbeddingModel != "custom-model" {
		t.Fatalf("unexpected embedding config: %+v", got)
	}
	if got.HotspotSimilarityThreshold != 0.91 {
		t.Fatalf("unexpected threshold: %f", got.HotspotSimilarityThreshold)
	}
	if got.HotspotWindow != 12*time.Hour {
		t.Fatalf("unexpected window: %s", got.HotspotWindow)
	}
}

func TestLoadSMTPConfig(t *testing.T) {
	t.Setenv("HOTKEY_SMTP_HOST", "smtp.example.com")
	t.Setenv("HOTKEY_SMTP_PORT", "465")
	t.Setenv("HOTKEY_SMTP_USERNAME", "daily")
	t.Setenv("HOTKEY_SMTP_PASSWORD", "secret")
	t.Setenv("HOTKEY_SMTP_FROM", "HotKey <daily@example.com>")
	t.Setenv("HOTKEY_SMTP_TLS", "true")
	t.Setenv("HOTKEY_SMTP_STARTTLS", "false")

	got := Load()

	if got.SMTPHost != "smtp.example.com" || got.SMTPPort != 465 {
		t.Fatalf("unexpected SMTP endpoint config: host=%q port=%d", got.SMTPHost, got.SMTPPort)
	}
	if got.SMTPUsername != "daily" || got.SMTPPassword != "secret" || got.SMTPFrom != "HotKey <daily@example.com>" {
		t.Fatalf("unexpected SMTP auth/from config: username=%q from=%q", got.SMTPUsername, got.SMTPFrom)
	}
	if !got.SMTPTLS || got.SMTPStartTLS {
		t.Fatalf("unexpected SMTP TLS config: tls=%t starttls=%t", got.SMTPTLS, got.SMTPStartTLS)
	}
}

func TestLoadSMTPPortFallsBackWhenOutOfRange(t *testing.T) {
	for _, value := range []string{"0", "-1", "70000", "not-a-port"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("HOTKEY_SMTP_PORT", value)

			got := Load()

			if got.SMTPPort != 587 {
				t.Fatalf("expected default SMTP port for %q, got %d", value, got.SMTPPort)
			}
		})
	}
}

func TestLoadMinIODefaults(t *testing.T) {
	t.Setenv("HOTKEY_MINIO_ENDPOINT", "")
	t.Setenv("HOTKEY_MINIO_ACCESS_KEY", "")
	t.Setenv("HOTKEY_MINIO_SECRET_KEY", "")
	t.Setenv("HOTKEY_MINIO_BUCKET", "")
	t.Setenv("HOTKEY_MINIO_USE_SSL", "")
	t.Setenv("HOTKEY_MINIO_LOCATION", "")
	t.Setenv("HOTKEY_CONTENT_RETENTION_DAYS", "")

	got := Load()

	if got.MinIOEndpoint != "localhost:9000" {
		t.Errorf("MinIOEndpoint = %q, want %q", got.MinIOEndpoint, "localhost:9000")
	}
	if got.MinIOAccessKey != "" {
		t.Errorf("MinIOAccessKey = %q, want empty (no weak default)", got.MinIOAccessKey)
	}
	if got.MinIOSecretKey != "" {
		t.Errorf("MinIOSecretKey = %q, want empty (no weak default)", got.MinIOSecretKey)
	}
	if got.MinIOBucket != "hotkey-snapshots" {
		t.Errorf("MinIOBucket = %q, want %q", got.MinIOBucket, "hotkey-snapshots")
	}
	if got.MinIOUseSSL {
		t.Errorf("MinIOUseSSL = %v, want false", got.MinIOUseSSL)
	}
	if got.MinIOLocation != "us-east-1" {
		t.Errorf("MinIOLocation = %q, want %q", got.MinIOLocation, "us-east-1")
	}
	if got.ContentRetentionDays != 30 {
		t.Errorf("ContentRetentionDays = %d, want 30", got.ContentRetentionDays)
	}
}

func TestLoadMinIOOverrides(t *testing.T) {
	t.Setenv("HOTKEY_MINIO_ENDPOINT", "minio.example.com:443")
	t.Setenv("HOTKEY_MINIO_ACCESS_KEY", "mykey")
	t.Setenv("HOTKEY_MINIO_SECRET_KEY", "mysecret")
	t.Setenv("HOTKEY_MINIO_BUCKET", "my-bucket")
	t.Setenv("HOTKEY_MINIO_USE_SSL", "true")
	t.Setenv("HOTKEY_MINIO_LOCATION", "eu-west-1")
	t.Setenv("HOTKEY_CONTENT_RETENTION_DAYS", "90")

	got := Load()

	if got.MinIOEndpoint != "minio.example.com:443" {
		t.Errorf("MinIOEndpoint = %q, want %q", got.MinIOEndpoint, "minio.example.com:443")
	}
	if got.MinIOAccessKey != "mykey" {
		t.Errorf("MinIOAccessKey = %q, want %q", got.MinIOAccessKey, "mykey")
	}
	if got.MinIOSecretKey != "mysecret" {
		t.Errorf("MinIOSecretKey = %q, want %q", got.MinIOSecretKey, "mysecret")
	}
	if got.MinIOBucket != "my-bucket" {
		t.Errorf("MinIOBucket = %q, want %q", got.MinIOBucket, "my-bucket")
	}
	if !got.MinIOUseSSL {
		t.Errorf("MinIOUseSSL = %v, want true", got.MinIOUseSSL)
	}
	if got.MinIOLocation != "eu-west-1" {
		t.Errorf("MinIOLocation = %q, want %q", got.MinIOLocation, "eu-west-1")
	}
	if got.ContentRetentionDays != 90 {
		t.Errorf("ContentRetentionDays = %d, want 90", got.ContentRetentionDays)
	}
}

func TestLoadMinIORetentionDaysFallsBackWhenInvalid(t *testing.T) {
	for _, value := range []string{"-1", "not-a-number"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("HOTKEY_CONTENT_RETENTION_DAYS", value)
			got := Load()
			if got.ContentRetentionDays != 30 {
				t.Fatalf("expected default retention days for %q, got %d", value, got.ContentRetentionDays)
			}
		})
	}
}

func TestLoadMinIORetentionDaysAllowsZero(t *testing.T) {
	t.Setenv("HOTKEY_CONTENT_RETENTION_DAYS", "0")
	got := Load()
	if got.ContentRetentionDays != 0 {
		t.Fatalf("expected 0 for immediate deletion, got %d", got.ContentRetentionDays)
	}
}

func TestLoadMinIORetentionDaysAllowsLargeValues(t *testing.T) {
	t.Setenv("HOTKEY_CONTENT_RETENTION_DAYS", "70000")
	got := Load()
	if got.ContentRetentionDays != 70000 {
		t.Fatalf("expected 70000 for long-term archival, got %d", got.ContentRetentionDays)
	}
}
