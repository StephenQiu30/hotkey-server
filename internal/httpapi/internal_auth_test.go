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
		body       string
		wantStatus int
	}{
		{name: "workflow status", method: http.MethodPost, path: "/api/v1/internal/workflow/status", body: `{"workflowName":"test","executionId":"exec-1","status":"succeeded"}`, wantStatus: http.StatusOK},
		{name: "batch ingest", method: http.MethodPost, path: "/api/v1/internal/ingest/contents", body: `{"sourceCode":"hn","sourceType":"propagation","items":[{"externalId":"1","url":"https://example.com","title":"Test","publishedAt":"2026-05-27T08:00:00Z"}]}`, wantStatus: http.StatusAccepted},
		{name: "daily candidates", method: http.MethodGet, path: "/api/v1/internal/daily/candidates", wantStatus: http.StatusOK},
		{name: "daily reports", method: http.MethodPost, path: "/api/v1/internal/daily/reports", body: `{"reportDate":"2026-05-26","markdown":"# Report"}`, wantStatus: http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Buffer
			if tt.body != "" {
				body = bytes.NewBufferString(tt.body)
			} else {
				body = bytes.NewBufferString("")
			}
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

func TestWorkflowStatusAcceptsSucceededStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"workflowName":"fact_source_collector","executionId":"exec-100","status":"succeeded","runStartedAt":"2026-05-27T08:00:00Z","runFinishedAt":"2026-05-27T08:05:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["executionId"] != "exec-100" {
		t.Fatalf("executionId = %v, want exec-100", resp["executionId"])
	}
	if resp["recordId"] != "rec_exec-100" {
		t.Fatalf("recordId = %v, want rec_exec-100", resp["recordId"])
	}
}

func TestWorkflowStatusAcceptsFailedStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"workflowName":"daily_report","executionId":"exec-200","status":"failed","errorMessage":"SMTP connection timeout"}`
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

func TestWorkflowStatusRejectsInvalidStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"workflowName":"test","executionId":"exec-300","status":"unknown_status"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/workflow/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestBatchIngestAcceptsValidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{
		"sourceCode": "hackernews",
		"sourceType": "propagation",
		"items": [
			{"externalId": "hn-1", "url": "https://news.ycombinator.com/item?id=1", "title": "Test", "publishedAt": "2026-05-27T08:00:00Z"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ingest/contents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
}

func TestBatchIngestRejectsInvalidSourceType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"sourceCode":"test","sourceType":"invalid","items":[{"url":"https://example.com","title":"Test","publishedAt":"2026-05-27T08:00:00Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ingest/contents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestBatchIngestRejectsMissingRequiredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"sourceCode":"test","sourceType":"fact","items":[{"title":"Test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ingest/contents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDailyCandidatesReturnsEmptyListWhenNoEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/daily/candidates?date=2026-05-26", nil)
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["date"] != "2026-05-26" {
		t.Fatalf("date = %v, want 2026-05-26", resp["date"])
	}
	events, ok := resp["events"].([]any)
	if !ok || len(events) != 0 {
		t.Fatalf("events should be empty array")
	}
}

func TestDailyReportSaveAcceptsValidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"reportDate":"2026-05-26","markdown":"# Daily Report\n\nToday's hotspots...","html":"<h1>Daily Report</h1>","workflowName":"daily_ai_hotspot_email_digest","executionId":"exec-500"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/daily/reports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["reportId"] == nil {
		t.Fatalf("reportId should not be nil")
	}
	if resp["saved"] != true {
		t.Fatalf("saved = %v, want true", resp["saved"])
	}
}

func TestDailyReportSaveRejectsMissingReportDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"markdown":"# Daily Report"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/daily/reports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDailyReportSaveRejectsInvalidDateFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestInternalRouter("test-key", "tenant-1")

	body := `{"reportDate":"05/26/2026","markdown":"# Daily Report"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/daily/reports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HotKey-Internal-Key", "test-key")
	req.Header.Set("X-HotKey-Tenant-ID", "tenant-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
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
		internalGroup.POST("/workflow/status", handleInternalWorkflowStatus())
		internalGroup.POST("/ingest/contents", handleInternalBatchIngestContents())
		internalGroup.GET("/daily/candidates", handleInternalDailyCandidates())
		internalGroup.POST("/daily/reports", handleInternalSaveDailyReport())
	}
	return router
}
