package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

const (
	InstantSearchQualityUnavailable = "unavailable"

	InstantSearchImportanceLow    = "low"
	InstantSearchImportanceMedium = "medium"

	InstantSearchSourceSuccess     InstantSearchSourceState = "success"
	InstantSearchSourceEmpty       InstantSearchSourceState = "empty"
	InstantSearchSourcePartial     InstantSearchSourceState = "partial"
	InstantSearchSourceFailed      InstantSearchSourceState = "failed"
	InstantSearchSourceUnavailable InstantSearchSourceState = "unavailable"

	instantSearchDefaultLimit = 20
	instantSearchMaximumLimit = 50
)

var instantSearchCapabilities = []string{
	"x", "bing_grounding", "google_agent_search", "duckduckgo", "hacker_news", "sogou", "bilibili", "weibo", "rss",
}

type InstantSearchSourceState string

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

type InstantSearchMetrics struct {
	ViewCount    *int64
	LikeCount    *int64
	CommentCount *int64
	ShareCount   *int64
}

type InstantSearchItem struct {
	SourceType       string
	SourceName       string
	ExternalID       string
	ContentType      string
	Title            string
	Summary          string
	CanonicalURL     string
	Author           string
	PublishedAt      *time.Time
	DiscoveredAt     time.Time
	Metrics          InstantSearchMetrics
	HeatScore        float64
	QualityState     string
	Relevance        int
	RelevanceReason  string
	KeywordMentioned bool
	Importance       string
}

type InstantSearchSourceStatus struct {
	SourceType  string
	SourceName  string
	State       InstantSearchSourceState
	ResultCount int
	ErrorCode   string
}

type InstantSearchResult struct {
	Query          string
	SearchedAt     time.Time
	Items          []InstantSearchItem
	SourceStatuses []InstantSearchSourceStatus
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
	result := InstantSearchResult{Query: query, SearchedAt: now, Items: []InstantSearchItem{}, SourceStatuses: []InstantSearchSourceStatus{}}
	for _, sourceType := range requested {
		group := selected[sourceType]
		if len(group) == 0 {
			result.SourceStatuses = append(result.SourceStatuses, InstantSearchSourceStatus{SourceType: sourceType, State: InstantSearchSourceUnavailable, ErrorCode: "not_configured"})
			continue
		}
		items, status := service.searchConnections(ctx, group, query, limit, now)
		result.Items = append(result.Items, items...)
		result.SourceStatuses = append(result.SourceStatuses, status)
	}
	result.Items = dedupeInstantSearchItems(result.Items)
	sort.SliceStable(result.Items, func(i, j int) bool {
		if result.Items[i].HeatScore != result.Items[j].HeatScore {
			return result.Items[i].HeatScore > result.Items[j].HeatScore
		}
		left, right := result.Items[i].DiscoveredAt, result.Items[j].DiscoveredAt
		return left.After(right)
	})
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	return result, nil
}

type instantConnectionOutcome struct {
	connection domain.SourceConnection
	items      []InstantSearchItem
	err        error
}

func (service *InstantSearchService) searchConnections(ctx context.Context, connections []domain.SourceConnection, query string, limit int, now time.Time) ([]InstantSearchItem, InstantSearchSourceStatus) {
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
	status := InstantSearchSourceStatus{SourceType: string(connections[0].SourceType), SourceName: connections[0].Name}
	items := []InstantSearchItem{}
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
	status.ResultCount = len(dedupeInstantSearchItems(items))
	switch {
	case successes == 0:
		status.State = InstantSearchSourceFailed
	case failures > 0:
		status.State = InstantSearchSourcePartial
	case status.ResultCount == 0:
		status.State = InstantSearchSourceEmpty
	default:
		status.State = InstantSearchSourceSuccess
	}
	return items, status
}

func (service *InstantSearchService) searchConnection(ctx context.Context, connection domain.SourceConnection, query string, limit int, now time.Time) ([]InstantSearchItem, error) {
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
	items := make([]InstantSearchItem, 0, len(result.Items))
	for _, candidate := range result.Items {
		if candidate.PublishedAt != nil && candidate.PublishedAt.Before(now.Add(-7*24*time.Hour)) {
			continue
		}
		relevance, mentioned := instantSearchRelevance(query, candidate.Title+" "+candidate.Body)
		if relevance < 50 || !mentioned && relevance < 65 {
			continue
		}
		canonicalURL := normalizeInstantSearchURL(candidate.URL)
		published := candidate.PublishedAt
		if published != nil {
			value := published.UTC()
			published = &value
		}
		heat := instantSearchHeat(candidate.Metrics)
		importance := InstantSearchImportanceLow
		if heat >= 25 {
			importance = InstantSearchImportanceMedium
		}
		items = append(items, InstantSearchItem{
			SourceType: string(connection.SourceType), SourceName: connection.Name,
			ExternalID: candidate.ExternalID, ContentType: candidate.ContentType,
			Title: strings.TrimSpace(candidate.Title), Summary: strings.TrimSpace(candidate.Body),
			CanonicalURL: canonicalURL, Author: strings.TrimSpace(candidate.Author), PublishedAt: published,
			DiscoveredAt: candidate.ObservedAt.UTC(), Metrics: InstantSearchMetrics{
				ViewCount: candidate.Metrics.ViewCount, LikeCount: candidate.Metrics.LikeCount,
				CommentCount: candidate.Metrics.CommentCount, ShareCount: candidate.Metrics.ShareCount,
			}, HeatScore: heat, QualityState: InstantSearchQualityUnavailable,
			Relevance: relevance, RelevanceReason: "标题或摘要与搜索词直接匹配",
			KeywordMentioned: mentioned, Importance: importance,
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

func instantSearchRelevance(query, text string) (int, bool) {
	query = normalizedCollectionText(query)
	text = normalizedCollectionText(text)
	if query != "" && strings.Contains(text, query) {
		return 100, true
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 0, false
	}
	matched := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			matched++
		}
	}
	return int(math.Round(float64(matched) / float64(len(terms)) * 80)), false
}

func instantSearchHeat(metrics domain.SourceMetrics) float64 {
	metric := func(value *int64) float64 {
		if value == nil {
			return 0
		}
		return math.Log1p(float64(*value))
	}
	score := metric(metrics.ViewCount)*2 + metric(metrics.LikeCount)*10 + metric(metrics.CommentCount)*4 + metric(metrics.ShareCount)*6
	return math.Round(math.Min(score, 100)*10) / 10
}

func normalizeInstantSearchURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return strings.TrimSpace(value)
	}
	parsed.Fragment = ""
	query := parsed.Query()
	for key := range query {
		canonical := strings.ToLower(key)
		if strings.HasPrefix(canonical, "utm_") || canonical == "fbclid" || canonical == "gclid" || canonical == "mc_cid" || canonical == "mc_eid" {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func dedupeInstantSearchItems(items []InstantSearchItem) []InstantSearchItem {
	seenIdentity := map[string]struct{}{}
	seenURL := map[string]struct{}{}
	result := make([]InstantSearchItem, 0, len(items))
	for _, item := range items {
		identity := item.SourceType + "\x00" + item.ExternalID
		if _, exists := seenIdentity[identity]; exists {
			continue
		}
		if item.CanonicalURL != "" {
			if _, exists := seenURL[item.CanonicalURL]; exists {
				continue
			}
			seenURL[item.CanonicalURL] = struct{}{}
		}
		seenIdentity[identity] = struct{}{}
		result = append(result, item)
	}
	return result
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
