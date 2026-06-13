package database

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// TestTopicRepoImplementsRepository verifies TopicRepo satisfies
// the topic.Repository interface.
func TestTopicRepoImplementsRepository(t *testing.T) {
	var _ topic.Repository = (*TopicRepo)(nil)
}

// TestTopicQueryServiceImplementsTopicQueryService verifies TopicQueryService
// satisfies the topic.TopicQueryService interface.
func TestTopicQueryServiceImplementsTopicQueryService(t *testing.T) {
	var _ topic.TopicQueryService = (*TopicQueryService)(nil)
}
