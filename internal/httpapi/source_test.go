package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/StephenQiu30/hotkey-server/internal/source"
	"github.com/gin-gonic/gin"
)

func TestAdminSourceEndpointsListAndUpdateSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithServices(keyword.NewService(), source.NewService(), content.NewService())

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sources", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listBody struct {
		Sources []source.Source `json:"sources"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listBody.Sources) < 2 {
		t.Fatalf("sources len = %d, want at least 2", len(listBody.Sources))
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/sources/arxiv-ai", bytes.NewBufferString(`{"enabled":false,"rateLimitPerHour":10}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var updated source.Source
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("updated.Enabled = true, want false")
	}
	if updated.RateLimitPerHour != 10 {
		t.Fatalf("rateLimitPerHour = %d, want 10", updated.RateLimitPerHour)
	}
	if updated.AccessMode == source.AccessModeBypass {
		t.Fatalf("source returned bypass access mode")
	}
}

func TestAdminSourceEndpointRejectsInvalidThrottle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithServices(keyword.NewService(), source.NewService(), content.NewService())

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/sources/arxiv-ai", bytes.NewBufferString(`{"rateLimitPerHour":0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body["error"]["code"] != "invalid_source_config" {
		t.Fatalf("error code = %q, want invalid_source_config", body["error"]["code"])
	}
}
