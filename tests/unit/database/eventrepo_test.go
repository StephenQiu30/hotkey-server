package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
)

func TestEventRepoImplementsInterface(t *testing.T) {
	var _ interface{ CreateEvent } = (*database.EventRepo)(nil)
}
