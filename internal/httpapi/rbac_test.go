package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/rbac"
	"github.com/gin-gonic/gin"
)

func TestRBACEndpointsGrantAuthorizeAndAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	grantReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-alpha/roles", bytes.NewBufferString(`{"userId":"admin-1","role":"admin"}`))
	grantReq.Header.Set("Content-Type", "application/json")
	grantRec := httptest.NewRecorder()
	router.ServeHTTP(grantRec, grantReq)
	if grantRec.Code != http.StatusCreated {
		t.Fatalf("grant status = %d, want %d; body=%s", grantRec.Code, http.StatusCreated, grantRec.Body.String())
	}

	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/tenant-alpha/authorize", bytes.NewBufferString(`{"userId":"admin-1","action":"manage_sources"}`))
	authReq.Header.Set("Content-Type", "application/json")
	authRec := httptest.NewRecorder()
	router.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("authorize status = %d, want %d; body=%s", authRec.Code, http.StatusOK, authRec.Body.String())
	}
	var authBody struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal(authRec.Body.Bytes(), &authBody); err != nil {
		t.Fatalf("decode authorize: %v", err)
	}
	if !authBody.Allowed {
		t.Fatalf("admin should be allowed to manage sources")
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tenants/tenant-alpha/audit-logs", nil)
	auditRec := httptest.NewRecorder()
	router.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want %d; body=%s", auditRec.Code, http.StatusOK, auditRec.Body.String())
	}
	var auditBody struct {
		Events []rbac.AuditEvent `json:"events"`
	}
	if err := json.Unmarshal(auditRec.Body.Bytes(), &auditBody); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	if len(auditBody.Events) == 0 {
		t.Fatalf("audit logs empty after role grant")
	}
}
