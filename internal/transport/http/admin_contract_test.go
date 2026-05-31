package http_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestAdminObservabilityHTTPFlow(t *testing.T) {
	adminService := serviceadmin.NewService(serviceadmin.NewMemoryRepository(), serviceadmin.Config{
		PostgreSQLPing: func(ctx context.Context) error { return nil },
		RedisPing:      func(ctx context.Context) error { return errors.New("connection refused") },
	})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	userToken := registerAndLogin(t, router, "admin-observability-user@example.com")
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	denied := getWithBearer(router, "/api/v1/admin/config/status", userToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected user role to get 403 for admin config status, got %d with body %s", denied.Code, denied.Body.String())
	}

	create := postJSONWithBearer(t, router, "/api/v1/admin/sources", adminToken, map[string]any{
		"name":             "Admin RSS",
		"type":             "rss",
		"url":              "https://example.com/admin.xml",
		"fetchIntervalMin": 30,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected admin source create 201, got %d with body %s", create.Code, create.Body.String())
	}

	auditLogs := getWithBearer(router, "/api/v1/admin/audit-logs", adminToken)
	if auditLogs.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d with body %s", auditLogs.Code, auditLogs.Body.String())
	}
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.0.actorId", "usr_admin")
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.0.resourceType", "source")
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.0.resourceId", jsonStringAt(t, create.Body.Bytes(), "source.id"))
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.0.result", "success")

	configStatus := getWithBearer(router, "/api/v1/admin/config/status", adminToken)
	if configStatus.Code != http.StatusOK {
		t.Fatalf("expected config status 200, got %d with body %s", configStatus.Code, configStatus.Body.String())
	}
	assertJSONField(t, configStatus.Body.Bytes(), "status.overall", "degraded")
	assertJSONField(t, configStatus.Body.Bytes(), "status.components.redis.status", "degraded")
	assertJSONField(t, configStatus.Body.Bytes(), "status.components.dashscope.reason", "missing_config")
	assertJSONField(t, configStatus.Body.Bytes(), "status.components.smtp.reason", "missing_config")

	rerun := postJSONWithBearer(t, router, "/api/v1/admin/daily-reports/rerun", adminToken, map[string]any{
		"date":      "2026-05-31",
		"channelId": "chn_ai",
		"userId":    "usr_admin",
	})
	if rerun.Code != http.StatusAccepted {
		t.Fatalf("expected daily report rerun 202, got %d with body %s", rerun.Code, rerun.Body.String())
	}
	jobID := jsonStringAt(t, rerun.Body.Bytes(), "job.id")
	if jobID == "" {
		t.Fatalf("expected rerun job id in response: %s", rerun.Body.String())
	}

	failedJobs := getWithBearer(router, "/api/v1/admin/jobs/failed", adminToken)
	if failedJobs.Code != http.StatusOK {
		t.Fatalf("expected failed jobs 200, got %d with body %s", failedJobs.Code, failedJobs.Body.String())
	}
}
