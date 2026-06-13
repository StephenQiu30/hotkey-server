package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/database"
)

// TestContentRepoImplementsPostRepository verifies ContentRepo satisfies
// the content.PostRepository interface.
func TestContentRepoImplementsPostRepository(t *testing.T) {
	var _ content.PostRepository = (*database.ContentRepo)(nil)
}

// TestContentRepoImplementsHitRepository verifies ContentRepo satisfies
// the content.HitRepository interface.
func TestContentRepoImplementsHitRepository(t *testing.T) {
	var _ content.HitRepository = (*database.ContentRepo)(nil)
}

// TestContentQueryServiceImplementsPostQueryService verifies ContentQueryService
// satisfies the content.PostQueryService interface.
func TestContentQueryServiceImplementsPostQueryService(t *testing.T) {
	var _ content.PostQueryService = (*database.ContentQueryService)(nil)
}
