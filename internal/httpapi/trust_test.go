package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/trust"
	"github.com/gin-gonic/gin"
)

func TestEventEvidenceEndpointsAddEvidenceSummaryAndReturnDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	factBody := `{"eventId":"cluster_1","sourceId":"arxiv-ai","sourceItemId":"item_1","layer":"fact","trustLevel":"high","title":"Research paper confirms release","url":"https://arxiv.org/abs/2605.00001"}`
	factReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/event-evidence", bytes.NewBufferString(factBody))
	factReq.Header.Set("Content-Type", "application/json")
	factRec := httptest.NewRecorder()
	router.ServeHTTP(factRec, factReq)
	if factRec.Code != http.StatusCreated {
		t.Fatalf("fact status = %d, want %d; body=%s", factRec.Code, http.StatusCreated, factRec.Body.String())
	}

	signalBody := `{"eventId":"cluster_1","sourceId":"low-trust-social","sourceItemId":"item_2","layer":"signal","trustLevel":"low","title":"Viral repost","url":"https://example.com/repost","heatWeight":5}`
	signalReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/event-evidence", bytes.NewBufferString(signalBody))
	signalReq.Header.Set("Content-Type", "application/json")
	signalRec := httptest.NewRecorder()
	router.ServeHTTP(signalRec, signalReq)
	if signalRec.Code != http.StatusCreated {
		t.Fatalf("signal status = %d, want %d; body=%s", signalRec.Code, http.StatusCreated, signalRec.Body.String())
	}

	summaryBody := `{"summary":"OpenAI released a new model.","citationIds":["item_1"]}`
	summaryReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/cluster_1/ai-summary", bytes.NewBufferString(summaryBody))
	summaryReq.Header.Set("Content-Type", "application/json")
	summaryRec := httptest.NewRecorder()
	router.ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("summary status = %d, want %d; body=%s", summaryRec.Code, http.StatusOK, summaryRec.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/events/cluster_1/evidence", nil)
	detailRec := httptest.NewRecorder()
	router.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%s", detailRec.Code, http.StatusOK, detailRec.Body.String())
	}

	var detail trust.EventEvidenceDetail
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if len(detail.FactEvidence) != 1 {
		t.Fatalf("fact evidence len = %d, want 1", len(detail.FactEvidence))
	}
	if len(detail.SignalEvidence) != 1 {
		t.Fatalf("signal evidence len = %d, want 1", len(detail.SignalEvidence))
	}
	if detail.AISummary == nil || len(detail.AISummary.CitationIDs) != 1 {
		t.Fatalf("ai summary missing citation: %#v", detail.AISummary)
	}
}

func TestEventSummaryEndpointRejectsMissingCitation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events/cluster_1/ai-summary", bytes.NewBufferString(`{"summary":"OpenAI released a new model."}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
