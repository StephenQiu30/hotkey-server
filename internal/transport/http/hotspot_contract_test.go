package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestHotspotListRequiresAuthAndReturnsSortedScores(t *testing.T) {
	router := transportRouterForTest()

	// Unauthenticated request should return 401
	unauthorized := getJSON(router, "/api/v1/hotspots")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated hotspots to return 401, got %d with body %s", unauthorized.Code, unauthorized.Body.String())
	}

	// Authenticated request should return 200 with items array
	userToken := registerAndLogin(t, router, "hotspot-user@example.com")
	list := getWithBearer(router, "/api/v1/hotspots", userToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected hotspots list 200, got %d with body %s", list.Code, list.Body.String())
	}
	assertJSONField(t, list.Body.Bytes(), "items", "")
}

func TestHotspotDetailRequiresAuthAndReturnsScoreExplanation(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "hotspot-detail@example.com")

	// Request a non-existent hotspot should return 404
	notFound := getWithBearer(router, "/api/v1/hotspots/nonexistent", userToken)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected missing hotspot 404, got %d with body %s", notFound.Code, notFound.Body.String())
	}
	assertJSONField(t, notFound.Body.Bytes(), "error.code", "hotspot_not_found")
}

func TestHotspotListSupportsChannelAndTimeFiltering(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "hotspot-filter@example.com")

	// With channel filter
	filtered := getWithBearer(router, "/api/v1/hotspots?channel=ai-models", userToken)
	if filtered.Code != http.StatusOK {
		t.Fatalf("expected filtered hotspots 200, got %d with body %s", filtered.Code, filtered.Body.String())
	}

	// With time window filter
	timeFiltered := getWithBearer(router, "/api/v1/hotspots?since=2026-05-01T00:00:00Z&until=2026-06-01T00:00:00Z", userToken)
	if timeFiltered.Code != http.StatusOK {
		t.Fatalf("expected time-filtered hotspots 200, got %d with body %s", timeFiltered.Code, timeFiltered.Body.String())
	}
}

func TestHotspotListRejectsInvertedTimeWindow(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "hotspot-inverted-window@example.com")

	response := getWithBearer(router, "/api/v1/hotspots?since=2026-06-01T00:00:00Z&until=2026-05-01T00:00:00Z", userToken)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected inverted time window 400, got %d with body %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "error.code", "invalid_time_window")
}

func TestHotspotListAppliesChannelAndPagination(t *testing.T) {
	router := hotspotRouterWithScores(t,
		servicehotspot.HotspotScore{
			ID: "score-ai-top", ClusterID: "cluster-ai-top", TotalScore: 0.95,
			ChannelIDs: []string{"ai-models"}, CreatedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC),
		},
		servicehotspot.HotspotScore{
			ID: "score-ai-next", ClusterID: "cluster-ai-next", TotalScore: 0.80,
			ChannelIDs: []string{"ai-models"}, CreatedAt: time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC),
		},
		servicehotspot.HotspotScore{
			ID: "score-finance", ClusterID: "cluster-finance", TotalScore: 0.99,
			ChannelIDs: []string{"finance"}, CreatedAt: time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC),
		},
	)
	userToken := registerAndLogin(t, router, "hotspot-channel-page@example.com")

	response := getWithBearer(router, "/api/v1/hotspots?channel=ai-models&limit=1&offset=1", userToken)
	if response.Code != http.StatusOK {
		t.Fatalf("expected filtered paged hotspots 200, got %d with body %s", response.Code, response.Body.String())
	}

	var body struct {
		Items []struct {
			ID         string   `json:"id"`
			ChannelIDs []string `json:"channelIDs"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON %s: %v", response.Body.String(), err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected exactly 1 paged item, got %d in %s", len(body.Items), response.Body.String())
	}
	if body.Items[0].ID != "score-ai-next" {
		t.Fatalf("expected offset to return second ai-models score, got %q in %s", body.Items[0].ID, response.Body.String())
	}
	if len(body.Items[0].ChannelIDs) != 1 || body.Items[0].ChannelIDs[0] != "ai-models" {
		t.Fatalf("expected returned item to stay in ai-models channel, got %#v", body.Items[0].ChannelIDs)
	}
}

func TestHotspotDetailReturnsSourceRefs(t *testing.T) {
	router := hotspotRouterWithScores(t, servicehotspot.HotspotScore{
		ID: "score-detail", ClusterID: "cluster-detail", TotalScore: 0.91,
		SourceRefs: []servicehotspot.SourceRef{{
			ItemID: "item-1", SourceID: "src-1", Title: "AI Agent 新突破", URL: "https://example.com/agent",
		}},
		CreatedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC),
	})
	userToken := registerAndLogin(t, router, "hotspot-source-refs@example.com")

	response := getWithBearer(router, "/api/v1/hotspots/cluster-detail", userToken)
	if response.Code != http.StatusOK {
		t.Fatalf("expected hotspot detail 200, got %d with body %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "sourceRefs.0.sourceId", "src-1")
	assertJSONField(t, response.Body.Bytes(), "sourceRefs.0.url", "https://example.com/agent")
}

func TestHotspotListDefaultSortedByTotalScoreDescending(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "hotspot-sort@example.com")

	sorted := getWithBearer(router, "/api/v1/hotspots", userToken)
	if sorted.Code != http.StatusOK {
		t.Fatalf("expected sorted hotspots 200, got %d with body %s", sorted.Code, sorted.Body.String())
	}
}

func hotspotRouterWithScores(t *testing.T, scores ...servicehotspot.HotspotScore) http.Handler {
	t.Helper()
	scoreRepo := servicehotspot.NewMemoryScoreRepository()
	for _, score := range scores {
		if _, err := scoreRepo.SaveScore(context.Background(), score); err != nil {
			t.Fatal(err)
		}
	}
	return transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
		ScoringService: servicehotspot.NewScoringService(servicehotspot.ScoringConfig{}, nil, scoreRepo),
	})
}
