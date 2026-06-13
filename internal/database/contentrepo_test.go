package database

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// TestContentRepoImplementsPostRepository verifies ContentRepo satisfies
// the content.PostRepository interface.
func TestContentRepoImplementsPostRepository(t *testing.T) {
	var _ content.PostRepository = (*ContentRepo)(nil)
}

// TestContentRepoImplementsHitRepository verifies ContentRepo satisfies
// the content.HitRepository interface.
func TestContentRepoImplementsHitRepository(t *testing.T) {
	var _ content.HitRepository = (*ContentRepo)(nil)
}

// TestContentQueryServiceImplementsPostQueryService verifies ContentQueryService
// satisfies the content.PostQueryService interface.
func TestContentQueryServiceImplementsPostQueryService(t *testing.T) {
	var _ content.PostQueryService = (*ContentQueryService)(nil)
}
