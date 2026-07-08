package testutil

import (
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/controller"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	"gorm.io/gorm"
)

// TestJWTSecret is a deterministic secret used across integration tests.
const TestJWTSecret = "test-jwt-secret-for-integration"

// SetupTestRouter wires the real service layer against the given *gorm.DB
// and returns a fully-initialised http.Handler ready for httptest.NewServer.
func SetupTestRouter(t *testing.T, db *gorm.DB) http.Handler {
	t.Helper()

	return controller.NewRouter(controller.Config{
		JWTSecret:     TestJWTSecret,
		SmokeTest:     false,
		AuthService:   service.NewAuthService(repository.NewUserRepo(db)),
		MonitorSvc:    service.NewMonitorService(repository.NewMonitorRepo(db), nil),
		NotifySvc:     service.NewNotifyService(repository.NewNotifyRepo(db)),
		ReportSvc:     service.NewReportService(repository.NewReportRepo(db), nil),
		PostQuerySvc:  repository.NewContentQueryService(db),
		TopicQuerySvc: repository.NewTopicQueryService(db),
		TrendQuerySvc: repository.NewTrendQueryService(db),
	})
}
