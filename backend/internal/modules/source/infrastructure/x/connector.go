package x

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/evidencecapture"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const (
	sourceCode              = "x"
	collectorProfileVersion = "x-recent-search-json-v2"
	metricProfileVersion    = "x-post-lookup-json-v1"
	maxQueryCharacters      = 512
	maxResponseBodyBytes    = 4 << 20
	maxRedirects            = 3
	cursorVersion           = 1
)

var (
	errUnsafeDestination = errors.New("unsafe X destination")
	errRedirectLimit     = errors.New("X redirect limit exceeded")
	postIDPattern        = regexp.MustCompile(`^[0-9]{1,19}$`)
	usernamePattern      = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}$`)
)

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

type connectorOptions struct {
	resolver       lookupIPAddrFunc
	dialContext    func(context.Context, string, string) (net.Conn, error)
	tlsConfig      *tls.Config
	now            func() time.Time
	lookupEnv      func(string) (string, bool)
	resourceLimits ResourceLimitProfile
	requestBudget  domain.ExternalRequestBudget
	retryWait      func(context.Context, int) error
}

type Connector struct {
	sourceID       int64
	endpoint       *url.URL
	credentialRef  string
	enabled        bool
	deleted        bool
	http           *http.Client
	now            func() time.Time
	lookupEnv      func(string) (string, bool)
	resourceLimits ResourceLimitProfile
	requestBudget  domain.ExternalRequestBudget
	retryWait      func(context.Context, int) error
}

type searchCursor struct {
	Version     int    `json:"v"`
	SinceID     string `json:"s,omitempty"`
	NextToken   string `json:"n,omitempty"`
	HighWaterID string `json:"h,omitempty"`
	Initialized bool   `json:"i,omitempty"`
	SortOrder   string `json:"o,omitempty"`
}

type searchResponse struct {
	Data     []post         `json:"data"`
	Errors   []apiProblem   `json:"errors"`
	Includes searchIncludes `json:"includes"`
	Meta     searchMeta     `json:"meta"`
}

type searchMeta struct {
	NewestID    string `json:"newest_id"`
	OldestID    string `json:"oldest_id"`
	NextToken   string `json:"next_token"`
	ResultCount int    `json:"result_count"`
}

type post struct {
	ID                string          `json:"id"`
	Text              string          `json:"text"`
	Username          string          `json:"username"`
	AuthorID          string          `json:"author_id"`
	CreatedAt         string          `json:"created_at"`
	ConversationID    string          `json:"conversation_id"`
	Language          string          `json:"lang"`
	PossiblySensitive bool            `json:"possibly_sensitive"`
	Withheld          json.RawMessage `json:"withheld"`
	Attachments       postAttachments `json:"attachments"`
	ReferencedPosts   []postReference `json:"referenced_posts"`
	ReferencedTweets  []postReference `json:"referenced_tweets"`
	NoteTweet         noteTweet       `json:"note_tweet"`
	PublicMetrics     publicMetrics   `json:"public_metrics"`
}

type noteTweet struct {
	Text string `json:"text"`
}

type postReference struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type postAttachments struct {
	MediaKeys []string `json:"media_keys"`
}

type publicMetrics struct {
	ImpressionCount *int64 `json:"impression_count"`
	LikeCount       *int64 `json:"like_count"`
	ReplyCount      *int64 `json:"reply_count"`
	RepostCount     *int64 `json:"repost_count"`
	RetweetCount    *int64 `json:"retweet_count"`
	QuoteCount      *int64 `json:"quote_count"`
}

type searchIncludes struct {
	Users []includedUser  `json:"users"`
	Media []includedMedia `json:"media"`
}

type includedUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type includedMedia struct {
	MediaKey        string `json:"media_key"`
	Type            string `json:"type"`
	URL             string `json:"url"`
	PreviewImageURL string `json:"preview_image_url"`
}

type apiProblem struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Value        string `json:"value"`
}

type fetchedJSONResponse struct {
	payload       []byte
	statusCode    int
	requestedURL  string
	finalURL      string
	redirectChain []string
	headers       http.Header
	capturedAt    time.Time
}

func (value fetchedJSONResponse) snapshot(profileVersion string) (domain.EvidenceSnapshot, error) {
	return evidencecapture.NewJSONSnapshot(
		value.payload, profileVersion, value.requestedURL, value.finalURL,
		value.redirectChain, value.statusCode, value.headers, value.capturedAt,
	)
}

func New(connection domain.SourceConnection, requestBudget domain.ExternalRequestBudget, resolvers ...sourcenet.Resolver) (*Connector, error) {
	options := connectorOptions{requestBudget: requestBudget}
	if len(resolvers) > 0 && resolvers[0] != nil {
		options.resolver = resolvers[0].LookupIPAddr
	}
	return newConnector(connection, options)
}

func NewWithCredentialLookup(connection domain.SourceConnection, resolver sourcenet.Resolver, lookup func(string) (string, bool), requestBudget domain.ExternalRequestBudget) (*Connector, error) {
	options := connectorOptions{lookupEnv: lookup, requestBudget: requestBudget}
	if resolver != nil {
		options.resolver = resolver.LookupIPAddr
	}
	return newConnector(connection, options)
}

func newConnector(connection domain.SourceConnection, options connectorOptions) (*Connector, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeX || normalized.Endpoint != domain.XRecentSearchEndpoint {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X source connection"))
	}
	endpoint, err := url.Parse(normalized.Endpoint)
	if err != nil || !sameOfficialEndpoint(endpoint, endpoint) {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X recent search endpoint"))
	}
	if options.resolver == nil {
		options.resolver = net.DefaultResolver.LookupIPAddr
	}
	if options.dialContext == nil {
		options.dialContext = (&net.Dialer{}).DialContext
	}
	if options.now == nil {
		options.now = func() time.Time { return time.Now().UTC() }
	}
	if options.lookupEnv == nil {
		options.lookupEnv = os.LookupEnv
	}
	if options.resourceLimits.Version == "" {
		options.resourceLimits = DefaultResourceLimitProfile()
	}
	if err := options.resourceLimits.Validate(); err != nil || options.requestBudget == nil || normalized.Config.MaxPagesPerRun > options.resourceLimits.MaxPages {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X resource limit profile"))
	}
	if options.retryWait == nil {
		options.retryWait = retryBackoff
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if options.tlsConfig != nil {
		tlsConfig = options.tlsConfig.Clone()
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	configuredReadTimeout := time.Duration(normalized.Config.RequestTimeoutSeconds) * time.Second
	reserveRedirect := func(ctx context.Context) error {
		return reserveXRequest(ctx, options.requestBudget, normalized.ID, options.resourceLimits, options.now)
	}
	transport := &http.Transport{
		Proxy: nil, ForceAttemptHTTP2: true, TLSClientConfig: tlsConfig,
		DisableCompression:  true,
		TLSHandshakeTimeout: options.resourceLimits.ConnectTimeout, ResponseHeaderTimeout: minimumDuration(configuredReadTimeout, options.resourceLimits.ReadTimeout),
		DialContext: secureDialContext(options.resolver, options.dialContext, options.resourceLimits.ConnectTimeout),
	}
	client := &http.Client{
		Timeout: options.resourceLimits.ReadTimeout, Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errRedirectLimit
			}
			if len(via) == 0 || !sameOfficialEndpoint(via[0].URL, request.URL) {
				return errUnsafeDestination
			}
			if err := reserveRedirect(request.Context()); err != nil {
				return err
			}
			return nil
		},
	}
	return &Connector{
		sourceID: normalized.ID, endpoint: endpoint, credentialRef: normalized.CredentialRef,
		enabled: normalized.Enabled, deleted: normalized.Deleted, http: client,
		now: options.now, lookupEnv: options.lookupEnv, resourceLimits: options.resourceLimits,
		requestBudget: options.requestBudget, retryWait: options.retryWait,
	}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeX || normalized.Endpoint != domain.XRecentSearchEndpoint ||
		(connector.sourceID > 0 && normalized.ID != connector.sourceID) || normalized.CredentialRef != connector.credentialRef {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X source connection does not match connector"))
	}
	return nil
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	if err := request.Validate(); err != nil || (connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID) {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X fetch request"))
	}
	if !connector.enabled || connector.deleted {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X source connection is unavailable"))
	}
	query, err := compileSearchQuery(request.Query, request.Languages, request.Regions)
	if err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X search query"))
	}
	cursor := searchCursor{Version: cursorVersion}
	if strings.TrimSpace(request.RequestCursor) != "" {
		cursor, err = decodeCursor(request.RequestCursor)
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X search cursor"))
		}
	}
	ctx, cancel := context.WithTimeout(ctx, connector.resourceLimits.WallClockTimeout)
	defer cancel()
	token, err := connector.token()
	if err != nil {
		return result, err
	}
	if request.Limit > connector.resourceLimits.MaxItems {
		request.Limit = connector.resourceLimits.MaxItems
	}
	parameters := searchParameters(query, request.Limit, cursor)
	captured, rateLimit, err := connector.get(ctx, parameters, token, newResponseByteBudget(connector.resourceLimits.MaxCumulativeResponseBytes))
	result.RateLimit = rateLimit
	if err != nil {
		return result, err
	}
	var response searchResponse
	if err := json.Unmarshal(captured.payload, &response); err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("decode X recent search response"))
	}
	if len(response.Data) > connector.resourceLimits.MaxItems {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X response exceeds collection item limit"))
	}
	snapshot, err := captured.snapshot(collectorProfileVersion)
	if err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("capture X recent search response"))
	}
	result.Snapshots = append(result.Snapshots, snapshot)
	users := make(map[string]string, len(response.Includes.Users))
	for _, user := range response.Includes.Users {
		if usernamePattern.MatchString(user.Username) {
			users[user.ID] = user.Username
		}
	}
	media := make(map[string]includedMedia, len(response.Includes.Media))
	for _, item := range response.Includes.Media {
		media[item.MediaKey] = item
	}
	for index, sourcePost := range response.Data {
		switch {
		case sourcePost.PossiblySensitive:
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "possibly_sensitive_post", SourceExternalID: safePostID(sourcePost.ID)})
			continue
		case len(sourcePost.Withheld) > 0 && string(sourcePost.Withheld) != "null":
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "withheld_post", SourceExternalID: safePostID(sourcePost.ID)})
			continue
		}
		item, err := connector.mapPost(sourcePost, users, media, captured.capturedAt)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "invalid_post", SourceExternalID: safePostID(sourcePost.ID)})
			continue
		}
		if err := evidencecapture.BindJSONPointer(&item, snapshot, fmt.Sprintf("/data/%d", index), domain.EvidenceUsageDocumentSource); err != nil {
			return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("bind X post evidence"))
		}
		result.Items = append(result.Items, item)
	}
	for _, problem := range response.Errors {
		externalID := safePostID(problem.ResourceID)
		if externalID == "" {
			externalID = safePostID(problem.Value)
		}
		result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "unavailable_post", SourceExternalID: externalID})
	}
	sort.Slice(result.Diagnostics, func(left, right int) bool {
		if result.Diagnostics[left].Code != result.Diagnostics[right].Code {
			return result.Diagnostics[left].Code < result.Diagnostics[right].Code
		}
		return result.Diagnostics[left].SourceExternalID < result.Diagnostics[right].SourceExternalID
	})
	highWater := cursor.HighWaterID
	if highWater == "" {
		highWater = safePostID(response.Meta.NewestID)
	}
	if highWater == "" {
		highWater = newestPostID(response.Data)
	}
	if strings.TrimSpace(response.Meta.NextToken) != "" {
		if highWater == "" || !validOpaqueToken(response.Meta.NextToken) {
			return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("invalid X pagination metadata"))
		}
		result.NextCursor, err = encodeCursor(searchCursor{
			Version: cursorVersion, SinceID: cursor.SinceID, NextToken: response.Meta.NextToken, HighWaterID: highWater,
			Initialized: cursor.Initialized, SortOrder: searchSortOrder(cursor),
		})
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("encode X search cursor"))
		}
		result.HasMore = true
		return result, nil
	}
	finalSince := cursor.SinceID
	if highWater != "" {
		finalSince = highWater
	}
	if finalSince != "" {
		result.NextCursor, err = encodeCursor(searchCursor{Version: cursorVersion, SinceID: finalSince, Initialized: true})
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("encode X search cursor"))
		}
	} else if !cursor.Initialized {
		result.NextCursor, err = encodeCursor(searchCursor{Version: cursorVersion, Initialized: true})
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("encode X search cursor"))
		}
	}
	return result, nil
}

func (connector *Connector) LookupPostMetrics(ctx context.Context, request domain.XPostMetricLookupRequest) (domain.XPostMetricLookupResult, error) {
	result := domain.XPostMetricLookupResult{
		Observations: []domain.XPostMetricObservation{}, Snapshots: []domain.EvidenceSnapshot{}, Diagnostics: []domain.FetchDiagnostic{},
	}
	if err := request.Validate(); err != nil || (connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID) {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X metric lookup request"))
	}
	if !connector.enabled || connector.deleted {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X source connection is unavailable"))
	}
	postIDs := normalizedPostIDs(request.PostIDs)
	if len(postIDs) > connector.resourceLimits.MaxItems {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X metric lookup request exceeds collection item limit"))
	}
	lookupEndpoint := *connector.endpoint
	lookupEndpoint.Path = "/2/tweets"
	lookupEndpoint.RawPath = ""
	parameters := url.Values{
		"ids":          {strings.Join(postIDs, ",")},
		"tweet.fields": {"public_metrics"},
	}
	ctx, cancel := context.WithTimeout(ctx, connector.resourceLimits.WallClockTimeout)
	defer cancel()
	token, err := connector.token()
	if err != nil {
		return result, err
	}
	captured, rateLimit, err := connector.getAt(ctx, &lookupEndpoint, parameters, token, newResponseByteBudget(connector.resourceLimits.MaxCumulativeResponseBytes))
	result.RateLimit = rateLimit
	if err != nil {
		return result, err
	}
	var response searchResponse
	if err := json.Unmarshal(captured.payload, &response); err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("decode X post lookup response"))
	}
	if len(response.Data) > connector.resourceLimits.MaxItems {
		return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X lookup response exceeds collection item limit"))
	}
	snapshot, err := captured.snapshot(metricProfileVersion)
	if err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("capture X post lookup response"))
	}
	result.Snapshots = append(result.Snapshots, snapshot)
	for _, value := range response.Data {
		if !postIDPattern.MatchString(value.ID) {
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "invalid_post"})
			continue
		}
		result.Observations = append(result.Observations, domain.XPostMetricObservation{
			PostID: value.ID, Metrics: mapMetrics(value.PublicMetrics), CapturedAt: captured.capturedAt,
		})
	}
	for _, problem := range response.Errors {
		externalID := safePostID(problem.ResourceID)
		if externalID == "" {
			externalID = safePostID(problem.Value)
		}
		result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "unavailable_post", SourceExternalID: externalID})
	}
	sort.Slice(result.Observations, func(left, right int) bool {
		return comparePostID(result.Observations[left].PostID, result.Observations[right].PostID) < 0
	})
	sort.Slice(result.Diagnostics, func(left, right int) bool {
		if result.Diagnostics[left].Code != result.Diagnostics[right].Code {
			return result.Diagnostics[left].Code < result.Diagnostics[right].Code
		}
		return comparePostID(result.Diagnostics[left].SourceExternalID, result.Diagnostics[right].SourceExternalID) < 0
	})
	return result, nil
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.now().UTC()
	if err := connector.Validate(ctx, connection); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: "invalid_source_connection"}
	}
	ctx, cancel := context.WithTimeout(ctx, connector.resourceLimits.WallClockTimeout)
	defer cancel()
	token, err := connector.token()
	if err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorAuthentication, DiagnosticCode: "credential_unavailable"}
	}
	parameters := url.Values{"query": {"from:XDevelopers"}, "max_results": {"10"}, "sort_order": {"recency"}}
	if _, _, err := connector.get(ctx, parameters, token, newResponseByteBudget(connector.resourceLimits.MaxCumulativeResponseBytes)); err != nil {
		code := "request_failed"
		if domain.ClassifyCollectionError(err) == domain.CollectionErrorAuthentication {
			code = "credential_unavailable"
		}
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: code}
	}
	return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
}

func (connector *Connector) token() (string, error) {
	name := strings.TrimPrefix(connector.credentialRef, "env:")
	token, ok := connector.lookupEnv(name)
	if !ok || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return "", domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("X credential is unavailable"))
	}
	return token, nil
}

func (connector *Connector) get(ctx context.Context, parameters url.Values, token string, byteBudget *responseByteBudget) (fetchedJSONResponse, domain.RateLimit, error) {
	return connector.getAt(ctx, connector.endpoint, parameters, token, byteBudget)
}

func (connector *Connector) getAt(ctx context.Context, endpoint *url.URL, parameters url.Values, token string, byteBudget *responseByteBudget) (fetchedJSONResponse, domain.RateLimit, error) {
	for attempt := 0; ; attempt++ {
		if err := reserveXRequest(ctx, connector.requestBudget, connector.sourceID, connector.resourceLimits, connector.now); err != nil {
			var quota requestQuotaError
			if errors.As(err, &quota) {
				resetAt := quota.resetAt.UTC()
				return fetchedJSONResponse{}, domain.RateLimit{Remaining: 0, ResetAt: &resetAt, RetryAfter: &resetAt}, domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("X daily request quota exceeded"))
			}
			return fetchedJSONResponse{}, domain.RateLimit{}, err
		}
		captured, rateLimit, err := connector.doRequest(ctx, endpoint, parameters, token, byteBudget)
		if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary || attempt >= connector.resourceLimits.MaxRetries {
			return captured, rateLimit, err
		}
		if err := connector.retryWait(ctx, attempt+1); err != nil {
			return fetchedJSONResponse{}, rateLimit, domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("X retry interrupted"))
		}
	}
}

func (connector *Connector) doRequest(ctx context.Context, endpoint *url.URL, parameters url.Values, token string, byteBudget *responseByteBudget) (fetchedJSONResponse, domain.RateLimit, error) {
	target := *endpoint
	target.RawQuery = parameters.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fetchedJSONResponse{}, domain.RateLimit{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("create X request"))
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := connector.http.Do(request)
	if err != nil {
		var quota requestQuotaError
		if errors.As(err, &quota) {
			resetAt := quota.resetAt.UTC()
			return fetchedJSONResponse{}, domain.RateLimit{Remaining: 0, ResetAt: &resetAt, RetryAfter: &resetAt}, domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("X daily request quota exceeded"))
		}
		return fetchedJSONResponse{}, domain.RateLimit{}, requestError(err)
	}
	rateLimit := parseRateLimit(response.Header)
	payload, readErr := byteBudget.readResponse(ctx, response.Body, response.Header.Get("Content-Encoding"))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		if errors.Is(readErr, errResponseByteLimit) {
			return fetchedJSONResponse{}, rateLimit, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X response exceeds cumulative byte limit"))
		}
		if errors.Is(readErr, sourcenet.ErrResponseBodyRead) || errors.Is(readErr, context.Canceled) || closeErr != nil {
			return fetchedJSONResponse{}, rateLimit, domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("read X response"))
		}
		return fetchedJSONResponse{}, rateLimit, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X compressed response is not permitted"))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		status := response.StatusCode
		if status == http.StatusTooManyRequests && rateLimit.ResetAt != nil {
			rateLimit.RetryAfter = cloneTime(rateLimit.ResetAt)
		}
		return fetchedJSONResponse{}, rateLimit, statusError(status)
	}
	return fetchedJSONResponse{
		payload: payload, statusCode: response.StatusCode, requestedURL: target.String(),
		finalURL: response.Request.URL.String(), redirectChain: evidencecapture.RedirectChain(target.String(), response.Request),
		headers: response.Header.Clone(), capturedAt: connector.now().UTC(),
	}, rateLimit, nil
}

type requestQuotaError struct{ resetAt time.Time }

func (err requestQuotaError) Error() string { return "X daily request quota exceeded" }

func reserveXRequest(ctx context.Context, budget domain.ExternalRequestBudget, sourceID int64, profile ResourceLimitProfile, now func() time.Time) error {
	decision, err := budget.ReserveExternalRequest(ctx, domain.ExternalRequestBudgetReservation{
		SourceConnectionID: sourceID, ResourceProfileVersion: profile.Version, DailyLimit: profile.DailyRequestQuota, At: now().UTC(),
	})
	if err != nil {
		return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("reserve X request budget"))
	}
	if err := decision.Validate(profile.DailyRequestQuota); err != nil {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid X request budget decision"))
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

func normalizedPostIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return comparePostID(result[left], result[right]) < 0 })
	return result
}

func comparePostID(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return strings.Compare(left, right)
}

func (connector *Connector) mapPost(value post, users map[string]string, media map[string]includedMedia, observedAt time.Time) (domain.SourceItem, error) {
	if !postIDPattern.MatchString(value.ID) {
		return domain.SourceItem{}, errors.New("invalid X post ID")
	}
	publishedAt := (*time.Time)(nil)
	if strings.TrimSpace(value.CreatedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, value.CreatedAt)
		if err != nil {
			return domain.SourceItem{}, errors.New("invalid X post time")
		}
		parsed = parsed.UTC()
		publishedAt = &parsed
	}
	username := value.Username
	if !usernamePattern.MatchString(username) {
		username = users[value.AuthorID]
	}
	itemURL := "https://x.com/i/web/status/" + value.ID
	if usernamePattern.MatchString(username) {
		itemURL = "https://x.com/" + username + "/status/" + value.ID
	}
	parentID := parentPostID(value)
	body := value.Text
	if strings.TrimSpace(value.NoteTweet.Text) != "" {
		body = value.NoteTweet.Text
	}
	evidence := domain.EvidenceCompletenessMetadataOnly
	if strings.TrimSpace(body) != "" {
		evidence = domain.EvidenceCompletenessFullBody
	}
	parties := []domain.SourcePartyAssertion{{
		Role: domain.SourcePartyRoleDistributor, Kind: domain.SourcePartyKindOrganization,
		IdentityNamespace: "platform", ExternalID: "x", DisplayName: "X", HomepageURL: "https://x.com",
	}}
	if value.AuthorID != "" && len(value.AuthorID) <= 512 && usernamePattern.MatchString(username) {
		account := domain.SourcePartyAssertion{
			Kind: domain.SourcePartyKindAccount, IdentityNamespace: "x:user", ExternalID: value.AuthorID,
			DisplayName: username, HomepageURL: "https://x.com/" + username,
		}
		account.Role = domain.SourcePartyRoleContentOrigin
		parties = append(parties, account)
		account.Role = domain.SourcePartyRoleAuthor
		parties = append(parties, account)
	}
	return domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: value.ID, ParentExternalID: parentID,
		ContentType: "post", Body: body, Language: value.Language, URL: itemURL,
		Author: username, PublishedAt: publishedAt, ObservedAt: observedAt.UTC(),
		EvidenceCompleteness: evidence, Attachments: mapAttachments(value.Attachments.MediaKeys, media),
		Metrics: mapMetrics(value.PublicMetrics), Parties: parties,
	})
}

func compileSearchQuery(base string, languages, regions []string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("X search query is required")
	}
	for _, character := range base {
		if unicode.IsControl(character) {
			return "", errors.New("X search query contains control characters")
		}
	}
	languages = normalizedFilterValues(languages)
	regions = normalizedFilterValues(regions)
	for _, language := range languages {
		if _, ok := supportedLanguages[language]; !ok {
			return "", errors.New("X search language is unsupported")
		}
	}
	for _, region := range regions {
		if len(region) != 2 || region != strings.ToUpper(region) {
			return "", errors.New("X search region is invalid")
		}
	}
	parts := []string{base}
	if len(languages) > 0 || len(regions) > 0 {
		parts[0] = "(" + base + ")"
	}
	if len(languages) > 0 {
		parts = append(parts, operatorGroup("lang:", languages))
	}
	if len(regions) > 0 {
		parts = append(parts, operatorGroup("place_country:", regions))
	}
	query := strings.Join(parts, " ")
	if utf8.RuneCountInString(query) > maxQueryCharacters {
		return "", errors.New("X search query exceeds the conservative character limit")
	}
	return query, nil
}

func normalizedFilterValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func operatorGroup(prefix string, values []string) string {
	operators := make([]string, 0, len(values))
	for _, value := range values {
		operators = append(operators, prefix+value)
	}
	if len(operators) == 1 {
		return operators[0]
	}
	return "(" + strings.Join(operators, " OR ") + ")"
}

func searchParameters(query string, limit int, cursor searchCursor) url.Values {
	if limit < 10 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	parameters := url.Values{
		"query": {query}, "max_results": {strconv.Itoa(limit)}, "sort_order": {searchSortOrder(cursor)},
		"tweet.fields": {"attachments,author_id,conversation_id,created_at,lang,note_tweet,possibly_sensitive,public_metrics,referenced_tweets,text,withheld"},
		"expansions":   {"attachments.media_keys,author_id,referenced_tweets.id"},
		"user.fields":  {"name,username"}, "media.fields": {"media_key,preview_image_url,type,url"},
	}
	if cursor.SinceID != "" {
		parameters.Set("since_id", cursor.SinceID)
	}
	if cursor.NextToken != "" {
		parameters.Set("next_token", cursor.NextToken)
	}
	return parameters
}

func searchSortOrder(cursor searchCursor) string {
	if cursor.SortOrder == "relevancy" || cursor.SortOrder == "recency" {
		return cursor.SortOrder
	}
	if !cursor.Initialized && cursor.SinceID == "" && cursor.NextToken == "" {
		return "relevancy"
	}
	return "recency"
}

func encodeCursor(cursor searchCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return encodeCursorPayload(payload), nil
}

func encodeCursorPayload(payload []byte) string {
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(value string) (searchCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(payload) == 0 || len(payload) > 8*1024 {
		return searchCursor{}, errors.New("invalid X cursor encoding")
	}
	var cursor searchCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != cursorVersion {
		return searchCursor{}, errors.New("invalid X cursor payload")
	}
	if (cursor.SinceID != "" && !postIDPattern.MatchString(cursor.SinceID)) ||
		(cursor.HighWaterID != "" && !postIDPattern.MatchString(cursor.HighWaterID)) ||
		(cursor.NextToken != "" && (!validOpaqueToken(cursor.NextToken) || cursor.HighWaterID == "")) ||
		(cursor.NextToken == "" && cursor.HighWaterID != "") ||
		(cursor.SortOrder != "" && cursor.SortOrder != "recency" && cursor.SortOrder != "relevancy") ||
		(cursor.NextToken == "" && cursor.SortOrder != "") {
		return searchCursor{}, errors.New("invalid X cursor state")
	}
	return cursor, nil
}

func validOpaqueToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 4096 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func parentPostID(value post) string {
	references := append(append([]postReference(nil), value.ReferencedPosts...), value.ReferencedTweets...)
	for _, reference := range references {
		if reference.Type == "replied_to" && postIDPattern.MatchString(reference.ID) && reference.ID != value.ID {
			return reference.ID
		}
	}
	for _, reference := range references {
		if postIDPattern.MatchString(reference.ID) && reference.ID != value.ID {
			return reference.ID
		}
	}
	if postIDPattern.MatchString(value.ConversationID) && value.ConversationID != value.ID {
		return value.ConversationID
	}
	return ""
}

func mapAttachments(keys []string, media map[string]includedMedia) []domain.SourceAttachment {
	attachments := make([]domain.SourceAttachment, 0, len(keys))
	for _, key := range keys {
		item, ok := media[key]
		if !ok {
			continue
		}
		attachmentURL := strings.TrimSpace(item.URL)
		if attachmentURL == "" {
			attachmentURL = strings.TrimSpace(item.PreviewImageURL)
		}
		parsed, err := url.Parse(attachmentURL)
		if err != nil || parsed == nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			continue
		}
		attachments = append(attachments, domain.SourceAttachment{URL: attachmentURL})
		if len(attachments) == domain.MaxSourceAttachments {
			break
		}
	}
	return attachments
}

func mapMetrics(metrics publicMetrics) domain.SourceMetrics {
	repost := metrics.RepostCount
	if repost == nil {
		repost = metrics.RetweetCount
	}
	return domain.SourceMetrics{
		ViewCount: cloneMetric(metrics.ImpressionCount), LikeCount: cloneMetric(metrics.LikeCount),
		CommentCount: cloneMetric(metrics.ReplyCount), ShareCount: sumMetrics(repost, metrics.QuoteCount),
	}
}

func cloneMetric(value *int64) *int64 {
	if value == nil {
		return nil
	}
	return domain.KnownMetric(*value)
}

func sumMetrics(left, right *int64) *int64 {
	if left == nil && right == nil {
		return nil
	}
	var total int64
	if left != nil {
		total += *left
	}
	if right != nil {
		total += *right
	}
	return domain.KnownMetric(total)
}

func newestPostID(posts []post) string {
	newest := ""
	for _, value := range posts {
		if !postIDPattern.MatchString(value.ID) {
			continue
		}
		if len(value.ID) > len(newest) || (len(value.ID) == len(newest) && value.ID > newest) {
			newest = value.ID
		}
	}
	return newest
}

func safePostID(value string) string {
	value = strings.TrimSpace(value)
	if postIDPattern.MatchString(value) {
		return value
	}
	return ""
}

func parseRateLimit(headers http.Header) domain.RateLimit {
	limit := domain.RateLimit{}
	if remaining, err := strconv.Atoi(strings.TrimSpace(headers.Get("x-rate-limit-remaining"))); err == nil && remaining >= 0 {
		limit.Remaining = remaining
	}
	if reset, err := strconv.ParseInt(strings.TrimSpace(headers.Get("x-rate-limit-reset")), 10, 64); err == nil && reset > 0 {
		value := time.Unix(reset, 0).UTC()
		limit.ResetAt = &value
	}
	return limit
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func requestError(err error) error {
	if errors.Is(err, errUnsafeDestination) || errors.Is(err, errRedirectLimit) {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X destination is not permitted"))
	}
	return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("X request failed"))
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("X authentication failed"))
	case http.StatusTooManyRequests:
		return domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("X rate limited"))
	}
	if status >= http.StatusInternalServerError {
		return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("X upstream unavailable"))
	}
	return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X upstream rejected request"))
}

func sameOfficialEndpoint(endpoint, candidate *url.URL) bool {
	return endpoint != nil && candidate != nil && candidate.Scheme == "https" &&
		strings.EqualFold(candidate.Hostname(), endpoint.Hostname()) &&
		(candidate.Port() == "" || candidate.Port() == "443") && candidate.Path == endpoint.Path
}

func secureDialContext(resolver lookupIPAddrFunc, dialContext func(context.Context, string, string) (net.Conn, error), timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		host, port, err := net.SplitHostPort(address)
		if err != nil || network != "tcp" || port != "443" {
			return nil, errUnsafeDestination
		}
		addresses, err := resolver(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errUnsafeDestination
		}
		for _, address := range addresses {
			if !publicAddress(address.IP) {
				return nil, errUnsafeDestination
			}
		}
		var dialErr error
		for _, address := range addresses {
			connection, err := dialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
			if err == nil {
				return connection, nil
			}
			dialErr = err
		}
		if dialErr != nil {
			return nil, dialErr
		}
		return nil, errUnsafeDestination
	}
}

func minimumDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func publicAddress(value net.IP) bool {
	address, ok := netip.AddrFromSlice(value)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, blocked := range blockedAddressRanges {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

var blockedAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

var supportedLanguages = map[string]struct{}{
	"am": {}, "ar": {}, "bg": {}, "bn": {}, "ca": {}, "cs": {}, "da": {}, "de": {},
	"el": {}, "en": {}, "es": {}, "et": {}, "eu": {}, "fa": {}, "fi": {}, "fr": {},
	"gu": {}, "hi": {}, "hr": {}, "hu": {}, "hy": {}, "in": {}, "it": {}, "iw": {},
	"ja": {}, "ka": {}, "kn": {}, "ko": {}, "lt": {}, "lv": {}, "ml": {}, "mr": {},
	"no": {}, "pl": {}, "pt": {}, "ro": {}, "ru": {}, "sk": {}, "sl": {}, "sr": {},
	"sv": {}, "ta": {}, "te": {}, "th": {}, "tr": {}, "uk": {}, "ur": {}, "vi": {},
	"zh-CN": {}, "zh-TW": {},
}
