package database_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// TestTrendQueryServiceImplementsTrendQueryService checks the still-active
// query service satisfies the service.TrendQueryService interface.
func TestTrendQueryServiceImplementsTrendQueryService(t *testing.T) {
	var _ service.TrendQueryService = (*repository.TrendQueryService)(nil)
}
