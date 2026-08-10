// Package weibo implements keyword collection through the official Weibo CLI
// API. It never calls public web pages, mobile endpoints, or undocumented APIs.
package weibo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
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
	commandGroup            = "search"
	commandAction           = "statuses/limited"
	collectorProfileVersion = "weibo-cli-search-json-v1"
	maxResponseBodyBytes    = 4 << 20
	maxQueryCharacters      = 120
	maxPageSize             = 50
	cursorVersion           = 1
)

var postIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

type connectorOptions struct {
	resolver    lookupIPAddrFunc
	dialContext func(context.Context, string, string) (net.Conn, error)
	tlsConfig   *tls.Config
	now         func() time.Time
	lookupEnv   func(string) (string, bool)
}

type Connector struct {
	sourceID      int64
	endpoint      *url.URL
	credentialRef string
	enabled       bool
	deleted       bool
	http          *http.Client
	now           func() time.Time
	lookupEnv     func(string) (string, bool)
}

type commandCatalog struct {
	Commands []commandDefinition `json:"commands"`
}

type commandDefinition struct {
	Group  string `json:"group"`
	Action string `json:"action"`
	Access string `json:"access"`
}

type invokeEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  *apiError       `json:"error"`
}

type apiError struct {
	Code string `json:"code"`
}

type invokeRequest struct {
	Group  string            `json:"group"`
	Action string            `json:"action"`
	Args   map[string]string `json:"args"`
}

type searchCursor struct {
	Version        int    `json:"v"`
	Page           int    `json:"p"`
	QuerySignature string `json:"q"`
}

type searchPage struct {
	Statuses    []post          `json:"statuses"`
	Data        json.RawMessage `json:"data"`
	Items       []post          `json:"items"`
	HasMore     bool            `json:"has_more"`
	NextCursor  json.RawMessage `json:"next_cursor"`
	TotalNumber int             `json:"total_number"`
}

type post struct {
	ID              json.RawMessage `json:"id"`
	IDStr           string          `json:"idstr"`
	MblogID         string          `json:"mblogid"`
	Text            string          `json:"text"`
	TextRaw         string          `json:"text_raw"`
	CreatedAt       string          `json:"created_at"`
	URL             string          `json:"url"`
	Deleted         bool            `json:"deleted"`
	IsDeleted       bool            `json:"is_deleted"`
	Visible         json.RawMessage `json:"visible"`
	RepostsCount    int64           `json:"reposts_count"`
	CommentsCount   int64           `json:"comments_count"`
	AttitudesCount  int64           `json:"attitudes_count"`
	User            user            `json:"user"`
	RetweetedStatus *post           `json:"retweeted_status"`
}

