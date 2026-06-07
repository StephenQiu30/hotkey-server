package http_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
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
	sourceRepo := servicesource.NewMemoryRepository()
	sourceService := servicesource.NewService(sourceRepo)
	adminRepo := serviceadmin.NewMemoryRepository()
	adminService := serviceadmin.NewService(adminRepo, serviceadmin.Config{
		RevokeSourceFunc: func(ctx context.Context, sourceID string) (serviceadmin.RevokedSource, error) {
			src, err := sourceService.RevokeSource(ctx, sourceID)
			if err != nil {
				return serviceadmin.RevokedSource{}, err
			}
			return serviceadmin.RevokedSource{
				ID:        src.ID,
				Name:      src.Name,
				Status:    string(src.Status),
				UpdatedAt: src.UpdatedAt,
			}, nil
		},
	})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		SourceService: sourceService,
		AdminService:  adminService,
	})
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	// Create a source first.
	create := postJSONWithBearer(t, router, "/api/v1/admin/sources", adminToken, map[string]any{
		"name":             "Revoke RSS",
		"type":             "rss",
		"url":              "https://example.com/revoke.xml",
		"fetchIntervalMin": 30,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected source create 201, got %d with body %s", create.Code, create.Body.String())
	}
	sourceID := jsonStringAt(t, create.Body.Bytes(), "source.id")

	// Non-admin cannot revoke.
	userToken := registerAndLogin(t, router, "revoke-user@example.com")
	denied := patchJSONWithBearer(t, router, "/api/v1/admin/sources/"+sourceID+"/revoke", userToken, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected user revoke 403, got %d with body %s", denied.Code, denied.Body.String())
	}

	// Admin revokes source.
	revoke := patchJSONWithBearer(t, router, "/api/v1/admin/sources/"+sourceID+"/revoke", adminToken, nil)
	if revoke.Code != http.StatusOK {
		t.Fatalf("expected revoke 200, got %d with body %s", revoke.Code, revoke.Body.String())
	}
	assertJSONField(t, revoke.Body.Bytes(), "source.status", "revoked")
	assertJSONField(t, revoke.Body.Bytes(), "source.id", sourceID)

	// Revoked source still appears in full list.
	list := getWithBearer(router, "/api/v1/admin/sources", adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected sources list 200, got %d with body %s", list.Code, list.Body.String())
	}

	// Audit log records the revoke.
	auditLogs := getWithBearer(router, "/api/v1/admin/audit-logs", adminToken)
	if auditLogs.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d with body %s", auditLogs.Code, auditLogs.Body.String())
	}
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.1.resourceType", "source")
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.1.action", "update")
}

func TestAdminDeleteAccountAndCleanupHTTPFlow(t *testing.T) {
	adminRepo := serviceadmin.NewMemoryRepository()
	adminRepo.SetUser("usr_victim", serviceadmin.UserRecord{
		ID:           "usr_victim",
		Email:        "victim@example.com",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		Role:         "user",
		Status:       "active",
	})
	adminService := serviceadmin.NewService(adminRepo, serviceadmin.Config{})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	// Non-admin cannot delete account.
	userToken := registerAndLogin(t, router, "delete-user@example.com")
	denied := deleteWithBearer(router, "/api/v1/admin/users/usr_victim", userToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected user delete 403, got %d with body %s", denied.Code, denied.Body.String())
	}

	// Admin deletes account.
	del := deleteWithBearer(router, "/api/v1/admin/users/usr_victim", adminToken)
	if del.Code != http.StatusAccepted {
		t.Fatalf("expected delete 202, got %d with body %s", del.Code, del.Body.String())
	}
	taskID := jsonStringAt(t, del.Body.Bytes(), "cleanupTask.id")
	if taskID == "" {
		t.Fatalf("expected cleanup task id in response: %s", del.Body.String())
	}
	assertJSONField(t, del.Body.Bytes(), "cleanupTask.status", "completed")
	assertJSONField(t, del.Body.Bytes(), "cleanupTask.userId", "usr_victim")

	// Cleanup status is queryable.
	status := getWithBearer(router, "/api/v1/admin/cleanup-tasks/"+taskID, adminToken)
	if status.Code != http.StatusOK {
		t.Fatalf("expected cleanup status 200, got %d with body %s", status.Code, status.Body.String())
	}
	assertJSONField(t, status.Body.Bytes(), "cleanupTask.id", taskID)
	assertJSONField(t, status.Body.Bytes(), "cleanupTask.status", "completed")

	// Delete nonexistent user returns 404.
	notFound := deleteWithBearer(router, "/api/v1/admin/users/nonexistent", adminToken)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected delete nonexistent 404, got %d with body %s", notFound.Code, notFound.Body.String())
	}

	// Audit log records the delete.
	auditLogs := getWithBearer(router, "/api/v1/admin/audit-logs", adminToken)
	if auditLogs.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d with body %s", auditLogs.Code, auditLogs.Body.String())
	}
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.0.resourceType", "user")
	assertJSONField(t, auditLogs.Body.Bytes(), "auditLogs.0.action", "delete")
}

func TestAdminCleanupRetryHTTPFlow(t *testing.T) {
	adminRepo := serviceadmin.NewMemoryRepository()
	adminRepo.SetUser("usr_retry", serviceadmin.UserRecord{
		ID:           "usr_retry",
		Email:        "retry@example.com",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		Role:         "user",
		Status:       "active",
	})
	adminRepo.SetDeleteReportError(errors.New("database timeout"))
	adminService := serviceadmin.NewService(adminRepo, serviceadmin.Config{})
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		AdminService: adminService,
	})
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	// Delete fails due to injected error.
	del := deleteWithBearer(router, "/api/v1/admin/users/usr_retry", adminToken)
	if del.Code != http.StatusAccepted {
		t.Fatalf("expected delete 202, got %d with body %s", del.Code, del.Body.String())
	}
	taskID := jsonStringAt(t, del.Body.Bytes(), "cleanupTask.id")
	assertJSONField(t, del.Body.Bytes(), "cleanupTask.status", "failed")

	// Fix the error and retry.
	adminRepo.SetDeleteReportError(nil)
	retry := postJSONWithBearer(t, router, "/api/v1/admin/cleanup-tasks/"+taskID+"/retry", adminToken, nil)
	if retry.Code != http.StatusAccepted {
		t.Fatalf("expected retry 202, got %d with body %s", retry.Code, retry.Body.String())
	}
	assertJSONField(t, retry.Body.Bytes(), "cleanupTask.status", "completed")

	// Retry on nonexistent task returns 404.
	notFound := postJSONWithBearer(t, router, "/api/v1/admin/cleanup-tasks/nonexistent/retry", adminToken, nil)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected retry nonexistent 404, got %d with body %s", notFound.Code, notFound.Body.String())
	}
}
