package application

import (
	"context"
	"errors"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedhotspot "github.com/StephenQiu30/hotkey-server/backend/internal/shared/hotspot"
)

func TestInstantSearchReturnsPartialResultsAndExplicitSourceStatuses(t *testing.T) {
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	repository := instantSearchSourceRepository{connections: []domain.SourceConnection{
		{ID: 1, SourceType: domain.SourceTypeHackerNews, Name: "HN", Enabled: true, Config: domain.DefaultSourceConfig()},
		{ID: 2, SourceType: domain.SourceTypeX, Name: "X", Enabled: true, Config: domain.DefaultSourceConfig()},
	}}
	registry := instantSearchRegistry{connectors: map[int64]domain.Connector{
		1: instantSearchConnector{result: domain.FetchResult{Items: []domain.SourceItem{
			{SourceCode: "hacker_news", ExternalID: "hn-1", ContentType: "article", Title: "Claude ships a realtime API", Body: "Anthropic announced a Claude API update.", URL: "https://news.example.test/claude?utm_source=test#top", Author: "alice", PublishedAt: timePointer(now.Add(-time.Hour)), ObservedAt: now, Metrics: domain.SourceMetrics{LikeCount: domain.KnownMetric(12), CommentCount: domain.KnownMetric(5)}},
			{SourceCode: "hacker_news", ExternalID: "hn-2", ContentType: "article", Title: "Duplicate Claude link", URL: "https://news.example.test/claude", PublishedAt: timePointer(now.Add(-2 * time.Hour)), ObservedAt: now},
			{SourceCode: "hacker_news", ExternalID: "hn-old", ContentType: "article", Title: "Claude archive", URL: "https://news.example.test/old", PublishedAt: timePointer(now.Add(-8 * 24 * time.Hour)), ObservedAt: now},
			{SourceCode: "hacker_news", ExternalID: "hn-other", ContentType: "article", Title: "PostgreSQL release", URL: "https://news.example.test/postgres", PublishedAt: timePointer(now), ObservedAt: now},
		}}},
		2: instantSearchConnector{err: domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("provider detail must stay private"))},
	}}
	service, err := NewInstantSearchService(InstantSearchDependencies{Sources: repository, Admission: instantSearchAdmission{}, Connectors: registry, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("NewInstantSearchService() error = %v", err)
	}

	result, err := service.Search(context.Background(), InstantSearchInput{
		Subject: identitydomain.Subject{UserID: 7, Role: identitydomain.RoleViewer},
		Query:   " Claude ", SourceTypes: []string{"hacker_news", "x", "duckduckgo"}, Limit: 20,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if result.Query != "Claude" || len(result.Items) != 1 {
		t.Fatalf("result = %#v, want one normalized Claude item", result)
	}
	item := result.Items[0]
	if item.CanonicalURL != "https://news.example.test/claude" || item.Relevance != 100 || !item.KeywordMentioned {
		t.Fatalf("normalized item = %#v", item)
	}
	if item.QualityState != sharedhotspot.QualityUnavailable || item.HeatScore <= 0 || item.Importance != sharedhotspot.ImportanceMedium {
		t.Fatalf("safe analysis fallback = %#v", item)
	}
	assertInstantStatus(t, result.SourceStatuses, "hacker_news", sharedhotspot.SourceSuccess, 1, "")
	assertInstantStatus(t, result.SourceStatuses, "x", sharedhotspot.SourceFailed, 0, "rate_limited")
	assertInstantStatus(t, result.SourceStatuses, "duckduckgo", sharedhotspot.SourceUnavailable, 0, "not_configured")
}

func TestInstantSearchValidatesAuthenticationAndInputBeforeCallingSources(t *testing.T) {
	service, err := NewInstantSearchService(InstantSearchDependencies{
		Sources: instantSearchSourceRepository{}, Admission: instantSearchAdmission{}, Connectors: instantSearchRegistry{},
	})
	if err != nil {
		t.Fatalf("NewInstantSearchService() error = %v", err)
	}
	_, err = service.Search(context.Background(), InstantSearchInput{Query: "Claude"})
	assertAppCode(t, err, sharederrors.CodeUnauthenticated)
	_, err = service.Search(context.Background(), InstantSearchInput{
		Subject: identitydomain.Subject{UserID: 1, Role: identitydomain.RoleViewer}, Query: " ",
	})
	assertAppCode(t, err, sharederrors.CodeInvalidCollectionRequest)
}

func TestInstantSearchAdmissionDenialStopsBeforeConnectorResolution(t *testing.T) {
	now := time.Date(2026, time.August, 29, 13, 0, 0, 0, time.UTC)
	connection := domain.SourceConnection{ID: 3, SourceType: domain.SourceTypeX, Name: "X", Enabled: true, Config: domain.DefaultSourceConfig()}
	resolveCalls := 0
	service, err := NewInstantSearchService(InstantSearchDependencies{
		Sources:    instantSearchSourceRepository{connections: []domain.SourceConnection{connection}},
		Admission:  instantSearchAdmission{err: domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("fixture denial"))},
		Connectors: instantSearchRegistry{resolveCalls: &resolveCalls}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Search(context.Background(), InstantSearchInput{
		Subject: identitydomain.Subject{UserID: 7, Role: identitydomain.RoleViewer}, Query: "Claude", SourceTypes: []string{"x"}, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertInstantStatus(t, result.SourceStatuses, "x", sharedhotspot.SourceFailed, 0, "rate_limited")
	if resolveCalls != 0 {
		t.Fatalf("connector resolutions after instant-search admission denial = %d", resolveCalls)
	}
}

func TestCollectionRelevanceUsesASCIIWordBoundariesAndKeepsCJKSubstrings(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		text      string
		relevance int
		mentioned bool
	}{
		{name: "standalone acronym", query: "AI", text: "AI governance", relevance: 100, mentioned: true},
		{name: "hyphenated acronym", query: "AI", text: "AI-generated summary", relevance: 100, mentioned: true},
		{name: "inside tailscale", query: "AI", text: "Tailscale database corruption", relevance: 0},
		{name: "inside available", query: "AI", text: "Community Edition is now available", relevance: 0},
		{name: "multi word phrase", query: "AI governance", text: "Operationalizing AI governance today", relevance: 100, mentioned: true},
		{name: "cjk substring", query: "热点", text: "持续监控热点事件", relevance: 100, mentioned: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relevance, mentioned := collectionRelevance(test.query, test.text)
			if relevance != test.relevance || mentioned != test.mentioned {
				t.Fatalf("collectionRelevance(%q, %q) = %d/%v, want %d/%v", test.query, test.text, relevance, mentioned, test.relevance, test.mentioned)
			}
		})
	}
}

func assertInstantStatus(t *testing.T, statuses []sharedhotspot.SourceStatus, sourceType string, state sharedhotspot.SourceState, count int, code string) {
	t.Helper()
	for _, status := range statuses {
		if status.SourceType == sourceType {
			if status.State != state || status.ResultCount != count || status.ErrorCode != code {
				t.Fatalf("status %q = %#v", sourceType, status)
			}
			return
		}
	}
	t.Fatalf("missing source status %q in %#v", sourceType, statuses)
}

type instantSearchSourceRepository struct{ connections []domain.SourceConnection }

func (repository instantSearchSourceRepository) List(context.Context, domain.SourceConnectionListQuery) ([]domain.SourceConnection, string, error) {
	return append([]domain.SourceConnection(nil), repository.connections...), "", nil
}

type instantSearchRegistry struct {
	connectors   map[int64]domain.Connector
	resolveCalls *int
}

func (registry instantSearchRegistry) Resolve(_ context.Context, connection domain.SourceConnection) (domain.Connector, error) {
	if registry.resolveCalls != nil {
		*registry.resolveCalls++
	}
	connector := registry.connectors[connection.ID]
	if connector == nil {
		return nil, errors.New("connector unavailable")
	}
	return connector, nil
}

type instantSearchAdmission struct{ err error }

func (admission instantSearchAdmission) AuthorizeCollection(context.Context, domain.SourceConnection) error {
	return admission.err
}

type instantSearchConnector struct {
	result domain.FetchResult
	err    error
}

func (connector instantSearchConnector) Validate(context.Context, domain.SourceConnection) error {
	return nil
}
func (connector instantSearchConnector) Fetch(context.Context, domain.FetchRequest) (domain.FetchResult, error) {
	return connector.result, connector.err
}
func (connector instantSearchConnector) Health(context.Context, domain.SourceConnection) domain.HealthResult {
	return domain.HealthResult{Healthy: true}
}

func timePointer(value time.Time) *time.Time { return &value }
