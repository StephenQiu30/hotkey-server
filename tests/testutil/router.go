package testutil

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/server"
)

// TestJWTSecret is a deterministic secret used across integration tests.
// Exported so that test helpers in other packages can mint valid tokens.
const TestJWTSecret = "test-jwt-secret-for-integration"

// SetupTestRouter wires the real service layer against the given *sql.DB
// and returns a fully-initialised http.Handler ready for httptest.NewServer.
func SetupTestRouter(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()

	// Auth
	authRepo := database.NewAuthRepo(db)
	authSvc := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authSvc, TestJWTSecret)

	// Monitor
	monitorRepo := database.NewMonitorRepo(db)
	monitorSvc := monitor.NewService(monitorRepo)
	monitorHandler := monitor.NewHandler(monitorSvc)

	// Notification
	notifyRepo := database.NewNotifyRepo(db)
	notifySvc := notify.NewService(notifyRepo)
	notifyHandler := notify.NewHandler(notifySvc)

	// Middleware
	authMiddleware := server.AuthMiddleware(TestJWTSecret)

	return server.NewRouter(server.Dependencies{
		AuthHandler:         authHandler,
		MonitorHandler:      monitorHandler,
		TopicHandler:        nil,
		TrendHandler:        nil,
		PostHandler:         nil,
		NotificationHandler: notifyHandler,
		AuthMiddleware:      authMiddleware,
	})
}
