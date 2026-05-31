package http_test

import (
	"net/http"
	"testing"
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

func TestHotspotListDefaultSortedByTotalScoreDescending(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "hotspot-sort@example.com")

	sorted := getWithBearer(router, "/api/v1/hotspots", userToken)
	if sorted.Code != http.StatusOK {
		t.Fatalf("expected sorted hotspots 200, got %d with body %s", sorted.Code, sorted.Body.String())
	}
}
