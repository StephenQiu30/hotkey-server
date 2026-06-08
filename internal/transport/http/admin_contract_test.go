package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAdminRevokeSourceHTTPFlow(t *testing.T) {
	adminRepo := serviceadmin.NewMemoryRepository()
	adminService := serviceadmin.NewService(adminRepo, serviceadmin.Config{})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	// Create a source
	create := postJSONWithBearer(t, router, "/api/v1/admin/sources", adminToken, map[string]any{
		"name":             "RSS To Revoke",
		"type":             "rss",
		"url":              "https://example.com/revoke.xml",
		"fetchIntervalMin": 30,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected source create 201, got %d with body %s", create.Code, create.Body.String())
	}
	sourceID := jsonStringAt(t, create.Body.Bytes(), "source.id")

	// Revoke the source
	revoke := postJSONWithBearer(t, router, "/api/v1/admin/sources/"+sourceID+"/revoke", adminToken, nil)
	if revoke.Code != http.StatusOK {
		t.Fatalf("expected revoke 200, got %d with body %s", revoke.Code, revoke.Body.String())
	}
	assertJSONField(t, revoke.Body.Bytes(), "source.status", "revoked")

	// Check audit log recorded
	auditLogs := getWithBearer(router, "/api/v1/admin/audit-logs", adminToken)
	if auditLogs.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d with body %s", auditLogs.Code, auditLogs.Body.String())
	}
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.0.resourceType", "source")
}

func TestAdminDeleteAccountHTTPFlow(t *testing.T) {
	adminRepo := serviceadmin.NewMemoryRepository()
	adminRepo.SetUser("usr_del_http", serviceadmin.UserRecord{ID: "usr_del_http", Email: "delete-target@example.com"})
	adminService := serviceadmin.NewService(adminRepo, serviceadmin.Config{})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	// Delete the account
	deleteResp := deleteWithBearer(router, "/api/v1/admin/users/usr_del_http", adminToken)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d with body %s", deleteResp.Code, deleteResp.Body.String())
	}
	assertJSONField(t, deleteResp.Body.Bytes(), "cleanupTask.status", "completed")

	cleanupID := jsonStringAt(t, deleteResp.Body.Bytes(), "cleanupTask.id")
	if cleanupID == "" {
		t.Fatalf("expected cleanup task id in response: %s", deleteResp.Body.String())
	}

	// Check cleanup status
	statusResp := getWithBearer(router, "/api/v1/admin/cleanup-tasks/"+cleanupID, adminToken)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected cleanup status 200, got %d with body %s", statusResp.Code, statusResp.Body.String())
	}
	assertJSONField(t, statusResp.Body.Bytes(), "cleanupTask.status", "completed")
}

func TestAdminObservabilityEndpointsReturn403ForNonAdmin(t *testing.T) {
	adminService := serviceadmin.NewService(serviceadmin.NewMemoryRepository(), serviceadmin.Config{
		PostgreSQLPing: func(ctx context.Context) error { return nil },
		RedisPing:      func(ctx context.Context) error { return nil },
	})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	userToken := registerAndLogin(t, router, "non-admin-obs@example.com")

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/config/status"},
		{http.MethodGet, "/api/v1/admin/audit-logs"},
		{http.MethodGet, "/api/v1/admin/jobs"},
		{http.MethodGet, "/api/v1/admin/jobs/failed"},
		{http.MethodGet, "/api/v1/admin/sources"},
	}
	for _, ep := range endpoints {
		var resp *httptest.ResponseRecorder
		switch ep.method {
		case http.MethodGet:
			resp = getWithBearer(router, ep.path, userToken)
		default:
			resp = getWithBearer(router, ep.path, userToken)
		}
		if resp.Code != http.StatusForbidden {
			t.Errorf("expected 403 for %s %s with user token, got %d: %s", ep.method, ep.path, resp.Code, resp.Body.String())
		}
	}
}

func TestAdminConfigStatusDoesNotLeakSecrets(t *testing.T) {
	realAPIKey := "sk-real-dashscope-key-12345"
	realSMTPHost := "smtp.example.com"

	adminService := serviceadmin.NewService(serviceadmin.NewMemoryRepository(), serviceadmin.Config{
		PostgreSQLPing: func(ctx context.Context) error { return nil },
		RedisPing:      func(ctx context.Context) error { return nil },
		DashScopeKey:   realAPIKey,
		SMTPHost:       realSMTPHost,
		MinIOPing:      func(ctx context.Context) error { return nil },
		MinIOEndpoint:  "minio.example.com:9000",
	})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	adminToken := registerAdminAndLogin(t, router, "admin-secret-check@example.com")

	resp := getWithBearer(router, "/api/v1/admin/config/status", adminToken)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected config status 200, got %d: %s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	if strings.Contains(body, realAPIKey) {
		t.Errorf("config status response must not contain real DashScope API key, got: %s", body)
	}

	// Verify the response structure only contains status/reason, not secret values
	var parsed map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	status, ok := parsed["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected status object in response: %s", body)
	}
	components, ok := status["components"].(map[string]any)
	if !ok {
		t.Fatalf("expected components object in response: %s", body)
	}
	for name, comp := range components {
		compMap, ok := comp.(map[string]any)
		if !ok {
			t.Errorf("expected component %s to be object, got %T", name, comp)
			continue
		}
		// Only "status" and "reason" fields are allowed — no secret values
		for key := range compMap {
			if key != "status" && key != "reason" {
				t.Errorf("component %s has unexpected field %q — must not expose config values", name, key)
			}
		}
	}
}

func TestAdminCleanupRetryHTTPFlow(t *testing.T) {
	adminRepo := serviceadmin.NewMemoryRepository()
	// Set up a user but inject an error for daily report deletion
	adminRepo.SetUser("usr_retry", serviceadmin.UserRecord{ID: "usr_retry", Email: "retry@example.com"})
	adminRepo.SetDeleteReportError(errors.New("db connection lost"))

	adminService := serviceadmin.NewService(adminRepo, serviceadmin.Config{})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	// Delete should fail on first step
	deleteResp := deleteWithBearer(router, "/api/v1/admin/users/usr_retry", adminToken)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d with body %s", deleteResp.Code, deleteResp.Body.String())
	}
	assertJSONField(t, deleteResp.Body.Bytes(), "cleanupTask.status", "failed")
	cleanupID := jsonStringAt(t, deleteResp.Body.Bytes(), "cleanupTask.id")

	// Fix the error and retry
	adminRepo.SetDeleteReportError(nil)
	retryResp := postJSONWithBearer(t, router, "/api/v1/admin/cleanup-tasks/"+cleanupID+"/retry", adminToken, nil)
	if retryResp.Code != http.StatusOK {
		t.Fatalf("expected retry 200, got %d with body %s", retryResp.Code, retryResp.Body.String())
	}
	assertJSONField(t, retryResp.Body.Bytes(), "cleanupTask.status", "completed")
}
