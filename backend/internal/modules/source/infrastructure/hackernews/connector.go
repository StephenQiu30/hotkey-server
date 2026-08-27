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

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/evidencecapture"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const (
	sourceCode              = "hacker_news"
	collectorProfileVersion = "hacker-news-firebase-json-v1"
	maxItemWorkers          = 4
)

type Connector struct {
	sourceID       int64
	client         *client
	mode           domain.HackerNewsMode
	resourceLimits ResourceLimitProfile
	requestBudget  domain.ExternalRequestBudget
	retryWait      func(context.Context, int) error
}

type hnItem struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Parent      int64  `json:"parent"`
	Poll        int64  `json:"poll"`
	Score       *int64 `json:"score"`
	Descendants *int64 `json:"descendants"`
	Deleted     bool   `json:"deleted"`
	Dead        bool   `json:"dead"`
}

type itemOutcome struct {
	id         int64
	item       domain.SourceItem
	diagnostic *domain.FetchDiagnostic
	retryAfter *time.Time
	err        error
	snapshot   *domain.EvidenceSnapshot
}

func hackerNewsSnapshot(response fetchedJSONResponse) (domain.EvidenceSnapshot, error) {
	return evidencecapture.NewJSONSnapshot(response.payload, collectorProfileVersion, response.requestedURL, response.finalURL,
		response.redirectChain, response.statusCode, response.headers, response.capturedAt)
}

// New binds the HN Connector to the only supported official endpoint.
func New(connection domain.SourceConnection, requestBudget domain.ExternalRequestBudget, resolvers ...sourcenet.Resolver) (*Connector, error) {
	options := connectorOptions{requestBudget: requestBudget}
	if len(resolvers) > 0 && resolvers[0] != nil {
		options.resolver = resolvers[0].LookupIPAddr
	}
	return newConnector(connection, options)
}

type connectorOptions struct {
	clientOptions
	resourceLimits ResourceLimitProfile
	requestBudget  domain.ExternalRequestBudget
	retryWait      func(context.Context, int) error
}

func newConnector(connection domain.SourceConnection, options connectorOptions) (*Connector, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeHackerNews || normalized.Endpoint != domain.HackerNewsEndpoint {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News source connection"))
	}
	endpoint, err := url.Parse(normalized.Endpoint)
	if err != nil || !sameOfficialHost(endpoint, endpoint) {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News endpoint"))
	}
	if options.resourceLimits.Version == "" {
		options.resourceLimits = DefaultResourceLimitProfile()
	}
	if err := options.resourceLimits.Validate(); err != nil || options.requestBudget == nil || normalized.Config.MaxPagesPerRun > options.resourceLimits.MaxPages {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News resource limit profile"))
	}
	if options.retryWait == nil {
		options.retryWait = retryBackoff
	}
	readTimeout := time.Duration(normalized.Config.RequestTimeoutSeconds) * time.Second
	reserveRequest := func(ctx context.Context) error {
		return reserveHackerNewsRequest(ctx, options.requestBudget, normalized.ID, options.resourceLimits, options.clientOptions.now)
	}
	return &Connector{
		sourceID: normalized.ID, client: newClient(endpoint, options.resourceLimits, readTimeout, reserveRequest, options.clientOptions), mode: normalized.Config.HackerNewsMode,
		resourceLimits: options.resourceLimits, requestBudget: options.requestBudget, retryWait: options.retryWait,
	}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeHackerNews || normalized.Endpoint != domain.HackerNewsEndpoint || (connector.sourceID > 0 && normalized.ID != connector.sourceID) {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("Hacker News source connection does not match connector"))
	}
	return nil
}

func (connector *Connector) get(ctx context.Context, path string, byteBudget *responseByteBudget) (fetchedJSONResponse, *time.Time, error) {
	for attempt := 0; ; attempt++ {
		if err := reserveHackerNewsRequest(ctx, connector.requestBudget, connector.sourceID, connector.resourceLimits, connector.client.now); err != nil {
			var quota requestQuotaError
			if !errors.As(err, &quota) {
				return fetchedJSONResponse{}, nil, err
			}
			resetAt := quota.resetAt.UTC()
			return fetchedJSONResponse{}, &resetAt, domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("Hacker News daily request quota exceeded"))
		}
		response, retryAfter, err := connector.client.get(ctx, path, byteBudget)
		if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary || attempt >= connector.resourceLimits.MaxRetries {
			return response, retryAfter, err
		}
		if err := connector.retryWait(ctx, attempt+1); err != nil {
			return fetchedJSONResponse{}, nil, domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("Hacker News retry interrupted"))
		}
	}
}