type user struct {
	ID         json.RawMessage `json:"id"`
	IDStr      string          `json:"idstr"`
	ScreenName string          `json:"screen_name"`
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

func (value fetchedJSONResponse) snapshot() (domain.EvidenceSnapshot, error) {
	return evidencecapture.NewJSONSnapshot(value.payload, collectorProfileVersion, value.requestedURL, value.finalURL,
		value.redirectChain, value.statusCode, value.headers, value.capturedAt)
}

func New(connection domain.SourceConnection, resolvers ...sourcenet.Resolver) (*Connector, error) {
	options := connectorOptions{}
	if len(resolvers) > 0 && resolvers[0] != nil {
		options.resolver = resolvers[0].LookupIPAddr
	}
	return newConnector(connection, options)
}

func NewWithCredentialLookup(connection domain.SourceConnection, resolver sourcenet.Resolver, lookup func(string) (string, bool)) (*Connector, error) {
	options := connectorOptions{lookupEnv: lookup}
	if resolver != nil {
		options.resolver = resolver.LookupIPAddr
	}
	return newConnector(connection, options)
}

func newConnector(connection domain.SourceConnection, options connectorOptions) (*Connector, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeWeibo || normalized.Endpoint != domain.WeiboCLIApiEndpoint {
		return nil, permanent("invalid Weibo source connection")
	}
	endpoint, _ := url.Parse(normalized.Endpoint)
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
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if options.tlsConfig != nil {
		tlsConfig = options.tlsConfig.Clone()
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	timeout := time.Duration(normalized.Config.RequestTimeoutSeconds) * time.Second
	transport := &http.Transport{
		Proxy: nil, ForceAttemptHTTP2: true, TLSClientConfig: tlsConfig,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout,
		DialContext: secureDialContext(options.resolver, options.dialContext),
	}
	client := &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 2 || !officialURL(request.URL) {
			return errors.New("unsafe Weibo redirect")
		}
		return nil
	}}
	return &Connector{
		sourceID: normalized.ID, endpoint: endpoint, credentialRef: normalized.CredentialRef,
		enabled: normalized.Enabled, deleted: normalized.Deleted, http: client,
		now: options.now, lookupEnv: options.lookupEnv,
	}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeWeibo || normalized.Endpoint != connector.endpoint.String() || normalized.CredentialRef != connector.credentialRef || (connector.sourceID > 0 && normalized.ID != connector.sourceID) {
		return permanent("Weibo source connection does not match connector")
	}
	return nil
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.now().UTC()
	if err := connector.Validate(ctx, connection); err != nil || connector.deleted {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorPermanent, DiagnosticCode: "invalid_source_connection"}
	}
	token, err := connector.token()
	if err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorAuthentication, DiagnosticCode: "credential_unavailable"}
	}
	var identity map[string]any
	if _, _, err := connector.request(ctx, http.MethodGet, "/cli/whoami", nil, nil, token, &identity); err != nil || len(identity) == 0 {
		return healthFailure(checkedAt, err, "credential_unavailable")
	}
	if err := connector.requireSearchCapability(ctx, token); err != nil {
		return healthFailure(checkedAt, err, "capability_unavailable")
	}
	return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	if err := request.Validate(); err != nil || (connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID) {
		return result, permanent("invalid Weibo fetch request")
	}
	if !connector.enabled || connector.deleted {
		return result, permanent("Weibo source connection is unavailable")
	}
	token, err := connector.token()
	if err != nil {
		return result, err
	}
	if err := connector.requireSearchCapability(ctx, token); err != nil {
		return result, err
	}
	query, err := normalizeQuery(request.Query)
	if err != nil {
		return result, permanent("invalid Weibo search query")
	}
	checkpoint, err := decodeCursor(request.RequestCursor, request.QuerySignature)
	if err != nil {
		return result, permanent("invalid Weibo search cursor")
	}
	count := request.Limit
	if count > maxPageSize {
		count = maxPageSize
	}
	body := invokeRequest{Group: commandGroup, Action: commandAction, Args: map[string]string{
		"q": query, "count": strconv.Itoa(count), "page": strconv.Itoa(checkpoint.Page),
	}}
	var envelope invokeEnvelope
	captured, rateLimit, err := connector.request(ctx, http.MethodPost, "/cli/invoke", nil, body, token, &envelope)
	result.RateLimit = rateLimit
	if err != nil {
		return result, err
	}
	if envelope.Error != nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return result, classifyAPIError(envelope.Error)
	}
	page, err := decodeSearchPage(envelope.Result)
	if err != nil {
		return result, parse("decode Weibo search result")
	}
	snapshot, err := captured.snapshot()
	if err != nil {
		return result, parse("capture Weibo search response")
	}
	result.Snapshots = append(result.Snapshots, snapshot)
	for index, value := range page.Statuses {
		externalID := value.stableID()
		if value.unavailable() {
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "unavailable_weibo_post", SourceExternalID: externalID})
			continue
		}
		mapped, err := connector.mapPost(value, captured.capturedAt)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "invalid_weibo_post", SourceExternalID: externalID})
			continue
		}
		pointer, pointerErr := weiboResultPointer(envelope.Result, index)
		if pointerErr != nil || evidencecapture.BindJSONPointer(&mapped, snapshot, "/result"+pointer, domain.EvidenceUsageDocumentSource) != nil {
			return domain.FetchResult{}, parse("bind Weibo post evidence")
		}
		result.Items = append(result.Items, mapped)
	}
	result.HasMore = page.HasMore || hasNextCursor(page.NextCursor) || len(page.Statuses) >= count
	if result.HasMore {
		checkpoint.Page++
		result.NextCursor = encodeCursor(checkpoint)
	}
	return result, nil
}

