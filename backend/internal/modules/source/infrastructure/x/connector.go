package x

import (
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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const (
	sourceCode           = "x"
	maxQueryCharacters   = 512
	maxResponseBodyBytes = 4 << 20
	maxRedirects         = 3
	cursorVersion        = 1
)

var (
	errUnsafeDestination = errors.New("unsafe X destination")
	errRedirectLimit     = errors.New("X redirect limit exceeded")
	postIDPattern        = regexp.MustCompile(`^[0-9]{1,19}$`)
	usernamePattern      = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}$`)
)

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

type searchCursor struct {
	Version     int    `json:"v"`
	SinceID     string `json:"s,omitempty"`
	NextToken   string `json:"n,omitempty"`
	HighWaterID string `json:"h,omitempty"`
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
	PublicMetrics     publicMetrics   `json:"public_metrics"`
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

func New(connection domain.SourceConnection, resolvers ...sourcenet.Resolver) (*Connector, error) {
	options := connectorOptions{}
	if len(resolvers) > 0 && resolvers[0] != nil {
		options.resolver = resolvers[0].LookupIPAddr
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
	client := &http.Client{
		Timeout: timeout, Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errRedirectLimit
			}
			if !sameOfficialEndpoint(endpoint, request.URL) {
				return errUnsafeDestination
			}
			return nil
		},
	}
	return &Connector{
		sourceID: normalized.ID, endpoint: endpoint, credentialRef: normalized.CredentialRef,
		enabled: normalized.Enabled, deleted: normalized.Deleted, http: client,
		now: options.now, lookupEnv: options.lookupEnv,
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
	token, err := connector.token()
	if err != nil {
		return result, err
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
	parameters := searchParameters(query, request.Limit, cursor)
	payload, rateLimit, err := connector.get(ctx, parameters, token)
	result.RateLimit = rateLimit
	if err != nil {
		return result, err
	}
	var response searchResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("decode X recent search response"))
	}
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
	for _, sourcePost := range response.Data {
		switch {
		case sourcePost.PossiblySensitive:
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "possibly_sensitive_post", SourceExternalID: safePostID(sourcePost.ID)})
			continue
		case len(sourcePost.Withheld) > 0 && string(sourcePost.Withheld) != "null":
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "withheld_post", SourceExternalID: safePostID(sourcePost.ID)})
			continue
		}
		item, err := connector.mapPost(sourcePost, users, media)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "invalid_post", SourceExternalID: safePostID(sourcePost.ID)})
			continue
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
		result.NextCursor, err = encodeCursor(searchCursor{Version: cursorVersion, SinceID: finalSince})
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("encode X search cursor"))
		}
	}
	return result, nil
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.now().UTC()
	if err := connector.Validate(ctx, connection); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: "invalid_source_connection"}
	}
	token, err := connector.token()
	if err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorAuthentication, DiagnosticCode: "credential_unavailable"}
	}
	parameters := url.Values{"query": {"from:XDevelopers"}, "max_results": {"10"}, "sort_order": {"recency"}}
	if _, _, err := connector.get(ctx, parameters, token); err != nil {
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

func (connector *Connector) get(ctx context.Context, parameters url.Values, token string) ([]byte, domain.RateLimit, error) {
	target := *connector.endpoint
	target.RawQuery = parameters.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, domain.RateLimit{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("create X request"))
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := connector.http.Do(request)
	if err != nil {
		return nil, domain.RateLimit{}, requestError(err)
	}
	rateLimit := parseRateLimit(response.Header)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		status := response.StatusCode
		closeResponse(response)
		if status == http.StatusTooManyRequests && rateLimit.ResetAt != nil {
			rateLimit.RetryAfter = cloneTime(rateLimit.ResetAt)
		}
		return nil, rateLimit, statusError(status)
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, rateLimit, domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("read X response"))
	}
	if len(payload) > maxResponseBodyBytes {
		return nil, rateLimit, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("X response exceeds body byte limit"))
	}
	return payload, rateLimit, nil
}

func (connector *Connector) mapPost(value post, users map[string]string, media map[string]includedMedia) (domain.SourceItem, error) {
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
	evidence := domain.EvidenceCompletenessMetadataOnly
	if strings.TrimSpace(value.Text) != "" {
		evidence = domain.EvidenceCompletenessFullBody
	}
	return domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: value.ID, ParentExternalID: parentID,
		ContentType: "post", Body: value.Text, Language: value.Language, URL: itemURL,
		Author: username, PublishedAt: publishedAt, ObservedAt: connector.now().UTC(),
		EvidenceCompleteness: evidence, Attachments: mapAttachments(value.Attachments.MediaKeys, media),
		Metrics: mapMetrics(value.PublicMetrics),
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
		"query": {query}, "max_results": {strconv.Itoa(limit)}, "sort_order": {"recency"},
		"post.fields": {"attachments,author_id,conversation_id,created_at,lang,possibly_sensitive,public_metrics,referenced_posts,text,withheld"},
		"expansions":  {"attachments.media_keys,author_id,referenced_posts"},
		"user.fields": {"name,username"}, "media.fields": {"media_key,preview_image_url,type,url"},
	}
	if cursor.SinceID != "" {
		parameters.Set("since_id", cursor.SinceID)
	}
	if cursor.NextToken != "" {
		parameters.Set("next_token", cursor.NextToken)
	}
	return parameters
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
		(cursor.NextToken == "" && cursor.HighWaterID != "") {
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

func closeResponse(response *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes+1))
	_ = response.Body.Close()
}

func sameOfficialEndpoint(endpoint, candidate *url.URL) bool {
	return endpoint != nil && candidate != nil && candidate.Scheme == "https" &&
		strings.EqualFold(candidate.Hostname(), endpoint.Hostname()) &&
		(candidate.Port() == "" || candidate.Port() == "443") && candidate.Path == endpoint.Path
}

func secureDialContext(resolver lookupIPAddrFunc, dialContext func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
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
