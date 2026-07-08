package config_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

func TestLoadConfigFailsWhenDatabaseURLMissing(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}

func TestLoadConfigFailsWhenJWTSecretMissing(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/testdb")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when JWT_SECRET is missing")
	}
}

func TestLoadConfigSuccess(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/testdb")
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("expected HTTP_ADDR :9090, got %s", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://localhost:5432/testdb" {
		t.Fatalf("expected DATABASE_URL, got %s", cfg.DatabaseURL)
	}
}

func TestKafkaDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("X_BEARER_TOKEN", "test-token")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.KafkaBrokers) == 0 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Fatalf("KafkaBrokers = %v, want [localhost:9092]", cfg.KafkaBrokers)
	}
	if cfg.KafkaConsumerGroup != "hotkey-workers" {
		t.Fatalf("KafkaConsumerGroup = %q, want hotkey-workers", cfg.KafkaConsumerGroup)
	}
}
