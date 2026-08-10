package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type radarHTTPRepositoryFake struct {
	page    domain.RadarPage
	queries []domain.RadarQuery
}

func (fake *radarHTTPRepositoryFake) ListRadar(_ context.Context, query domain.RadarQuery) (domain.RadarPage, error) {
	fake.queries = append(fake.queries, query)
	if query.Cursor == "bad-cursor" {
		return domain.RadarPage{}, sharedrepository.ErrInvalidInput
	}
	return fake.page, nil
}

type eventUpdateHTTPRepositoryFake struct {
	page    domain.EventUpdatePage
	queries []domain.EventUpdateListQuery
}

func (*eventUpdateHTTPRepositoryFake) PreviousHeatSnapshot(context.Context, int64, int, time.Time) (*domain.HeatResult, error) {
	return nil, nil
}
func (*eventUpdateHTTPRepositoryFake) AppendUpdate(context.Context, domain.EventUpdateCandidate) (*domain.EventUpdate, bool, error) {
	return nil, false, nil
}
func (fake *eventUpdateHTTPRepositoryFake) ListUpdates(_ context.Context, query domain.EventUpdateListQuery) (domain.EventUpdatePage, error) {
	fake.queries = append(fake.queries, query)
	return fake.page, nil
}

type radarViewerAuthenticator struct{}

func (radarViewerAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 8, SessionID: 9, Role: httptransport.RoleViewer}, nil
}

func TestRadarAndEventUpdateGetRoutesRequireBearerAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRadarRoutes(router, application.NewRadarService(&radarHTTPRepositoryFake{}), httptransport.NewUnavailableAuthenticator())
	RegisterEventUpdateRoutes(router, application.NewUpdateService(&eventUpdateHTTPRepositoryFake{}), httptransport.NewUnavailableAuthenticator())
	for _, target := range []string{"/api/v1/radar/events", "/api/v1/events/7/updates"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		assertRadarHTTPResultCode(t, recorder, http.StatusUnauthorized, 20000)
	}
}

