package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// TestTopicQueryServiceImplementsTopicQueryService checks the still-active
// query service satisfies the topic.TopicQueryService interface.
func TestTopicQueryServiceImplementsTopicQueryService(t *testing.T) {
	var _ topic.TopicQueryService = (*repository.TopicQueryService)(nil)
}