type requestQuotaError struct{ resetAt time.Time }

func (err requestQuotaError) Error() string { return "Hacker News daily request quota exceeded" }

func reserveHackerNewsRequest(ctx context.Context, budget domain.ExternalRequestBudget, sourceID int64, profile ResourceLimitProfile, now func() time.Time) error {
	at := time.Now().UTC()
	if now != nil {
		at = now().UTC()
	}
	decision, err := budget.ReserveExternalRequest(ctx, domain.ExternalRequestBudgetReservation{
		SourceConnectionID: sourceID, ResourceProfileVersion: profile.Version, DailyLimit: profile.DailyRequestQuota, At: at,
	})
	if err != nil {
		return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("reserve Hacker News request budget"))
	}
	if err := decision.Validate(profile.DailyRequestQuota); err != nil {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News request budget decision"))
	}
	if !decision.Allowed {
		return requestQuotaError{resetAt: decision.ResetAt}
	}
	return nil
}

func retryBackoff(ctx context.Context, attempt int) error {
	delay := time.Duration(100*(1<<min(attempt-1, 5))) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	if err := request.Validate(); err != nil {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News fetch request"))
	}
	if connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("Hacker News fetch request source does not match connector"))
	}
	ctx, cancel := context.WithTimeout(ctx, connector.resourceLimits.WallClockTimeout)
	defer cancel()
	if request.Limit > connector.resourceLimits.MaxItems {
		request.Limit = connector.resourceLimits.MaxItems
	}
	byteBudget := newResponseByteBudget(connector.resourceLimits.MaxCumulativeResponseBytes)
	if connector.mode != domain.HackerNewsModeNew {
		return connector.fetchRanked(ctx, request, byteBudget)
	}
	cursor, initial, err := parseCursor(request.RequestCursor)
	if err != nil {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid Hacker News request cursor"))
	}
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	maximumResponse, retry, err := connector.get(ctx, "maxitem.json", byteBudget)
	if err != nil {
		result.RateLimit.RetryAfter = retry
		return result, err
	}
	var newest int64
	if err := json.Unmarshal(maximumResponse.payload, &newest); err != nil || newest < 0 {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("decode Hacker News max item"))
	}
	start, end := itemRange(cursor, newest, int64(request.Limit), initial)
	if end < start {
		if !initial {
			result.NextCursor = strconv.FormatInt(cursor, 10)
		}
		return result, nil
	}
	outcomes, failure := connector.fetchItems(ctx, start, end, byteBudget)
	if failure != nil && ctx.Err() != nil {
		result.RateLimit.RetryAfter = failure.retryAfter
		return domain.FetchResult{RateLimit: result.RateLimit}, failure.err
	}
	responded := 0
	for _, outcome := range outcomes {
		if outcome.err == nil {
			responded++
		}
	}
	if failure != nil && responded == 0 {
		result.RateLimit.RetryAfter = failure.retryAfter
		return domain.FetchResult{RateLimit: result.RateLimit}, failure.err
	}
	completeThrough := end
	if failure != nil {
		incompleteID, incomplete := firstIncompleteOutcome(outcomes, start, end)
		if incomplete {
			completeThrough = incompleteID - 1
			result.Diagnostics = append(result.Diagnostics, incompleteDiagnostic(outcomes, incompleteID))
		}
		result.RateLimit.RetryAfter = failure.retryAfter
	}
	for _, outcome := range outcomes {
		if outcome.id > completeThrough {
			break
		}
		if outcome.diagnostic != nil {
			result.Diagnostics = append(result.Diagnostics, *outcome.diagnostic)
			continue
		}
		if outcome.err != nil {
			continue
		}
		item := outcome.item
		if outcome.snapshot == nil {
			return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("Hacker News item evidence is missing"))
		}
		result.Snapshots = appendUniqueSnapshot(result.Snapshots, *outcome.snapshot)
		result.Items = append(result.Items, item)
	}
	result.NextCursor = strconv.FormatInt(completeThrough, 10)
	result.HasMore = completeThrough < newest
	return result, nil
}

