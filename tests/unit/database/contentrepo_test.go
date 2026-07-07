package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// TestContentQueryServiceImplementsPostQueryService checks the still-active
// query service satisfies the content.PostQueryService interface.
func TestContentQueryServiceImplementsPostQueryService(t *testing.T) {
	var _ content.PostQueryService = (*database.ContentQueryService)(nil)
}
