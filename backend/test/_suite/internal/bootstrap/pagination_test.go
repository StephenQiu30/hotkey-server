package bootstrap

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
)

func TestPaginationCodecUsesStableConfiguredSecretAndRejectsAnotherRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Authentication.JWTSecret = "pagination-production-secret-32-bytes-a"
	first, err := newPaginationCodec(cfg)
	if err != nil {
		t.Fatalf("newPaginationCodec(first): %v", err)
	}
	second, err := newPaginationCodec(cfg)
	if err != nil {
		t.Fatalf("newPaginationCodec(second): %v", err)
	}
	encoded, err := first.Encode("id", false, "sources", 42)
	if err != nil {
		t.Fatal(err)
	}
	if cursor, err := second.Decode(encoded, "id", false, "sources"); err != nil || cursor.ID != 42 {
		t.Fatalf("stable configured cursor = %#v / %v", cursor, err)
	}

	other := cfg
	other.Authentication.JWTSecret = "pagination-production-secret-32-bytes-b"
	third, err := newPaginationCodec(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := third.Decode(encoded, "id", false, "sources"); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-runtime cursor error = %v", err)
	}
}

func TestPaginationCodecFallsBackOnlyForWorkerDatabaseRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Authentication.JWTSecret = "short-worker-secret"
	cfg.DatabaseURL = "postgres://hotkey:strong-database-password@db.example.test:5432/hotkey?sslmode=require"
	if _, err := newPaginationCodec(cfg); err != nil {
		t.Fatalf("worker database cursor codec: %v", err)
	}
	cfg.DatabaseURL = "short"
	if _, err := newPaginationCodec(cfg); err == nil {
		t.Fatal("pagination codec accepted short JWT and database secrets")
	}
}
