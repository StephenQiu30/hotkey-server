package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/event"
	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/StephenQiu30/hotkey-server/internal/source"
	"github.com/gin-gonic/gin"
)

func TestAdminEventClusterEndpointsUpsertAndListClusters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouterWithServices(
		keyword.NewService(),
		source.NewService(),
		content.NewService(),
		event.NewService(event.Options{VectorEnabled: true, SimilarityThreshold: 0.85}),
	)

	firstBody := `{"sourceItemId":"item_1","title":"OpenAI releases new reasoning model","contentHash":"hash-1","vector":[1,0,0]}`
	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/event-candidates", bytes.NewBufferString(firstBody))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d; body=%s", firstRec.Code, http.StatusCreated, firstRec.Body.String())
	}

	secondBody := `{"sourceItemId":"item_2","title":"OpenAI announces reasoning model update","contentHash":"hash-2","vector":[0.95,0.05,0]}`
	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/event-candidates", bytes.NewBufferString(secondBody))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d; body=%s", secondRec.Code, http.StatusOK, secondRec.Body.String())
	}

	var second event.ClusterMatch
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if second.MatchMethod != event.MatchMethodVector {
		t.Fatalf("matchMethod = %q, want %q", second.MatchMethod, event.MatchMethodVector)
	}
	if second.Similarity < 0.85 {
		t.Fatalf("similarity = %f, want >= 0.85", second.Similarity)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/event-clusters", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listBody struct {
		Clusters []event.EventCluster `json:"clusters"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listBody.Clusters) != 1 {
		t.Fatalf("clusters len = %d, want 1", len(listBody.Clusters))
	}
	if len(listBody.Clusters[0].Items) != 2 {
		t.Fatalf("cluster items len = %d, want 2", len(listBody.Clusters[0].Items))
	}
}
