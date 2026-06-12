package config

import (
	"testing"
)

func TestLoadConfigFailsWhenDatabaseURLMissing(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
}