func (connector *Connector) requireSearchCapability(ctx context.Context, token string) error {
	query := url.Values{"group": {commandGroup}, "access": {"available"}}
	var catalog commandCatalog
	if _, _, err := connector.request(ctx, http.MethodGet, "/cli/commands", query, nil, token, &catalog); err != nil {
		return err
	}
	for _, command := range catalog.Commands {
		if command.Group == commandGroup && command.Action == commandAction && command.Access != "locked" {
			return nil
		}
	}
	return authentication("required Weibo search capability is unavailable")
}

func (connector *Connector) request(ctx context.Context, method, path string, query url.Values, body any, token string, output any) (fetchedJSONResponse, domain.RateLimit, error) {
	target := *connector.endpoint
	target.Path = strings.TrimSuffix(connector.endpoint.Path, "/") + path
	if query != nil {
		target.RawQuery = query.Encode()
	}
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fetchedJSONResponse{}, domain.RateLimit{}, permanent("encode Weibo request")
		}
		payload = bytes.NewReader(encoded)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, target.String(), payload)
	if err != nil {
		return fetchedJSONResponse{}, domain.RateLimit{}, permanent("build Weibo request")
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	response, err := connector.http.Do(httpRequest)
	if err != nil {
		return fetchedJSONResponse{}, domain.RateLimit{}, temporary("request Weibo CLI API")
	}
	defer response.Body.Close()
	rateLimit := parseRateLimit(response.Header)
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return fetchedJSONResponse{}, rateLimit, temporary("read Weibo response")
	}
	if len(responseBody) > maxResponseBodyBytes {
		return fetchedJSONResponse{}, rateLimit, permanent("Weibo response exceeds body byte limit")
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusPaymentRequired:
		return fetchedJSONResponse{}, rateLimit, authentication("Weibo authorization rejected")
	case response.StatusCode == http.StatusTooManyRequests:
		return fetchedJSONResponse{}, rateLimit, domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("Weibo rate limited"))
	case response.StatusCode >= 500:
		return fetchedJSONResponse{}, rateLimit, temporary("Weibo service unavailable")
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return fetchedJSONResponse{}, rateLimit, permanent("Weibo request rejected")
	}
	if len(responseBody) == 0 || json.Unmarshal(responseBody, output) != nil {
		return fetchedJSONResponse{}, rateLimit, parse("decode Weibo response")
	}
	return fetchedJSONResponse{
		payload: responseBody, statusCode: response.StatusCode, requestedURL: target.String(),
		finalURL: response.Request.URL.String(), redirectChain: evidencecapture.RedirectChain(target.String(), response.Request),
		headers: response.Header.Clone(), capturedAt: connector.now().UTC(),
	}, rateLimit, nil
}

func (connector *Connector) token() (string, error) {
	name := strings.TrimPrefix(connector.credentialRef, "env:")
	token, ok := connector.lookupEnv(name)
	token = strings.TrimSpace(token)
	if !ok || token == "" || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return "", authentication("Weibo credential is unavailable")
	}
	return token, nil
}

