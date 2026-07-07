package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
)

// TestNewEventRepo verifies the gormimpl EventRepo constructor.
func TestNewEventRepo(t *testing.T) {
	repo := gormimpl.NewEventRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil EventRepo")
	}
}
