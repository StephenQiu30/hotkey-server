package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/redisinfra"
	"github.com/gin-gonic/gin"
)

func TestRedisInfraEndpointsRateLimitAndQueueRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	body := `{"userId":"user-1","scope":"keyword","target":"OpenAI"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh-queue", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("refresh %d status = %d, want %d; body=%s", i+1, rec.Code, http.StatusAccepted, rec.Body.String())
		}
	}

	limitedReq := httptest.NewRequest(http.MethodPost, "/api/v1/refresh-queue", bytes.NewBufferString(body))
	limitedReq.Header.Set("Content-Type", "application/json")
	limitedRec := httptest.NewRecorder()
	router.ServeHTTP(limitedRec, limitedReq)
	if limitedRec.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status = %d, want %d; body=%s", limitedRec.Code, http.StatusTooManyRequests, limitedRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/refresh-queue", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listBody struct {
		Queue []redisinfra.RefreshQueueItem `json:"queue"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listBody.Queue) != 2 {
		t.Fatalf("queue len = %d, want 2", len(listBody.Queue))
	}
}

func TestRedisInfraHealthEndpointReturnsMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/redis/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var status redisinfra.HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if !status.Available || status.Mode != redisinfra.ModeAvailable {
		t.Fatalf("status = %#v, want available", status)
	}
}