func (connector *Connector) fetchRanked(ctx context.Context, request domain.FetchRequest, byteBudget *responseByteBudget) (domain.FetchResult, error) {
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	path := "topstories.json"
	if connector.mode == domain.HackerNewsModeBest {
		path = "beststories.json"
	}
	rankedResponse, retry, err := connector.get(ctx, path, byteBudget)
	if err != nil {
		result.RateLimit.RetryAfter = retry
		return result, err
	}
	var listed []int64
	if err := json.Unmarshal(rankedResponse.payload, &listed); err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("decode Hacker News ranked stories"))
	}
	ids := make([]int64, 0, min(len(listed), request.Limit))
	seen := make(map[int64]struct{}, len(listed))
	for _, id := range listed {
		if id <= 0 {
			return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("decode Hacker News ranked story ID"))
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) == request.Limit {
			break
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	outcomes, failure := connector.fetchRankedItems(ctx, ids, byteBudget)
	if ctx.Err() != nil {
		return domain.FetchResult{}, canceledPageFailure(ctx.Err()).err
	}
	responded := 0
	rankedSnapshot, err := hackerNewsSnapshot(rankedResponse)
	if err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("capture Hacker News ranked response"))
	}
	result.Snapshots = append(result.Snapshots, rankedSnapshot)
	rankByID := make(map[int64]int, len(ids))
	for index, id := range ids {
		rankByID[id] = index
	}
	for _, outcome := range outcomes {
		if outcome.err == nil {
			responded++
		}
	}
	if failure != nil && responded == 0 {
		result.RateLimit.RetryAfter = failure.retryAfter
		return result, failure.err
	}
	for _, outcome := range outcomes {
		switch {
		case outcome.diagnostic != nil:
			result.Diagnostics = append(result.Diagnostics, *outcome.diagnostic)
		case outcome.err != nil:
			result.Diagnostics = append(result.Diagnostics, incompleteDiagnostic(outcomes, outcome.id))
		default:
			item := outcome.item
			if outcome.snapshot == nil {
				return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("Hacker News item evidence is missing"))
			}
			if err := evidencecapture.BindJSONPointer(&item, rankedSnapshot, "/"+strconv.Itoa(rankByID[outcome.id]), domain.EvidenceUsageContext); err != nil {
				return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("bind Hacker News ranked evidence"))
			}
			item.ObservedAt = rankedSnapshot.CapturedAt
			item, err = domain.NormalizeSourceItem(item)
			if err != nil {
				return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("normalize Hacker News ranked evidence"))
			}
			result.Snapshots = appendUniqueSnapshot(result.Snapshots, *outcome.snapshot)
			result.Items = append(result.Items, item)
		}
	}
	if failure != nil {
		result.RateLimit.RetryAfter = failure.retryAfter
	}
	return result, nil
}

func firstIncompleteOutcome(outcomes []itemOutcome, start, end int64) (int64, bool) {
	index := 0
	for id := start; id <= end; id++ {
		for index < len(outcomes) && outcomes[index].id < id {
			index++
		}
		if index >= len(outcomes) || outcomes[index].id != id || outcomes[index].err != nil {
			return id, true
		}
		index++
	}
	return 0, false
}

func incompleteDiagnostic(outcomes []itemOutcome, id int64) domain.FetchDiagnostic {
	code := "item_unfinished"
	for _, outcome := range outcomes {
		if outcome.id != id || outcome.err == nil {
			continue
		}
		switch domain.ClassifyCollectionError(outcome.err) {
		case domain.CollectionErrorAuthentication:
			code = "item_authentication_failure"
		case domain.CollectionErrorRateLimited:
			code = "item_rate_limited"
		case domain.CollectionErrorTemporary:
			code = "item_temporary_failure"
		case domain.CollectionErrorParse:
			code = "item_parse_failure"
		case domain.CollectionErrorPermanent:
			code = "item_permanent_failure"
		}
		break
	}
	return domain.FetchDiagnostic{Code: code, SourceExternalID: strconv.FormatInt(id, 10)}
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.client.now()
	if err := connector.Validate(ctx, connection); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: "invalid_source_connection"}
	}
	path := "maxitem.json"
	if connector.mode == domain.HackerNewsModeTop {
		path = "topstories.json"
	} else if connector.mode == domain.HackerNewsModeBest {
		path = "beststories.json"
	}
	ctx, cancel := context.WithTimeout(ctx, connector.resourceLimits.WallClockTimeout)
	defer cancel()
	if _, _, err := connector.get(ctx, path, newResponseByteBudget(connector.resourceLimits.MaxCumulativeResponseBytes)); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: "request_failed"}
	}
	return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
}

