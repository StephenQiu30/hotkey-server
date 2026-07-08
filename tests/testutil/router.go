package testutil

import (
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"gorm.io/gorm"
)

// TestJWTSecret is a deterministic secret used across integration tests.
const TestJWTSecret = "test-jwt-secret-for-integration"

// SetupTestRouter wires the real service layer against the given *gorm.DB
// and returns a fully-initialised http.Handler ready for httptest.NewServer.
func SetupTestRouter(t *testing.T, db *gorm.DB) http.Handler {
	t.Helper()

	return platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     TestJWTSecret,
		SmokeTest:     false,
		AuthService:   auth.NewService(repository.NewUserRepo(db)),
		MonitorSvc:    monitor.NewService(repository.NewMonitorRepo(db), nil),
		NotifySvc:     notify.NewService(repository.NewNotifyRepo(db)),
		ReportSvc:     report.NewService(repository.NewReportRepo(db), nil),
		PostQuerySvc:  repository.NewContentQueryService(db),
		TopicQuerySvc: repository.NewTopicQueryService(db),
		TrendQuerySvc: repository.NewTrendQueryService(db),
	})
}