func (connector *Connector) mapPost(value post, observedAt time.Time) (domain.SourceItem, error) {
	id := value.stableID()
	if !postIDPattern.MatchString(id) {
		return domain.SourceItem{}, errors.New("invalid Weibo post ID")
	}
	body := strings.TrimSpace(value.TextRaw)
	if body == "" {
		body = strings.TrimSpace(value.Text)
	}
	publishedAt, err := parsePublishedAt(value.CreatedAt)
	if err != nil {
		return domain.SourceItem{}, err
	}
	itemURL := strings.TrimSpace(value.URL)
	if !officialContentURL(itemURL) {
		itemURL = "https://weibo.com/detail/" + id
	}
	parentID := ""
	if value.RetweetedStatus != nil {
		parentID = value.RetweetedStatus.stableID()
		if !postIDPattern.MatchString(parentID) || parentID == id {
			parentID = ""
		}
	}
	evidence := domain.EvidenceCompletenessMetadataOnly
	if body != "" {
		evidence = domain.EvidenceCompletenessFullBody
	}
	parties := []domain.SourcePartyAssertion{{
		Role: domain.SourcePartyRoleDistributor, Kind: domain.SourcePartyKindOrganization,
		IdentityNamespace: "platform", ExternalID: "weibo", DisplayName: "微博", HomepageURL: "https://weibo.com",
	}}
	userID := value.User.stableID()
	if userID != "" && strings.TrimSpace(value.User.ScreenName) != "" {
		account := domain.SourcePartyAssertion{
			Kind: domain.SourcePartyKindAccount, IdentityNamespace: "weibo:user", ExternalID: userID,
			DisplayName: strings.TrimSpace(value.User.ScreenName),
		}
		account.Role = domain.SourcePartyRoleContentOrigin
		parties = append(parties, account)
		account.Role = domain.SourcePartyRoleAuthor
		parties = append(parties, account)
	}
	return domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: "weibo", ExternalID: id, ParentExternalID: parentID,
		ContentType: "post", Body: body, URL: itemURL, Author: strings.TrimSpace(value.User.ScreenName),
		PublishedAt: publishedAt, ObservedAt: observedAt.UTC(), EvidenceCompleteness: evidence,
		Metrics: domain.SourceMetrics{
			LikeCount: domain.KnownMetric(value.AttitudesCount), CommentCount: domain.KnownMetric(value.CommentsCount), ShareCount: domain.KnownMetric(value.RepostsCount),
		}, Parties: parties,
	})
}

func (value user) stableID() string {
	if postIDPattern.MatchString(strings.TrimSpace(value.IDStr)) {
		return strings.TrimSpace(value.IDStr)
	}
	return rawID(value.ID)
}

func weiboResultPointer(raw json.RawMessage, index int) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || index < 0 {
		return "", errors.New("Weibo result locator is invalid")
	}
	if trimmed[0] == '[' {
		return "/" + strconv.Itoa(index), nil
	}
	var record map[string]json.RawMessage
	if json.Unmarshal(trimmed, &record) != nil {
		return "", errors.New("Weibo result locator is invalid")
	}
	for _, field := range []string{"statuses", "items"} {
		if value := bytes.TrimSpace(record[field]); len(value) > 0 && value[0] == '[' {
			return "/" + field + "/" + strconv.Itoa(index), nil
		}
	}
	if nested := bytes.TrimSpace(record["data"]); len(nested) > 0 && string(nested) != "null" {
		pointer, err := weiboResultPointer(nested, index)
		if err == nil {
			return "/data" + pointer, nil
		}
	}
	return "", errors.New("Weibo result locator did not resolve")
}

func decodeSearchPage(raw json.RawMessage) (searchPage, error) {
	var direct []post
	if len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &direct); err != nil {
			return searchPage{}, err
		}
		return searchPage{Statuses: direct}, nil
	}
	var page searchPage
	if err := json.Unmarshal(raw, &page); err != nil {
		return searchPage{}, err
	}
	if len(page.Statuses) == 0 {
		page.Statuses = page.Items
	}
	if len(page.Statuses) == 0 && len(page.Data) > 0 && string(page.Data) != "null" {
		nested, err := decodeSearchPage(page.Data)
		if err != nil {
			return searchPage{}, err
		}
		page.Statuses, page.HasMore, page.NextCursor = nested.Statuses, nested.HasMore, nested.NextCursor
	}
	return page, nil
}

func (value post) stableID() string {
	if postIDPattern.MatchString(strings.TrimSpace(value.IDStr)) {
		return strings.TrimSpace(value.IDStr)
	}
	return rawID(value.ID)
}

func rawID(raw json.RawMessage) string {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if postIDPattern.MatchString(value) {
		return value
	}
	return ""
}

func (value post) unavailable() bool {
	if value.Deleted || value.IsDeleted {
		return true
	}
	var visible bool
	return len(value.Visible) > 0 && json.Unmarshal(value.Visible, &visible) == nil && !visible
}

