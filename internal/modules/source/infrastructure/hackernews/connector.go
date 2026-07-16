package hackernews

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

const (
	sourceCode     = "hacker_news"
	maxItemWorkers = 4
)

type Connector struct {
	sourceID int64
	client   *client
}

type hnItem struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Score       int64  `json:"score"`
	Descendants int64  `json:"descendants"`
	Deleted     bool   `json:"deleted"`
	Dead        bool   `json:"dead"`
}

type itemOutcome struct {
	id         int64
	item       domain.SourceItem
	diagnostic *domain.FetchDiagnostic
	retryAfter *time.Time
	err        error
}

// New binds the HN Connector to the only supported official endpoint.
func New(connection domain.SourceConnection) (*Connector, error) {
	return newConnector(connection, clientOptions{})
}

func newConnector(connection domain.SourceConnection, options clientOptions) (*Connector, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeHackerNews || normalized.Endpoint != domain.HackerNewsEndpoint {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News source connection"))
	}
	endpoint, err := url.Parse(normalized.Endpoint)
	if err != nil || !sameOfficialHost(endpoint, endpoint) {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News endpoint"))
	}
	timeout := time.Duration(normalized.Config.RequestTimeoutSeconds) * time.Second
	return &Connector{sourceID: normalized.ID, client: newClient(endpoint, timeout, options)}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeHackerNews || normalized.Endpoint != domain.HackerNewsEndpoint || (connector.sourceID > 0 && normalized.ID != connector.sourceID) {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("Hacker News source connection does not match connector"))
	}
	return nil
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	if err := request.Validate(); err != nil {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News fetch request"))
	}
	if connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("Hacker News fetch request source does not match connector"))
	}
	cursor, initial, err := parseCursor(request.RequestCursor)
	if err != nil {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News request cursor"))
	}
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	maxPayload, retry, err := connector.client.get(ctx, "maxitem.json")
	if err != nil {
		result.RateLimit.RetryAfter = retry
		return result, err
	}
	var newest int64
	if err := json.Unmarshal(maxPayload, &newest); err != nil || newest < 0 {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("decode Hacker News max item"))
	}
	start, end := itemRange(cursor, newest, int64(request.Limit), initial)
	if end < start {
		if !initial {
			result.NextCursor = strconv.FormatInt(cursor, 10)
		}
		return result, nil
	}
	outcomes, failure := connector.fetchItems(ctx, start, end)
	if failure != nil {
		result.RateLimit.RetryAfter = failure.retryAfter
		return domain.FetchResult{RateLimit: result.RateLimit}, failure.err
	}
	for _, outcome := range outcomes {
		if outcome.diagnostic != nil {
			result.Diagnostics = append(result.Diagnostics, *outcome.diagnostic)
			continue
		}
		result.Items = append(result.Items, outcome.item)
	}
	result.NextCursor = strconv.FormatInt(end, 10)
	result.HasMore = end < newest
	return result, nil
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.client.now()
	if err := connector.Validate(ctx, connection); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: "invalid_source_connection"}
	}
	if _, _, err := connector.client.get(ctx, "maxitem.json"); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: "request_failed"}
	}
	return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
}

func parseCursor(value string) (int64, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true, nil
	}
	cursor, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cursor < 0 {
		return 0, false, fmt.Errorf("cursor must be a non-negative item ID")
	}
	return cursor, false, nil
}

func itemRange(cursor, newest, limit int64, initial bool) (int64, int64) {
	if initial && newest > limit {
		cursor = newest - limit
	}
	if newest <= cursor {
		return 1, 0
	}
	end := newest
	if cursor <= math.MaxInt64-limit && cursor+limit < end {
		end = cursor + limit
	}
	return cursor + 1, end
}

func (connector *Connector) fetchItems(parent context.Context, start, end int64) ([]itemOutcome, *itemOutcome) {
	if err := parent.Err(); err != nil {
		return nil, canceledPageFailure(err)
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	jobs := make(chan int64)
	outcomes := make(chan itemOutcome, end-start+1)
	workers := maxItemWorkers
	if remaining := int(end - start + 1); remaining < workers {
		workers = remaining
	}
	var group sync.WaitGroup
	var failureMu sync.Mutex
	var failure *itemOutcome
	recordFailure := func(outcome itemOutcome) {
		failureMu.Lock()
		defer failureMu.Unlock()
		if failure == nil || preferredPageFailure(outcome, *failure) {
			candidate := outcome
			failure = &candidate
		}
	}
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for id := range jobs {
				outcome := connector.fetchItem(ctx, id)
				outcomes <- outcome
				if outcome.err != nil {
					recordFailure(outcome)
					cancel()
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for id := start; id <= end; id++ {
			select {
			case <-ctx.Done():
				return
			case jobs <- id:
			}
		}
	}()
	go func() {
		group.Wait()
		close(outcomes)
	}()
	collected := make([]itemOutcome, 0, end-start+1)
	for outcome := range outcomes {
		collected = append(collected, outcome)
	}
	sort.Slice(collected, func(left, right int) bool { return collected[left].id < collected[right].id })
	if err := parent.Err(); err != nil {
		return collected, canceledPageFailure(err)
	}
	failureMu.Lock()
	defer failureMu.Unlock()
	if failure != nil {
		return collected, failure
	}
	if len(collected) != int(end-start+1) {
		return collected, &itemOutcome{err: domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("Hacker News item page was interrupted"))}
	}
	return collected, nil
}

func canceledPageFailure(_ error) *itemOutcome {
	return &itemOutcome{err: domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("Hacker News item page canceled"))}
}

func preferredPageFailure(candidate, current itemOutcome) bool {
	candidateKind := domain.ClassifyCollectionError(candidate.err)
	currentKind := domain.ClassifyCollectionError(current.err)
	if candidateKind == domain.CollectionErrorRateLimited && currentKind != domain.CollectionErrorRateLimited {
		return true
	}
	return candidate.retryAfter != nil && current.retryAfter == nil
}

func (connector *Connector) fetchItem(ctx context.Context, id int64) itemOutcome {
	payload, retry, err := connector.client.get(ctx, "item/"+strconv.FormatInt(id, 10)+".json")
	if err != nil {
		return itemOutcome{id: id, retryAfter: retry, err: err}
	}
	if bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "missing_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	var item hnItem
	if err := json.Unmarshal(payload, &item); err != nil || item.ID != id {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "invalid_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	if item.Deleted {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "deleted_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	if item.Dead {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "dead_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	contentType := ""
	switch item.Type {
	case "story":
		contentType = "article"
	case "comment":
		contentType = "comment"
	default:
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "unsupported_item_type", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	var publishedAt *time.Time
	if item.Time > 0 {
		published := time.Unix(item.Time, 0).UTC()
		publishedAt = &published
	}
	itemURL := strings.TrimSpace(item.URL)
	if contentType == "comment" || itemURL == "" {
		itemURL = "https://news.ycombinator.com/item?id=" + strconv.FormatInt(id, 10)
	}
	normalized, err := domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: strconv.FormatInt(id, 10), ContentType: contentType,
		Title: item.Title, Body: item.Text, URL: itemURL, Author: item.By, PublishedAt: publishedAt,
		ObservedAt: connector.client.now(), Metrics: domain.SourceMetrics{LikeCount: domain.KnownMetric(item.Score), CommentCount: domain.KnownMetric(item.Descendants)},
	})
	if err != nil {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "invalid_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	return itemOutcome{id: id, item: normalized}
}
