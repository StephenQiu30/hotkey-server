package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// TestTrendQueryServiceImplementsTrendQueryService checks the still-active
// query service satisfies the trend.TrendQueryService interface.
func TestTrendQueryServiceImplementsTrendQueryService(t *testing.T) {
	var _ trend.TrendQueryService = (*database.TrendQueryService)(nil)
}
