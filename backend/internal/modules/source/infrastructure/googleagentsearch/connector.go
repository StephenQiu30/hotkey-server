// Package googleagentsearch collects keyword results from the official Google
// Agent Search Discovery Engine v1 API. It never requests Google Search pages.
package googleagentsearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
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
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const (
	maxResponseBodyBytes = 4 << 20
	maxQueryCharacters   = 256
	maxPageSize          = 25
	cursorVersion        = 1
)

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

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
	servingConfig string
	credentialRef string
	enabled       bool
	deleted       bool
	http          *http.Client
	now           func() time.Time
	lookupEnv     func(string) (string, bool)
}

type searchRequest struct {
	Query             string            `json:"query"`
	PageSize          int               `json:"pageSize"`
	PageToken         string            `json:"pageToken,omitempty"`
	SafeSearch        bool              `json:"safeSearch"`
	ContentSearchSpec contentSearchSpec `json:"contentSearchSpec"`
}

type contentSearchSpec struct {
	SnippetSpec snippetSpec `json:"snippetSpec"`
}

type snippetSpec struct {
	ReturnSnippet bool `json:"returnSnippet"`
}

type searchResponse struct {
	Results       []searchResult `json:"results"`
	NextPageToken string         `json:"nextPageToken"`
}

type searchResult struct {
	ID       string         `json:"id"`
	Document searchDocument `json:"document"`
}

type searchDocument struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	DerivedStructData derivedStructData `json:"derivedStructData"`
}

type derivedStructData struct {
	Link      string    `json:"link"`
	Title     string    `json:"title"`
	HTMLTitle string    `json:"htmlTitle"`
	Snippets  []snippet `json:"snippets"`
}

type snippet struct {
	Snippet       string `json:"snippet"`
	SnippetStatus string `json:"snippetStatus"`
	LegacyStatus  string `json:"snippet_status"`
}

type searchCursor struct {
	Version        int    `json:"v"`
	PageToken      string `json:"t"`
	QuerySignature string `json:"q"`
}

func New(connection domain.SourceConnection, resolvers ...sourcenet.Resolver) (*Connector, error) {
	options := connectorOptions{}
	if len(resolvers) > 0 && resolvers[0] != nil {
		options.resolver = resolvers[0].LookupIPAddr
	}
	return newConnector(connection, options)
}

func newConnector(connection domain.SourceConnection, options connectorOptions) (*Connector, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeGoogleAgentSearch {
		return nil, permanent("invalid Google Agent Search source connection")
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
		DialContext: secureDialContext(endpoint.Hostname(), options.resolver, options.dialContext),
	}
	client := &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("Google Agent Search redirects are not allowed")
	}}
	return &Connector{
		sourceID: normalized.ID, endpoint: endpoint, servingConfig: normalized.Config.GoogleServingConfig,
		credentialRef: normalized.CredentialRef, enabled: normalized.Enabled, deleted: normalized.Deleted,
		http: client, now: options.now, lookupEnv: options.lookupEnv,
	}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeGoogleAgentSearch || normalized.Endpoint != connector.endpoint.String() || normalized.Config.GoogleServingConfig != connector.servingConfig || normalized.CredentialRef != connector.credentialRef || (connector.sourceID > 0 && normalized.ID != connector.sourceID) {
		return permanent("Google Agent Search source connection does not match connector")
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
	var response searchResponse
	_, err = connector.search(ctx, token, searchRequest{Query: "HotKey connectivity check", PageSize: 1, SafeSearch: true, ContentSearchSpec: contentSearchSpec{SnippetSpec: snippetSpec{ReturnSnippet: false}}}, &response)
	if err != nil {
		return healthFailure(checkedAt, err)
	}
	return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	if err := request.Validate(); err != nil || (connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID) {
		return result, permanent("invalid Google Agent Search fetch request")
	}
	if !connector.enabled || connector.deleted {
		return result, permanent("Google Agent Search source connection is unavailable")
	}
	token, err := connector.token()
	if err != nil {
		return result, err
	}
	query, err := normalizeQuery(request.Query)
	if err != nil {
		return result, permanent("invalid Google Agent Search query")
	}
	cursor, err := decodeCursor(request.RequestCursor, request.QuerySignature)
	if err != nil {
		return result, permanent("invalid Google Agent Search cursor")
	}
	pageSize := request.Limit
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	var response searchResponse
	result.RateLimit, err = connector.search(ctx, token, searchRequest{
		Query: query, PageSize: pageSize, PageToken: cursor.PageToken, SafeSearch: true,
		ContentSearchSpec: contentSearchSpec{SnippetSpec: snippetSpec{ReturnSnippet: true}},
	}, &response)
	if err != nil {
		return result, err
	}
	for _, value := range response.Results {
		item, err := connector.mapResult(value)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "invalid_google_agent_search_result", SourceExternalID: stableID(value)})
			continue
		}
		result.Items = append(result.Items, item)
	}
	if response.NextPageToken != "" {
		result.HasMore = true
		result.NextCursor = encodeCursor(searchCursor{PageToken: response.NextPageToken, QuerySignature: request.QuerySignature})
	}
	return result, nil
}

