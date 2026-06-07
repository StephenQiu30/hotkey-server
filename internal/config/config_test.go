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

func TestLoadMinIOConfigDefaults(t *testing.T) {
	t.Setenv("HOTKEY_MINIO_ENDPOINT", "")
	t.Setenv("HOTKEY_MINIO_ACCESS_KEY", "")
	t.Setenv("HOTKEY_MINIO_SECRET_KEY", "")
	t.Setenv("HOTKEY_MINIO_BUCKET", "")
	t.Setenv("HOTKEY_MINIO_USE_SSL", "")
	t.Setenv("HOTKEY_MINIO_LOCATION", "")
	t.Setenv("HOTKEY_CONTENT_RETENTION_DAYS", "")

	got := Load()

	if got.MinIOEndpoint != "127.0.0.1:9000" {
		t.Fatalf("expected default MinIO endpoint, got %q", got.MinIOEndpoint)
	}
	if got.MinIOAccessKey != "" {
		t.Fatalf("expected empty MinIO access key, got %q", got.MinIOAccessKey)
	}
	if got.MinIOSecretKey != "" {
		t.Fatalf("expected empty MinIO secret key, got %q", got.MinIOSecretKey)
	}
	if got.MinIOBucket != "hotkey-content" {
		t.Fatalf("expected default MinIO bucket, got %q", got.MinIOBucket)
	}
	if got.MinIOUseSSL {
		t.Fatalf("expected MinIO SSL false, got %t", got.MinIOUseSSL)
	}
	if got.MinIOLocation != "us-east-1" {
		t.Fatalf("expected default MinIO location, got %q", got.MinIOLocation)
	}
	if got.ContentRetentionDays != 30 {
		t.Fatalf("expected default retention days 30, got %d", got.ContentRetentionDays)
	}
}

func TestLoadMinIOConfigOverrides(t *testing.T) {
	t.Setenv("HOTKEY_MINIO_ENDPOINT", "minio.example.com:9000")
	t.Setenv("HOTKEY_MINIO_ACCESS_KEY", "my-access-key")
	t.Setenv("HOTKEY_MINIO_SECRET_KEY", "my-secret-key")
	t.Setenv("HOTKEY_MINIO_BUCKET", "custom-bucket")
	t.Setenv("HOTKEY_MINIO_USE_SSL", "true")
	t.Setenv("HOTKEY_MINIO_LOCATION", "cn-hangzhou")
	t.Setenv("HOTKEY_CONTENT_RETENTION_DAYS", "90")

	got := Load()

	if got.MinIOEndpoint != "minio.example.com:9000" {
		t.Fatalf("expected MinIO endpoint override, got %q", got.MinIOEndpoint)
	}
	if got.MinIOAccessKey != "my-access-key" {
		t.Fatalf("expected MinIO access key override, got %q", got.MinIOAccessKey)
	}
	if got.MinIOSecretKey != "my-secret-key" {
		t.Fatalf("expected MinIO secret key override, got %q", got.MinIOSecretKey)
	}
	if got.MinIOBucket != "custom-bucket" {
		t.Fatalf("expected MinIO bucket override, got %q", got.MinIOBucket)
	}
	if !got.MinIOUseSSL {
		t.Fatalf("expected MinIO SSL true, got %t", got.MinIOUseSSL)
	}
	if got.MinIOLocation != "cn-hangzhou" {
		t.Fatalf("expected MinIO location override, got %q", got.MinIOLocation)
	}
	if got.ContentRetentionDays != 90 {
		t.Fatalf("expected retention days override 90, got %d", got.ContentRetentionDays)
	}
}
