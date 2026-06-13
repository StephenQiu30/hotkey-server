package database

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TestTrendRepoImplementsRepository verifies TrendRepo satisfies
// the trend.Repository interface.
func TestTrendRepoImplementsRepository(t *testing.T) {
	var _ trend.Repository = (*TrendRepo)(nil)
}

// TestTrendQueryServiceImplementsTrendQueryService verifies TrendQueryService
// satisfies the trend.TrendQueryService interface.
func TestTrendQueryServiceImplementsTrendQueryService(t *testing.T) {
	var _ trend.TrendQueryService = (*TrendQueryService)(nil)
}
