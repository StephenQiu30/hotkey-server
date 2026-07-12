package testutil

import (
	"context"
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/controller"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	"gorm.io/gorm"
)

// TestJWTSecret is a deterministic secret used across integration tests.
const TestJWTSecret = "test-jwt-secret-for-integration"
const TestVerificationCode = "123456"

type testMailer struct{}

func (testMailer) Send(context.Context, string, string, string) (string, error) {
	return "test-message", nil
}

type fixedCodeGenerator struct{}

func (fixedCodeGenerator) Generate() string { return TestVerificationCode }

// SetupTestRouter wires the real service layer against the given *gorm.DB
// and returns a fully-initialised http.Handler ready for httptest.NewServer.
func SetupTestRouter(t *testing.T, db *gorm.DB) http.Handler {
	t.Helper()

	rdb := NewTestRedis(t)
	FlushTestRedis(t, rdb)
	t.Cleanup(func() { CleanupTestRedis(t, rdb) })
	users := repository.NewUserRepo(db)
	verification := service.NewVerificationService(rdb, "integration-verification-pepper", testMailer{}, service.RealClock{}, fixedCodeGenerator{}, users)
	sessions := service.NewSessionService(repository.NewAuthSessionRepo(db), service.NewTokenManager(TestJWTSecret, "hotkey-server", "hotkey-web"))
	auth := service.NewAuthServiceV2(users, verification, sessions, testMailer{}, verification)

	return controller.NewRouter(controller.Config{
		JWTSecret:     TestJWTSecret,
		JWTIssuer:     "hotkey-server",
		JWTAudience:   "hotkey-web",
		SmokeTest:     false,
		AuthService:   auth,
		MonitorSvc:    service.NewMonitorService(repository.NewMonitorRepo(db), nil),
		NotifySvc:     service.NewNotifyService(repository.NewNotifyRepo(db)),
		ReportSvc:     service.NewReportService(repository.NewReportRepo(db), nil),
		PostQuerySvc:  repository.NewContentQueryService(db),
		TopicQuerySvc: repository.NewTopicQueryService(db),
		TrendQuerySvc: repository.NewTrendQueryService(db),
	})
}
