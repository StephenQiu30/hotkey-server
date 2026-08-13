package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharedhotspot "github.com/StephenQiu30/hotkey-server/backend/internal/shared/hotspot"
	"github.com/gin-gonic/gin"
)

func TestInstantSearchRouteReturnsFlatHotspotCardsAndSourceStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	searchedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	service := &instantSearchServiceFake{result: sourceapplication.InstantSearchResult{
		Query: "Claude", SearchedAt: searchedAt,
		Items: []sharedhotspot.Card{{
			SourceType: "hacker_news", SourceName: "HN", ExternalID: "42", ContentType: "article",
			Title: "Claude API", Summary: "A realtime update", CanonicalURL: "https://example.test/claude",
			Author: "alice", DiscoveredAt: searchedAt, HeatScore: 31.2,
			QualityState: sharedhotspot.QualityUnavailable, Relevance: 100,
			RelevanceReason: "direct match", KeywordMentioned: true, Importance: sharedhotspot.ImportanceMedium,
		}},
		SourceStatuses: []sharedhotspot.SourceStatus{{
			SourceType: "hacker_news", SourceName: "HN", State: sharedhotspot.SourceSuccess, ResultCount: 1,
		}, {SourceType: "x", State: sharedhotspot.SourceFailed, ErrorCode: "rate_limited"}},
	}}
	router := gin.New()
	RegisterInstantSearchRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 7, SessionID: 8, Role: httptransport.RoleViewer}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(`{"query":" Claude ","source_types":["hacker_news","x"]}`))
	request.Header.Set("Authorization", "Bearer viewer")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Results        []map[string]any `json:"results"`
			SourceStatuses []map[string]any `json:"source_statuses"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 0 || len(envelope.Data.Results) != 1 || len(envelope.Data.SourceStatuses) != 2 {
		t.Fatalf("response = %s", response.Body.String())
	}
	card := envelope.Data.Results[0]
	for _, field := range []string{"title", "summary", "source_type", "heat_score", "quality_state", "relevance", "importance", "canonical_url"} {
		if _, exists := card[field]; !exists {
			t.Fatalf("flat card is missing %q: %#v", field, card)
		}
	}
	if _, nested := card["analysis"]; nested || strings.Contains(response.Body.String(), "provider detail") {
		t.Fatalf("response contains nested analysis or unsafe provider detail: %s", response.Body.String())
	}
	if service.input.Subject.UserID != 7 || service.input.Query != " Claude " || len(service.input.SourceTypes) != 2 {
		t.Fatalf("service input = %#v", service.input)
	}
}

func TestInstantSearchRouteRejectsInvalidOrUnauthenticatedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name          string
		authenticator httptransport.Authenticator
		body          string
		want          int
	}{
		{name: "missing auth", authenticator: testAuthenticator{}, body: `{"query":"Claude"}`, want: http.StatusUnauthorized},
		{name: "empty query", authenticator: testAuthenticator{subject: httptransport.Subject{UserID: 7, SessionID: 8, Role: httptransport.RoleViewer}}, body: `{"query":""}`, want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			RegisterInstantSearchRoutes(router, &instantSearchServiceFake{}, test.authenticator)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.name != "missing auth" {
				request.Header.Set("Authorization", "Bearer viewer")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

type instantSearchServiceFake struct {
	result sourceapplication.InstantSearchResult
	input  sourceapplication.InstantSearchInput
}

func (service *instantSearchServiceFake) Search(_ context.Context, input sourceapplication.InstantSearchInput) (sourceapplication.InstantSearchResult, error) {
	service.input = input
	if strings.TrimSpace(input.Query) == "" {
		return sourceapplication.InstantSearchResult{}, domain.InvalidCollectionRequest()
	}
	return service.result, nil
}
