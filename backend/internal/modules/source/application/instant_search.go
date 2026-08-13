package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharedhotspot "github.com/StephenQiu30/hotkey-server/backend/internal/shared/hotspot"
)

const (
	instantSearchDefaultLimit = 20
	instantSearchMaximumLimit = 50
)

var instantSearchCapabilities = []string{
	"x", "bing_grounding", "google_agent_search", "duckduckgo", "hacker_news", "sogou", "bilibili", "weibo", "rss",
}

type InstantSearchSourceReader interface {
	List(context.Context, domain.SourceConnectionListQuery) ([]domain.SourceConnection, string, error)
}

type InstantSearchDependencies struct {
	Sources    InstantSearchSourceReader
	Connectors domain.CollectionConnectorRegistry
	Now        func() time.Time
}

type InstantSearchService struct {
	sources    InstantSearchSourceReader
	connectors domain.CollectionConnectorRegistry
	now        func() time.Time
}

type InstantSearchInput struct {
	Subject     identitydomain.Subject
	Query       string
	SourceTypes []string
	Limit       int
}

type InstantSearchResult struct {
	Query          string
	SearchedAt     time.Time
	Items          []sharedhotspot.Card
	SourceStatuses []sharedhotspot.SourceStatus
}

func NewInstantSearchService(dependencies InstantSearchDependencies) (*InstantSearchService, error) {
	if dependencies.Sources == nil || dependencies.Connectors == nil {
		return nil, errors.New("instant search dependencies are required")
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	return &InstantSearchService{sources: dependencies.Sources, connectors: dependencies.Connectors, now: dependencies.Now}, nil
}

func (service *InstantSearchService) Search(ctx context.Context, input InstantSearchInput) (InstantSearchResult, error) {
	if err := requireAuthenticated(input.Subject); err != nil {
		return InstantSearchResult{}, err
	}
	query := strings.TrimSpace(input.Query)
	limit := input.Limit
	if limit == 0 {
		limit = instantSearchDefaultLimit
	}
	requested, err := normalizeInstantSearchSourceTypes(input.SourceTypes)
	if err != nil || query == "" || utf8.RuneCountInString(query) > 200 || limit < 1 || limit > instantSearchMaximumLimit {
		return InstantSearchResult{}, domain.InvalidCollectionRequest()
	}
	connections, _, err := service.sources.List(ctx, domain.SourceConnectionListQuery{Limit: 200})
	if err != nil {
		return InstantSearchResult{}, collectionControlError(err)
	}
	selected := make(map[string][]domain.SourceConnection, len(requested))
	for _, connection := range connections {
		sourceType := string(connection.SourceType)
		if !connection.Enabled || connection.Deleted || !containsInstantSource(requested, sourceType) {
			continue
		}
		selected[sourceType] = append(selected[sourceType], connection)
	}
	now := service.now().UTC()
	result := InstantSearchResult{Query: query, SearchedAt: now, Items: []sharedhotspot.Card{}, SourceStatuses: []sharedhotspot.SourceStatus{}}
	for _, sourceType := range requested {
		group := selected[sourceType]
		if len(group) == 0 {
			result.SourceStatuses = append(result.SourceStatuses, sharedhotspot.SourceStatus{SourceType: sourceType, State: sharedhotspot.SourceUnavailable, ErrorCode: "not_configured"})
			continue
		}
		items, status := service.searchConnections(ctx, group, query, limit, now)
		result.Items = append(result.Items, items...)
		result.SourceStatuses = append(result.SourceStatuses, status)
	}
	result.Items = sharedhotspot.Dedupe(result.Items)
	sharedhotspot.SortByHeat(result.Items)
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	return result, nil
}

type instantConnectionOutcome struct {
	connection domain.SourceConnection
	items      []sharedhotspot.Card
	err        error
}

func (service *InstantSearchService) searchConnections(ctx context.Context, connections []domain.SourceConnection, query string, limit int, now time.Time) ([]sharedhotspot.Card, sharedhotspot.SourceStatus) {
	outcomes := make(chan instantConnectionOutcome, len(connections))
	var group sync.WaitGroup
	for _, connection := range connections {
		connection := connection
		group.Add(1)
		go func() {
			defer group.Done()
			items, err := service.searchConnection(ctx, connection, query, limit, now)
			outcomes <- instantConnectionOutcome{connection: connection, items: items, err: err}
		}()
	}
	group.Wait()
	close(outcomes)
	status := sharedhotspot.SourceStatus{SourceType: string(connections[0].SourceType), SourceName: connections[0].Name}
	items := []sharedhotspot.Card{}
	successes, failures := 0, 0
	for outcome := range outcomes {
		if outcome.err != nil {
			failures++
			if status.ErrorCode == "" {
				status.ErrorCode = instantSearchErrorCode(outcome.err)
			}
			continue
		}
		successes++
		items = append(items, outcome.items...)
	}
	status.ResultCount = len(sharedhotspot.Dedupe(items))
	switch {
	case successes == 0:
		status.State = sharedhotspot.SourceFailed
	case failures > 0:
		status.State = sharedhotspot.SourcePartial
	case status.ResultCount == 0:
		status.State = sharedhotspot.SourceEmpty
	default:
		status.State = sharedhotspot.SourceSuccess
	}
	return items, status
}

func (service *InstantSearchService) searchConnection(ctx context.Context, connection domain.SourceConnection, query string, limit int, now time.Time) ([]sharedhotspot.Card, error) {
	connector, err := service.connectors.Resolve(ctx, connection)
	if err != nil {
		return nil, err
	}
	if err := connector.Validate(ctx, connection); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(string(connection.SourceType) + "\n" + query))
	result, err := connector.Fetch(ctx, domain.FetchRequest{
		CollectionRunID: connection.ID, SourceConnectionID: connection.ID,
		QuerySignature: hex.EncodeToString(digest[:]), Query: query,
		WindowStart: now.Add(-7 * 24 * time.Hour), WindowEnd: now, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]sharedhotspot.Card, 0, len(result.Items))
	for _, candidate := range result.Items {
		if candidate.PublishedAt != nil && candidate.PublishedAt.Before(now.Add(-7*24*time.Hour)) {
			continue
		}
		relevance, mentioned := collectionRelevance(query, candidate.Title+" "+candidate.Body)
		if relevance < 50 || !mentioned && relevance < 65 {
			continue
		}
		canonicalURL, err := sharedhotspot.NormalizeURL(candidate.URL)
		if err != nil {
			continue
		}
		published := candidate.PublishedAt
		if published != nil {
			value := published.UTC()
			published = &value
		}
		metrics := sharedhotspot.Metrics{
			ViewCount: candidate.Metrics.ViewCount, LikeCount: candidate.Metrics.LikeCount,
			CommentCount: candidate.Metrics.CommentCount, ShareCount: candidate.Metrics.ShareCount,
		}
		heat := sharedhotspot.HeatScore(metrics)
		items = append(items, sharedhotspot.Card{
			SourceType: string(connection.SourceType), SourceName: connection.Name,
			ExternalID: candidate.ExternalID, ContentType: candidate.ContentType,
			Title: strings.TrimSpace(candidate.Title), Summary: strings.TrimSpace(candidate.Body),
			CanonicalURL: canonicalURL, Author: strings.TrimSpace(candidate.Author), PublishedAt: published,
			DiscoveredAt: candidate.ObservedAt.UTC(), Metrics: metrics,
			HeatScore: heat, QualityState: sharedhotspot.QualityUnavailable,
			Relevance: relevance, RelevanceReason: "标题或摘要与搜索词直接匹配",
			KeywordMentioned: mentioned, Importance: sharedhotspot.Importance(heat),
		})
	}
	return items, nil
}

func normalizeInstantSearchSourceTypes(values []string) ([]string, error) {
	if len(values) == 0 {
		return append([]string(nil), instantSearchCapabilities...), nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !containsInstantSource(instantSearchCapabilities, value) {
			return nil, errors.New("unsupported instant search source")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func containsInstantSource(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func instantSearchErrorCode(err error) string {
	switch domain.ClassifyCollectionError(err) {
	case domain.CollectionErrorAuthentication:
		return "authentication"
	case domain.CollectionErrorRateLimited:
		return "rate_limited"
	case domain.CollectionErrorTemporary:
		return "temporary"
	case domain.CollectionErrorParse:
		return "parse"
	default:
		return "unavailable"
	}
}
