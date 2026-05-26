package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/propagation"
	"github.com/gin-gonic/gin"
)

func TestPropagationEndpointsExposePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	body := `{"sourceId":"arxiv","layer":"fact","url":"https://arxiv.org/abs/1","observedAt":"2026-05-26T10:30:00Z","note":"paper source"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/event_1/propagation", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/events/event_1/propagation", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	var path propagation.Path
	if err := json.Unmarshal(getRec.Body.Bytes(), &path); err != nil {
		t.Fatalf("decode path: %v", err)
	}
	if len(path.Steps) != 1 || path.Steps[0].SourceID != "arxiv" {
		t.Fatalf("path = %#v", path)
	}
}

func TestArbitrationEndpointExplainsFactConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	for _, body := range []string{
		`{"claimKey":"benchmark_score","value":"92.1","sourceId":"lab-a","layer":"fact","trustScore":92}`,
		`{"claimKey":"benchmark_score","value":"84.7","sourceId":"lab-b","layer":"fact","trustScore":88}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/event_1/claims", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/event_1/arbitration", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var result propagation.ArbitrationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode arbitration: %v", err)
	}
	if result.Status != propagation.StatusConflict || result.Explanation == "" {
		t.Fatalf("result = %#v", result)
	}
}