func TestRadarHandlerParsesPublicShapeAndReturnsOnlySafeResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	asOf := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	confirmationScore, relevance, finalScore := 100.0, 91.0, 99.0
	repository := &radarHTTPRepositoryFake{page: domain.RadarPage{AsOf: asOf, NextCursor: "next-safe", Items: []domain.RadarEvent{{
		EventID: 7, Attention: 80, Momentum: 75, Breadth: 50, IndependentSourceCount: 2,
		Confirmation: domain.RadarConfirmationCorroborated, ConfirmationScore: &confirmationScore,
		DataConfidence: 64, WatchRelevance: &relevance, WatchFinalScore: &finalScore, RankingScore: 99,
		LatestUpdate: &domain.EventUpdate{ID: 12, Version: 1, EventID: 7, SequenceNo: 2, Kind: domain.EventUpdateRising, Summary: "evidence-bound", ObservedAt: asOf, ReasonCodes: []string{"rising"}, IdempotencyKey: "must-not-leak"},
	}}}}
	router := gin.New()
	RegisterRadarRoutes(router, application.NewRadarService(repository), radarViewerAuthenticator{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/radar/events?q=%E5%8F%91%E5%B8%83&window=6h&monitor_id=9&lifecycle=active,cooling&trend=rising,stable&min_heat=42.5&sort=relevance&limit=25&cursor=opaque", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || len(repository.queries) != 1 {
		t.Fatalf("Radar response = %d/%s queries=%#v", recorder.Code, recorder.Body.String(), repository.queries)
	}
	query := repository.queries[0]
	if query.Keyword != "发布" || query.Window != domain.RadarWindow6Hours || query.MonitorID == nil || *query.MonitorID != 9 || query.Sort != domain.RadarSortRelevance || query.Limit != 25 || query.Cursor != "opaque" || query.MinHeat == nil || *query.MinHeat != 42.5 || len(query.Lifecycles) != 2 || len(query.Trends) != 2 || len(query.Verifications) != 0 {
		t.Fatalf("parsed Radar query = %#v", query)
	}
	assertRadarHTTPSafeEnvelope(t, recorder.Body.Bytes())
	body := recorder.Body.String()
	for _, forbidden := range []string{"must-not-leak", "idempotency_key", "provider", "prompt", "raw_content", "credential", "endpoint", "confirmation", "data_confidence", "credibility", "verification"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Radar response leaked %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{`"as_of":"2026-08-04T12:00:00Z"`, `"independent_source_count":2`, `"watch_final_score":99`, `"latest_update"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("Radar response missing %s: %s", required, body)
		}
	}
}

func TestRadarHandlerKeepsInsufficientScoreNullAndRejectsInvalidParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &radarHTTPRepositoryFake{page: domain.RadarPage{Items: []domain.RadarEvent{{EventID: 1, Confirmation: domain.RadarConfirmationInsufficient}}}}
	router := gin.New()
	RegisterRadarRoutes(router, application.NewRadarService(repository), radarViewerAuthenticator{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/radar/events", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "confirmation") || strings.Contains(recorder.Body.String(), "watch_relevance") || strings.Contains(recorder.Body.String(), "watch_final_score") {
		t.Fatalf("global insufficient Radar response = %d/%s", recorder.Code, recorder.Body.String())
	}
	for _, target := range []string{
		"/api/v1/radar/events?window=2h",
		"/api/v1/radar/events?sort=relevance",
		"/api/v1/radar/events?monitor_id=0",
		"/api/v1/radar/events?min_heat=101",
		"/api/v1/radar/events?limit=101",
		"/api/v1/radar/events?cursor=bad-cursor",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer viewer")
		router.ServeHTTP(recorder, request)
		assertRadarHTTPResultCode(t, recorder, http.StatusBadRequest, 10000)
	}
}

func TestEventUpdateHandlerParsesPathAndPageWithoutLeakingInternalKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	repository := &eventUpdateHTTPRepositoryFake{page: domain.EventUpdatePage{NextCursor: 8, Items: []domain.EventUpdate{{ID: 9, Version: 1, EventID: 7, SequenceNo: 9, Kind: domain.EventUpdateRising, Summary: "safe summary", ObservedAt: now, ReasonCodes: []string{"rising"}, EvidenceSetHash: strings.Repeat("a", 64), IdempotencyKey: "internal-key-must-not-leak"}}}}
	router := gin.New()
	RegisterEventUpdateRoutes(router, application.NewUpdateService(repository), radarViewerAuthenticator{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/7/updates?limit=20&cursor=10", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || len(repository.queries) != 1 || repository.queries[0].EventID != 7 || repository.queries[0].Limit != 20 || repository.queries[0].Cursor != 10 {
		t.Fatalf("EventUpdate response/query = %d/%s/%#v", recorder.Code, recorder.Body.String(), repository.queries)
	}
	assertRadarHTTPSafeEnvelope(t, recorder.Body.Bytes())
	if strings.Contains(recorder.Body.String(), "internal-key-must-not-leak") || strings.Contains(recorder.Body.String(), "idempotency_key") || !strings.Contains(recorder.Body.String(), `"sequence_no":9`) {
		t.Fatalf("unsafe EventUpdate response: %s", recorder.Body.String())
	}
	for _, target := range []string{"/api/v1/events/0/updates", "/api/v1/events/7/updates?limit=101", "/api/v1/events/7/updates?cursor=-1"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer viewer")
		router.ServeHTTP(recorder, request)
		assertRadarHTTPResultCode(t, recorder, http.StatusBadRequest, 10000)
	}
}

func assertRadarHTTPResultCode(t *testing.T, recorder *httptest.ResponseRecorder, status, code int) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, status, recorder.Body.String())
	}
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.Code != code {
		t.Fatalf("Result code = %d/%v, want %d body=%s", result.Code, err, code, recorder.Body.String())
	}
}

func assertRadarHTTPSafeEnvelope(t *testing.T, payload []byte) {
	t.Helper()
	var result map[string]json.RawMessage
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode Result: %v", err)
	}
	if len(result) != 3 || result["code"] == nil || result["message"] == nil || result["data"] == nil {
		t.Fatalf("Result keys = %#v", result)
	}
}