func (connector *Connector) search(ctx context.Context, token string, body searchRequest, output *searchResponse) (domain.RateLimit, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.RateLimit{}, permanent("encode Google Agent Search request")
	}
	target := *connector.endpoint
	target.Path = "/v1/" + connector.servingConfig + ":search"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		return domain.RateLimit{}, permanent("build Google Agent Search request")
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := connector.http.Do(httpRequest)
	if err != nil {
		return domain.RateLimit{}, temporary("request Google Agent Search")
	}
	defer response.Body.Close()
	rateLimit := parseRateLimit(response.Header)
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil {
		return rateLimit, temporary("read Google Agent Search response")
	}
	if len(responseBody) > maxResponseBodyBytes {
		return rateLimit, permanent("Google Agent Search response exceeds body byte limit")
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return rateLimit, authentication("Google Agent Search authorization rejected")
	case response.StatusCode == http.StatusTooManyRequests:
		return rateLimit, domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("Google Agent Search rate limited"))
	case response.StatusCode >= 500:
		return rateLimit, temporary("Google Agent Search service unavailable")
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return rateLimit, permanent("Google Agent Search request rejected")
	}
	if len(responseBody) == 0 || json.Unmarshal(responseBody, output) != nil {
		return rateLimit, parse("decode Google Agent Search response")
	}
	return rateLimit, nil
}

func (connector *Connector) token() (string, error) {
	name := strings.TrimPrefix(connector.credentialRef, "env:")
	token, ok := connector.lookupEnv(name)
	token = strings.TrimSpace(token)
	if !ok || token == "" || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return "", authentication("Google Agent Search credential is unavailable")
	}
	return token, nil
}

func (connector *Connector) mapResult(value searchResult) (domain.SourceItem, error) {
	externalID := stableID(value)
	if externalID == "" || utf8.RuneCountInString(externalID) > 512 || containsControl(externalID) {
		return domain.SourceItem{}, errors.New("invalid Google Agent Search document ID")
	}
	itemURL := strings.TrimSpace(value.Document.DerivedStructData.Link)
	if !safeResultURL(itemURL) {
		return domain.SourceItem{}, errors.New("invalid Google Agent Search result URL")
	}
	title := cleanText(value.Document.DerivedStructData.Title)
	if title == "" {
		title = cleanText(value.Document.DerivedStructData.HTMLTitle)
	}
	body := ""
	for _, candidate := range value.Document.DerivedStructData.Snippets {
		status := strings.ToUpper(strings.TrimSpace(candidate.SnippetStatus))
		if status == "" {
			status = strings.ToUpper(strings.TrimSpace(candidate.LegacyStatus))
		}
		if status == "" || status == "SUCCESS" {
			body = cleanText(candidate.Snippet)
			if body != "" {
				break
			}
		}
	}
	evidence := domain.EvidenceCompletenessMetadataOnly
	if body != "" {
		evidence = domain.EvidenceCompletenessSummaryOnly
	}
	return domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: "google_agent_search", ExternalID: externalID, ContentType: "search_result",
		Title: title, Body: body, URL: itemURL, ObservedAt: connector.now().UTC(), EvidenceCompleteness: evidence,
	})
}

func stableID(value searchResult) string {
	for _, candidate := range []string{value.Document.ID, value.ID} {
		if normalized := strings.TrimSpace(candidate); normalized != "" {
			return normalized
		}
	}
	name := strings.TrimSpace(value.Document.Name)
	if index := strings.LastIndex(name, "/documents/"); index >= 0 {
		return strings.TrimSpace(name[index+len("/documents/"):])
	}
	return ""
}

func normalizeQuery(value string) (string, error) {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" || utf8.RuneCountInString(value) > maxQueryCharacters || containsControl(value) {
		return "", errors.New("Google Agent Search query is invalid")
	}
	return value, nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func cleanText(value string) string {
	value = html.UnescapeString(htmlTagPattern.ReplaceAllString(value, ""))
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func safeResultURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == ""
}

func encodeCursor(value searchCursor) string {
	value.Version = cursorVersion
	payload, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(raw, querySignature string) (searchCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return searchCursor{Version: cursorVersion, QuerySignature: querySignature}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return searchCursor{}, err
	}
	var value searchCursor
	if json.Unmarshal(payload, &value) != nil || value.Version != cursorVersion || value.PageToken == "" || len(value.PageToken) > 4096 || containsControl(value.PageToken) || value.QuerySignature != querySignature {
		return searchCursor{}, errors.New("invalid cursor")
	}
	return value, nil
}

func secureDialContext(host string, resolver lookupIPAddrFunc, dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		requestHost, port, err := net.SplitHostPort(address)
		if err != nil || network != "tcp" || requestHost != host || port != "443" {
			return nil, errors.New("unsafe Google Agent Search destination")
		}
		addresses, err := resolver(ctx, requestHost)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("resolve Google Agent Search destination")
		}
		for _, candidate := range addresses {
			if !publicAddress(candidate.IP) {
				return nil, errors.New("unsafe Google Agent Search address")
			}
		}
		var dialErr error
		for _, candidate := range addresses {
			connection, err := dial(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			if err == nil {
				return connection, nil
			}
			dialErr = err
		}
		return nil, dialErr
	}
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

func parseRateLimit(header http.Header) domain.RateLimit {
	result := domain.RateLimit{}
	if remaining, err := strconv.Atoi(header.Get("X-RateLimit-Remaining")); err == nil {
		result.Remaining = remaining
	}
	if seconds, err := strconv.Atoi(header.Get("Retry-After")); err == nil && seconds > 0 {
		retry := time.Now().UTC().Add(time.Duration(seconds) * time.Second)
		result.RetryAfter = &retry
	}
	return result
}

func healthFailure(checkedAt time.Time, err error) domain.HealthResult {
	kind := domain.ClassifyCollectionError(err)
	code := "request_failed"
	if kind == domain.CollectionErrorAuthentication {
		code = "credential_unavailable"
	} else if kind == domain.CollectionErrorPermanent || kind == domain.CollectionErrorParse {
		code = "capability_unavailable"
	}
	return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: kind, DiagnosticCode: code}
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
