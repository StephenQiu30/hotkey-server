package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/hotspot"
	"github.com/gin-gonic/gin"
)

func TestHotspotEndpointsListAndDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/hotspots?region=global&language=en&sort=heat", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listBody struct {
		Hotspots []hotspot.HotspotSummary `json:"hotspots"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listBody.Hotspots) == 0 {
		t.Fatalf("hotspots is empty")
	}
	if listBody.Hotspots[0].Region != "global" || listBody.Hotspots[0].Language != "en" {
		t.Fatalf("first hotspot locale = %s/%s", listBody.Hotspots[0].Region, listBody.Hotspots[0].Language)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/hotspots/"+listBody.Hotspots[0].ID, nil)
	detailRec := httptest.NewRecorder()
	router.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%s", detailRec.Code, http.StatusOK, detailRec.Body.String())
	}

	var detail hotspot.HotspotDetail
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detail.ID != listBody.Hotspots[0].ID {
		t.Fatalf("detail id = %q, want %q", detail.ID, listBody.Hotspots[0].ID)
	}
	if len(detail.RelatedContent) == 0 {
		t.Fatalf("detail related content is empty")
	}
	if len(detail.Evidence.FactEvidenceIDs) == 0 {
		t.Fatalf("detail fact evidence is empty")
	}
	if detail.SimilarityScore == 0 {
		t.Fatalf("detail similarity score is zero")
	}
}