func (connector *Connector) fetchRankedItems(parent context.Context, ids []int64, byteBudget *responseByteBudget) ([]itemOutcome, *itemOutcome) {
	if err := parent.Err(); err != nil {
		return nil, canceledPageFailure(err)
	}
	jobs := make(chan int64)
	results := make(chan itemOutcome, len(ids))
	workers := min(maxItemWorkers, len(ids))
	var group sync.WaitGroup
	var failureMu sync.Mutex
	var failure *itemOutcome
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for id := range jobs {
				outcome := connector.fetchItem(parent, id, byteBudget)
				results <- outcome
				if outcome.err != nil {
					failureMu.Lock()
					if failure == nil || preferredPageFailure(outcome, *failure) {
						candidate := outcome
						failure = &candidate
					}
					failureMu.Unlock()
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, id := range ids {
			select {
			case <-parent.Done():
				return
			case jobs <- id:
			}
		}
	}()
	go func() {
		group.Wait()
		close(results)
	}()
	byID := make(map[int64]itemOutcome, len(ids))
	for outcome := range results {
		byID[outcome.id] = outcome
	}
	ordered := make([]itemOutcome, 0, len(ids))
	for _, id := range ids {
		if outcome, ok := byID[id]; ok {
			ordered = append(ordered, outcome)
		}
	}
	if err := parent.Err(); err != nil {
		return ordered, canceledPageFailure(err)
	}
	failureMu.Lock()
	defer failureMu.Unlock()
	if failure != nil {
		return ordered, failure
	}
	if len(ordered) != len(ids) {
		return ordered, &itemOutcome{err: domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("Hacker News ranked page was interrupted"))}
	}
	return ordered, nil
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

func (connector *Connector) fetchItems(parent context.Context, start, end int64, byteBudget *responseByteBudget) ([]itemOutcome, *itemOutcome) {
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
				outcome := connector.fetchItem(ctx, id, byteBudget)
				outcomes <- outcome
				if outcome.err != nil {
					recordFailure(outcome)
					if stopsHNPage(outcome.err) {
						cancel()
					}
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

func stopsHNPage(err error) bool {
	switch domain.ClassifyCollectionError(err) {
	case domain.CollectionErrorAuthentication, domain.CollectionErrorRateLimited, domain.CollectionErrorPermanent:
		return true
	default:
		return false
	}
}

func canceledPageFailure(_ error) *itemOutcome {
	return &itemOutcome{err: domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("Hacker News item page canceled"))}
}

func preferredPageFailure(candidate, current itemOutcome) bool {
	candidateKind := domain.ClassifyCollectionError(candidate.err)
	currentKind := domain.ClassifyCollectionError(current.err)
	if candidatePriority, currentPriority := hnFailurePriority(candidateKind), hnFailurePriority(currentKind); candidatePriority != currentPriority {
		return candidatePriority > currentPriority
	}
	if candidate.retryAfter != nil && current.retryAfter == nil {
		return true
	}
	return candidate.id > 0 && (current.id <= 0 || candidate.id < current.id)
}

func hnFailurePriority(kind domain.CollectionErrorKind) int {
	switch kind {
	case domain.CollectionErrorRateLimited:
		return 5
	case domain.CollectionErrorAuthentication:
		return 4
	case domain.CollectionErrorPermanent:
		return 3
	case domain.CollectionErrorParse:
		return 2
	case domain.CollectionErrorTemporary:
		return 1
	default:
		return 0
	}
}

func (connector *Connector) fetchItem(ctx context.Context, id int64, byteBudget *responseByteBudget) itemOutcome {
	response, retry, err := connector.get(ctx, "item/"+strconv.FormatInt(id, 10)+".json", byteBudget)
	if err != nil {
		return itemOutcome{id: id, retryAfter: retry, err: err}
	}
	if bytes.Equal(bytes.TrimSpace(response.payload), []byte("null")) {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "missing_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	var item hnItem
	if err := json.Unmarshal(response.payload, &item); err != nil || item.ID != id {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "invalid_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	if item.Deleted {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "deleted_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	if item.Dead {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "dead_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	contentType := ""
	parentExternalID := ""
	switch item.Type {
	case "story", "job", "poll":
		contentType = "article"
	case "comment":
		contentType = "comment"
		if item.Parent > 0 {
			parentExternalID = strconv.FormatInt(item.Parent, 10)
		}
	case "pollopt":
		contentType = "comment"
		if item.Poll > 0 {
			parentExternalID = strconv.FormatInt(item.Poll, 10)
		}
	default:
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "unsupported_item_type", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	var publishedAt *time.Time
	if item.Time > 0 {
		published := time.Unix(item.Time, 0).UTC()
		publishedAt = &published
	}
	itemURL := strings.TrimSpace(item.URL)
	discussionURL := "https://news.ycombinator.com/item?id=" + strconv.FormatInt(id, 10)
	if contentType == "comment" || itemURL == "" {
		itemURL = discussionURL
	}
	evidenceCompleteness := domain.EvidenceCompletenessMetadataOnly
	if strings.TrimSpace(item.Text) != "" {
		evidenceCompleteness = domain.EvidenceCompletenessFullBody
	}
	parties := []domain.SourcePartyAssertion{{
		Role: domain.SourcePartyRoleDistributor, Kind: domain.SourcePartyKindOrganization,
		IdentityNamespace: "platform", ExternalID: "hacker-news", DisplayName: "Hacker News", HomepageURL: "https://news.ycombinator.com",
	}}
	if strings.TrimSpace(item.By) != "" && itemURL == discussionURL {
		account := domain.SourcePartyAssertion{Kind: domain.SourcePartyKindAccount, IdentityNamespace: "hacker-news:user", ExternalID: item.By, DisplayName: item.By}
		account.Role = domain.SourcePartyRoleAuthor
		parties = append(parties, account)
		account.Role = domain.SourcePartyRoleContentOrigin
		parties = append(parties, account)
	}
	normalized, err := domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: strconv.FormatInt(id, 10), ParentExternalID: parentExternalID, ContentType: contentType,
		Title: item.Title, Body: item.Text, URL: itemURL, DiscussionURL: discussionURL, Author: item.By, PublishedAt: publishedAt,
		ObservedAt: response.capturedAt, EvidenceCompleteness: evidenceCompleteness,
		Metrics: domain.SourceMetrics{LikeCount: cloneHNMetric(item.Score), CommentCount: cloneHNMetric(item.Descendants)},
		Parties: parties,
	})
	if err != nil {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "invalid_item", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	snapshot, err := hackerNewsSnapshot(response)
	if err != nil {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "invalid_item_evidence", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	normalized.EvidenceReferences = []domain.EvidenceReference{wholePayloadReference(snapshot, domain.EvidenceUsageDocumentSource)}
	normalized.SnapshotKey = snapshot.Key
	normalized.ItemLocator = "/"
	normalized, err = domain.NormalizeSourceItem(normalized)
	if err != nil {
		return itemOutcome{id: id, diagnostic: &domain.FetchDiagnostic{Code: "invalid_item_evidence", SourceExternalID: strconv.FormatInt(id, 10)}}
	}
	return itemOutcome{id: id, item: normalized, snapshot: &snapshot}
}

func wholePayloadReference(snapshot domain.EvidenceSnapshot, usage domain.EvidenceUsage) domain.EvidenceReference {
	return domain.EvidenceReference{
		SnapshotKey: snapshot.Key, Usage: usage, LocatorType: domain.EvidenceLocatorWholePayload,
		LocatorValue: "/", SelectedPayloadSHA256: snapshot.PayloadSHA256, SelectorVersion: domain.WholePayloadSelectorVersion,
	}
}

func appendUniqueSnapshot(values []domain.EvidenceSnapshot, snapshot domain.EvidenceSnapshot) []domain.EvidenceSnapshot {
	for _, existing := range values {
		if existing.Key == snapshot.Key {
			return values
		}
	}
	return append(values, snapshot)
}

func cloneHNMetric(value *int64) *int64 {
	if value == nil {
		return nil
	}
	return domain.KnownMetric(*value)
}
