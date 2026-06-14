package database_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
)

func TestEnsureReadyInvalidDatabaseURL(t *testing.T) {
	t.Parallel()

	err := database.EnsureReady(context.Background(), "://invalid")
	if err == nil {
		t.Fatal("expected error for invalid database url")
	}
}

func TestEnsureReadyMissingDatabaseName(t *testing.T) {
	t.Parallel()

	err := database.EnsureReady(context.Background(), "postgres://localhost:5432/?sslmode=disable")
	if err == nil {
		t.Fatal("expected error for missing database name")
	}
}
