package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// TestTopicQueryServiceImplementsTopicQueryService checks the still-active
// query service satisfies the service.TopicQueryService interface.
func TestTopicQueryServiceImplementsTopicQueryService(t *testing.T) {
	var _ service.TopicQueryService = (*repository.TopicQueryService)(nil)
}
