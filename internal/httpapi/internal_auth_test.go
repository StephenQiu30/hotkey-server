package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalAPIMiddlewareRejectsMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", nil)
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object in response")
	}
	if errObj["code"] != "unauthorized" {
		t.Fatalf("error code = %v, want unauthorized", errObj["code"])
	}
}

func TestInternalAPIMiddlewareRejectsWrongKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", nil)
	req.Header.Set("X-HotKey-Internal-Key", "wrong-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestInternalAPIMiddlewareRejectsMissingTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", nil)
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object in response")
	}
	if errObj["code"] != "missing_tenant_id" {
		t.Fatalf("error code = %v, want missing_tenant_id", errObj["code"])
	}
}

func TestInternalAPIMiddlewareAcceptsValidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"workflowName":"test","executionId":"exec-1","status":"succeeded"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestInternalAPIMiddlewareUsesDefaultTenantWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "default-tenant")

	body := `{"workflowName":"test","executionId":"exec-1","status":"succeeded"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestInternalAPIPlaceholderEndpointsAreMounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "workflow status", method: http.MethodPost, path: "/api/v1/internal/workflow/status", wantStatus: http.StatusOK},
		{name: "batch ingest", method: http.MethodPost, path: "/api/v1/internal/ingest/contents", wantStatus: http.StatusAccepted},
		{name: "daily candidates", method: http.MethodGet, path: "/api/v1/internal/daily/candidates", wantStatus: http.StatusOK},
		{name: "daily reports", method: http.MethodPost, path: "/api/v1/internal/daily/reports", wantStatus: http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBufferString(`{"workflowName":"test","executionId":"exec-1","status":"succeeded"}`)
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-HotKey-Internal-Key", "test-key")
			req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestIdempotencyKeyPreventsDuplicateExecution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"workflowName":"test","executionId":"exec-dup","status":"succeeded"}`

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", bytes.NewBufferString(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-HotKey-Internal-Key", "test-key")
	req1.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	req1.Header.Set("Idempotency-Key", "idem-key-1")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want %d", rec1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-HotKey-Internal-Key", "test-key")
	req2.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	req2.Header.Set("Idempotency-Key", "idem-key-1")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("second request: status = %d, want %d", rec2.Code, http.StatusOK)
	}

	var resp1, resp2 map[string]any
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode second response: %v", err)
	}

	if resp1["idempotentReplay"] == true {
		t.Fatalf("first request should not be a replay")
	}
	if resp2["idempotentReplay"] != true {
		t.Fatalf("second request should be a replay")
	}
}

func newTestInternalRouter(internalKey string, defaultTenantID string) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	internalGroup := router.Group("/api/v1/internal")
	internalGroup.Use(InternalAPIAuthMiddleware(internalKey, defaultTenantID))
	{
		writeOK := func(status int) gin.HandlerFunc {
			return func(c *gin.Context) {
				c.JSON(status, gin.H{
					"ok":               true,
					"idempotentReplay": isIdempotentReplay(c),
					"tenantId":         getTenantID(c),
				})
			}
		}
		internalGroup.POST("/workflow/status", writeOK(http.StatusOK))
		internalGroup.POST("/ingest/contents", writeOK(http.StatusAccepted))
		internalGroup.GET("/daily/candidates", writeOK(http.StatusOK))
		internalGroup.POST("/daily/reports", writeOK(http.StatusCreated))
	}
	return router
}
