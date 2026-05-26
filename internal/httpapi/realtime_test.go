package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/realtime"
	"github.com/gin-gonic/gin"
)

func TestRealtimePushEndpointAcceptsAuthorizedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	body := `{
		"sourceId":"openai-realtime",
		"token":"demo-realtime-token",
		"sourceItemId":"rt_1",
		"title":"OpenAI releases realtime model",
		"contentHash":"hash-rt-1",
		"receivedAt":"2026-05-26T10:30:00Z",
		"vector":[0.91,0.12]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/realtime/events", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var response realtime.PushResult
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != realtime.StatusAccepted || response.Match.ClusterID == "" {
		t.Fatalf("response = %#v", response)
	}
}

func TestRealtimePushEndpointReturnsDegradedForRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()
	body := `{"sourceId":"openai-realtime","token":"demo-realtime-token","sourceItemId":"rt_1","title":"Realtime event","contentHash":"hash-1"}`
	first := httptest.NewRequest(http.MethodPost, "/api/v1/realtime/events", bytes.NewBufferString(body))
	router.ServeHTTP(httptest.NewRecorder(), first)

	limited := httptest.NewRequest(http.MethodPost, "/api/v1/realtime/events", bytes.NewBufferString(`{"sourceId":"openai-realtime","token":"demo-realtime-token","sourceItemId":"rt_2","title":"Realtime event 2","contentHash":"hash-2"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, limited)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
	var response realtime.PushResult
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != realtime.StatusDegraded || response.FallbackReason != realtime.FallbackRateLimited {
		t.Fatalf("response = %#v", response)
	}
}
