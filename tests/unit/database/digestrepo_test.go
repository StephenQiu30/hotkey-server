package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/digest"
)

// TestDigestRepoImplementsRepository verifies DigestRepo satisfies
// the digest.Repository interface.
func TestDigestRepoImplementsRepository(t *testing.T) {
	var _ digest.Repository = (*database.DigestRepo)(nil)
}
