package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// TestTopicRepoImplementsRepository verifies TopicRepo satisfies
// the topic.Repository interface.
func TestTopicRepoImplementsRepository(t *testing.T) {
	var _ topic.Repository = (*database.TopicRepo)(nil)
}

// TestTopicQueryServiceImplementsTopicQueryService verifies TopicQueryService
// satisfies the topic.TopicQueryService interface.
func TestTopicQueryServiceImplementsTopicQueryService(t *testing.T) {
	var _ topic.TopicQueryService = (*database.TopicQueryService)(nil)
}
