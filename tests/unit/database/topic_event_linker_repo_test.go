package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
)

func TestNewTopicEventLinkerRepo(t *testing.T) {
	repo := database.NewTopicEventLinkerRepo(nil)
	if repo == nil {
		t.Fatal("expected non-nil TopicEventLinkerRepo")
	}
}