func normalizeQuery(value string) (string, error) {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" || utf8.RuneCountInString(value) > maxQueryCharacters {
		return "", errors.New("Weibo query length is invalid")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", errors.New("Weibo query contains control characters")
		}
	}
	return value, nil
}

func parsePublishedAt(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, time.RFC1123Z, "Mon Jan 02 15:04:05 -0700 2006"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			parsed = parsed.UTC()
			return &parsed, nil
		}
	}
	return nil, errors.New("invalid Weibo published time")
}

func encodeCursor(value searchCursor) string {
	value.Version = cursorVersion
	payload, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(raw, querySignature string) (searchCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return searchCursor{Version: cursorVersion, Page: 1, QuerySignature: querySignature}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return searchCursor{}, err
	}
	var value searchCursor
	if json.Unmarshal(payload, &value) != nil || value.Version != cursorVersion || value.Page < 1 || value.Page > 10000 || value.QuerySignature != querySignature {
		return searchCursor{}, errors.New("invalid cursor")
	}
	return value, nil
}

func hasNextCursor(raw json.RawMessage) bool {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	return value != "" && value != "0" && value != "null"
}

func officialURL(value *url.URL) bool {
	return value != nil && value.Scheme == "https" && value.Hostname() == "open.weibo.com" && (value.Port() == "" || value.Port() == "443") && value.User == nil
}

func officialContentURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "weibo.com" || host == "www.weibo.com" || strings.HasSuffix(host, ".weibo.com")
}

func secureDialContext(resolver lookupIPAddrFunc, dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host != "open.weibo.com" || port != "443" {
			return nil, errors.New("unsafe Weibo destination")
		}
		addresses, err := resolver(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("resolve Weibo destination")
		}
		for _, candidate := range addresses {
			parsed, ok := netip.AddrFromSlice(candidate.IP)
			if !ok || !parsed.Unmap().IsGlobalUnicast() || parsed.Unmap().IsPrivate() {
				return nil, errors.New("unsafe Weibo address")
			}
		}
		return dial(ctx, network, address)
	}
}

func parseRateLimit(header http.Header) domain.RateLimit {
	result := domain.RateLimit{}
	if remaining, err := strconv.Atoi(header.Get("X-RateLimit-Remaining")); err == nil {
		result.Remaining = remaining
	}
	if epoch, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64); err == nil && epoch > 0 {
		reset := time.Unix(epoch, 0).UTC()
		result.ResetAt = &reset
	}
	if seconds, err := strconv.Atoi(header.Get("Retry-After")); err == nil && seconds > 0 {
		retry := time.Now().UTC().Add(time.Duration(seconds) * time.Second)
		result.RetryAfter = &retry
	}
	return result
}

func classifyAPIError(value *apiError) error {
	if value == nil {
		return parse("Weibo invoke result is unavailable")
	}
	code := strings.ToUpper(strings.TrimSpace(value.Code))
	if strings.Contains(code, "AUTH") || strings.Contains(code, "TOKEN") || strings.Contains(code, "PLAN") || strings.Contains(code, "SUBSCRIPTION") {
		return authentication("Weibo invoke authorization rejected")
	}
	if strings.Contains(code, "RATE") || strings.Contains(code, "QUOTA") || strings.Contains(code, "CREDIT") {
		return domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("Weibo quota unavailable"))
	}
	return permanent("Weibo invoke rejected")
}

func healthFailure(checkedAt time.Time, err error, fallback string) domain.HealthResult {
	if err == nil {
		err = parse("empty Weibo health response")
	}
	code := fallback
	if domain.ClassifyCollectionError(err) == domain.CollectionErrorTemporary {
		code = "request_failed"
	}
	return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: code}
}

func permanent(message string) error {
	return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New(message))
}
func authentication(message string) error {
	return domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New(message))
}
func temporary(message string) error {
	return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New(message))
}
func parse(message string) error {
	return domain.NewCollectionError(domain.CollectionErrorParse, errors.New(message))
}

var _ domain.Connector = (*Connector)(nil)
