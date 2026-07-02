package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
)

func TestNewEventRepo(t *testing.T) {
	repo := database.NewEventRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil EventRepo")
	}
}
