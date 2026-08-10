// Package bilibili implements the official Open Platform authorized-account
// APIs. It deliberately does not call public web pages or undocumented APIs.
package bilibili

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/evidencecapture"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const (
	collectorProfileVersion = "bilibili-open-platform-json-v1"
	maxResponseBodyBytes    = 4 << 20
	cursorVersion           = 1
	pageSize                = 50
)

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

type connectorOptions struct {
	resolver    lookupIPAddrFunc
	dialContext func(context.Context, string, string) (net.Conn, error)
	tlsConfig   *tls.Config
	now         func() time.Time
	nonce       func() string
	lookupEnv   func(string) (string, bool)
}

type Connector struct {
	sourceID              int64
	endpoint              *url.URL
	credentialRef, openID string
	enabled, deleted      bool
	http                  *http.Client
	now                   func() time.Time
	nonce                 func() string
	lookupEnv             func(string) (string, bool)
}

type credentials struct {
	ClientID    string `json:"client_id"`
	AppSecret   string `json:"app_secret"`
	AccessToken string `json:"access_token"`
}

type cursor struct{ Version, VideoPage, ArticlePage int }

type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

type scopeResponse struct {
	OpenID string   `json:"openid"`
	Scopes []string `json:"scopes"`
}

type videoItem struct {
	ResourceID  string `json:"resource_id"`
	Title       string `json:"title"`
	Cover       string `json:"cover"`
	Description string `json:"desc"`
	CTime       int64  `json:"ctime"`
	PTime       int64  `json:"ptime"`
	VideoInfo   struct {
		ShareURL string `json:"share_url"`
	} `json:"video_info"`
}

type articleItem struct {
	ID          int64                                                    `json:"id"`
	Title       string                                                   `json:"title"`
	Summary     string                                                   `json:"summary"`
	BannerURL   string                                                   `json:"banner_url"`
	ImageURLs   []string                                                 `json:"image_urls"`
	PublishTime int64                                                    `json:"publish_time"`
	CTime       int64                                                    `json:"ctime"`
	Stats       struct{ View, Favorite, Like, Reply, Share, Coin int64 } `json:"stats"`
}

type videoPage struct {
	List []videoItem `json:"list"`
	Page struct {
		Total int `json:"total"`
	} `json:"page"`
}

type articlePage struct {
	List []articleItem `json:"list"`
	Page struct {
		Total int `json:"total"`
	} `json:"page"`
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
	if err != nil || normalized.SourceType != domain.SourceTypeBilibili || normalized.Endpoint != domain.BilibiliOpenEndpoint {
		return nil, permanent("invalid Bilibili source connection")
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
	if options.nonce == nil {
		options.nonce = randomNonce
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
	transport := &http.Transport{Proxy: nil, ForceAttemptHTTP2: true, TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout, DialContext: secureDialContext(options.resolver, options.dialContext)}
	client := &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(request *http.Request, _ []*http.Request) error {
		if !officialURL(request.URL) {
			return errors.New("unsafe Bilibili redirect")
		}
		return nil
	}}
	return &Connector{sourceID: normalized.ID, endpoint: endpoint, credentialRef: normalized.CredentialRef, openID: normalized.Config.BilibiliOpenID, enabled: normalized.Enabled, deleted: normalized.Deleted, http: client, now: options.now, nonce: options.nonce, lookupEnv: options.lookupEnv}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeBilibili || normalized.Endpoint != connector.endpoint.String() || normalized.CredentialRef != connector.credentialRef || normalized.Config.BilibiliOpenID != connector.openID || (connector.sourceID > 0 && normalized.ID != connector.sourceID) {
		return permanent("Bilibili source connection does not match connector")
	}
	return nil
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.now().UTC()
	if err := connector.Validate(ctx, connection); err != nil || connector.deleted {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorPermanent, DiagnosticCode: "invalid_source_connection"}
	}
	credential, err := connector.credentials()
	if err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorAuthentication, DiagnosticCode: "credential_unavailable"}
	}
	_, err = connector.authorizedScopes(ctx, credential)
	if err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: healthCode(err)}
	}
	return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	if err := request.Validate(); err != nil || (connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID) {
		return result, permanent("invalid Bilibili fetch request")
	}
	if !connector.enabled || connector.deleted {
		return result, permanent("Bilibili source connection is unavailable")
	}
	credential, err := connector.credentials()
	if err != nil {
		return result, err
	}
	scopes, err := connector.authorizedScopes(ctx, credential)
	if err != nil {
		return result, err
	}
	checkpoint, err := decodeCursor(request.RequestCursor)
	if err != nil {
		return result, permanent("invalid Bilibili cursor")
	}
	if checkpoint.VideoPage == 0 {
		checkpoint.VideoPage = 1
	}
	if checkpoint.ArticlePage == 0 {
		checkpoint.ArticlePage = 1
	}
	hasVideo, hasArticle := scopes["ARC_BASE"], scopes["ATC_BASE"]
	videoMore, articleMore := false, false
	if hasVideo {
		var page videoPage
		captured, err := connector.getCaptured(ctx, credential, "/archive/viewlist", url.Values{"pn": {strconv.Itoa(checkpoint.VideoPage)}, "ps": {strconv.Itoa(pageSize)}, "status": {"all"}}, &page)
		if err != nil {
			return result, err
		}
		snapshot, err := captured.snapshot()
		if err != nil {
			return result, parse("capture Bilibili video response")
		}
		result.Snapshots = append(result.Snapshots, snapshot)
		for index, item := range page.List {
			mapped, ok := mapVideo(item, snapshot.CapturedAt)
			if ok {
				mapped.Parties = connector.accountParties()
				mapped, err = domain.NormalizeSourceItem(mapped)
				if err != nil || evidencecapture.BindJSONPointer(&mapped, snapshot, fmt.Sprintf("/data/list/%d", index), domain.EvidenceUsageDocumentSource) != nil {
					return domain.FetchResult{}, parse("bind Bilibili video evidence")
				}
				result.Items = append(result.Items, mapped)
			} else {
				result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "invalid_bilibili_video"})
			}
		}
		videoMore = checkpoint.VideoPage*pageSize < page.Page.Total
		if videoMore {
			checkpoint.VideoPage++
		} else {
			checkpoint.VideoPage = 1
		}
	}
	if hasArticle {
		var page articlePage
		captured, err := connector.getCaptured(ctx, credential, "/article/list", url.Values{"pn": {strconv.Itoa(checkpoint.ArticlePage)}, "ps": {strconv.Itoa(pageSize)}, "sort": {"publish_time"}}, &page)
		if err != nil {
			return result, err
		}
		snapshot, err := captured.snapshot()
		if err != nil {
			return result, parse("capture Bilibili article response")
		}
		result.Snapshots = append(result.Snapshots, snapshot)
		for index, item := range page.List {
			mapped, ok := mapArticle(item, snapshot.CapturedAt, scopes["ATC_DATA"])
			if ok {
				mapped.Parties = connector.accountParties()
				mapped, err = domain.NormalizeSourceItem(mapped)
				if err != nil || evidencecapture.BindJSONPointer(&mapped, snapshot, fmt.Sprintf("/data/list/%d", index), domain.EvidenceUsageDocumentSource) != nil {
					return domain.FetchResult{}, parse("bind Bilibili article evidence")
				}
				result.Items = append(result.Items, mapped)
			} else {
				result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: "invalid_bilibili_article"})
			}
		}
		articleMore = checkpoint.ArticlePage*pageSize < page.Page.Total
		if articleMore {
			checkpoint.ArticlePage++
		} else {
			checkpoint.ArticlePage = 1
		}
	}
	result.HasMore = videoMore || articleMore
	result.NextCursor = encodeCursor(checkpoint)
	return result, nil
}

func (connector *Connector) authorizedScopes(ctx context.Context, credential credentials) (map[string]bool, error) {
	var response scopeResponse
	if err := connector.get(ctx, credential, "/user/account/scopes", nil, &response); err != nil {
		return nil, err
	}
	if response.OpenID != connector.openID {
		return nil, authentication("authorized Bilibili account does not match")
	}
	scopes := make(map[string]bool, len(response.Scopes))
	for _, scope := range response.Scopes {
		scopes[strings.TrimSpace(scope)] = true
	}
	if !scopes["USER_INFO"] || (!scopes["ARC_BASE"] && !scopes["ATC_BASE"]) {
		return nil, authentication("required Bilibili scopes are unavailable")
	}
	return scopes, nil
}

func (connector *Connector) get(ctx context.Context, credential credentials, path string, query url.Values, output any) error {
	_, err := connector.getCaptured(ctx, credential, path, query, output)
	return err
}

func (connector *Connector) getCaptured(ctx context.Context, credential credentials, path string, query url.Values, output any) (fetchedJSONResponse, error) {
	target := *connector.endpoint
	target.Path = strings.TrimSuffix(connector.endpoint.Path, "/") + path
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fetchedJSONResponse{}, permanent("build Bilibili request")
	}
	addSignature(request, credential, connector.now(), connector.nonce())
	response, err := connector.http.Do(request)
	if err != nil {
		return fetchedJSONResponse{}, temporary("request Bilibili Open Platform")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil || len(body) > maxResponseBodyBytes {
		return fetchedJSONResponse{}, parse("read Bilibili response")
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return fetchedJSONResponse{}, authentication("Bilibili authorization rejected")
	case response.StatusCode == http.StatusTooManyRequests:
		return fetchedJSONResponse{}, domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("Bilibili rate limited"))
	case response.StatusCode >= 500:
		return fetchedJSONResponse{}, temporary("Bilibili service unavailable")
	case response.StatusCode != http.StatusOK:
		return fetchedJSONResponse{}, permanent("Bilibili request rejected")
	}
	var wrapped envelope
	if json.Unmarshal(body, &wrapped) != nil || wrapped.Code != 0 || len(wrapped.Data) == 0 {
		return fetchedJSONResponse{}, parse("decode Bilibili response")
	}
	if json.Unmarshal(wrapped.Data, output) != nil {
		return fetchedJSONResponse{}, parse("decode Bilibili data")
	}
	return fetchedJSONResponse{
		payload: body, statusCode: response.StatusCode, requestedURL: target.String(),
		finalURL: response.Request.URL.String(), redirectChain: evidencecapture.RedirectChain(target.String(), response.Request),
		headers: response.Header.Clone(), capturedAt: connector.now().UTC(),
	}, nil
}

func (connector *Connector) accountParties() []domain.SourcePartyAssertion {
	account := domain.SourcePartyAssertion{
		Kind: domain.SourcePartyKindAccount, IdentityNamespace: "bilibili:openid", ExternalID: connector.openID,
		DisplayName: connector.openID,
	}
	account.Role = domain.SourcePartyRoleContentOrigin
	parties := []domain.SourcePartyAssertion{account, {
		Role: domain.SourcePartyRoleDistributor, Kind: domain.SourcePartyKindOrganization,
		IdentityNamespace: "platform", ExternalID: "bilibili", DisplayName: "哔哩哔哩", HomepageURL: "https://www.bilibili.com",
	}}
	account.Role = domain.SourcePartyRoleAuthor
	return append(parties, account)
}

func (connector *Connector) credentials() (credentials, error) {
	name := strings.TrimPrefix(connector.credentialRef, "env:")
	raw, ok := connector.lookupEnv(name)
	if !ok {
		return credentials{}, authentication("Bilibili credential is unavailable")
	}
	var value credentials
	if json.Unmarshal([]byte(raw), &value) != nil || strings.TrimSpace(value.ClientID) == "" || strings.TrimSpace(value.AppSecret) == "" || strings.TrimSpace(value.AccessToken) == "" {
		return credentials{}, authentication("Bilibili credential is invalid")
	}
	return value, nil
}

func addSignature(request *http.Request, credential credentials, now time.Time, nonce string) {
	emptyMD5 := md5.Sum(nil)
	headers := map[string]string{"x-bili-accesskeyid": credential.ClientID, "x-bili-content-md5": hex.EncodeToString(emptyMD5[:]), "x-bili-signature-method": "HMAC-SHA256", "x-bili-signature-nonce": nonce, "x-bili-signature-version": "2.0", "x-bili-timestamp": strconv.FormatInt(now.UTC().Unix(), 10)}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		request.Header.Set(key, headers[key])
		lines = append(lines, key+":"+headers[key])
	}
	mac := hmac.New(sha256.New, []byte(credential.AppSecret))
	_, _ = mac.Write([]byte(strings.Join(lines, "\n")))
	request.Header.Set("Authorization", hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set("access-token", credential.AccessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
}

func mapVideo(item videoItem, observed time.Time) (domain.SourceItem, bool) {
	if item.ResourceID == "" || item.Title == "" || !officialContentURL(item.VideoInfo.ShareURL) {
		return domain.SourceItem{}, false
	}
	published := unixTime(item.PTime)
	if published == nil {
		published = unixTime(item.CTime)
	}
	attachments := []domain.SourceAttachment{}
	if officialAssetURL(item.Cover) {
		attachments = append(attachments, domain.SourceAttachment{URL: item.Cover})
	}
	mapped, err := domain.NormalizeSourceItem(domain.SourceItem{SourceCode: "bilibili", ExternalID: "video:" + item.ResourceID, ContentType: "video", Title: item.Title, Body: item.Description, URL: item.VideoInfo.ShareURL, PublishedAt: published, ObservedAt: observed.UTC(), EvidenceCompleteness: completeness(item.Description), Attachments: attachments})
	return mapped, err == nil
}

func mapArticle(item articleItem, observed time.Time, metricsAllowed bool) (domain.SourceItem, bool) {
	if item.ID <= 0 || item.Title == "" {
		return domain.SourceItem{}, false
	}
	attachments := []domain.SourceAttachment{}
	for _, raw := range append([]string{item.BannerURL}, item.ImageURLs...) {
		if officialAssetURL(raw) {
			attachments = append(attachments, domain.SourceAttachment{URL: raw})
		}
	}
	published := unixTime(item.PublishTime)
	if published == nil {
		published = unixTime(item.CTime)
	}
	metrics := domain.SourceMetrics{}
	if metricsAllowed {
		metrics.ViewCount = domain.KnownMetric(item.Stats.View)
		metrics.LikeCount = domain.KnownMetric(item.Stats.Like)
		metrics.CommentCount = domain.KnownMetric(item.Stats.Reply)
		metrics.ShareCount = domain.KnownMetric(item.Stats.Share)
	}
	mapped, err := domain.NormalizeSourceItem(domain.SourceItem{SourceCode: "bilibili", ExternalID: "article:" + strconv.FormatInt(item.ID, 10), ContentType: "article", Title: item.Title, Body: item.Summary, URL: "https://www.bilibili.com/read/cv" + strconv.FormatInt(item.ID, 10), PublishedAt: published, ObservedAt: observed.UTC(), EvidenceCompleteness: completeness(item.Summary), Attachments: attachments, Metrics: metrics})
	return mapped, err == nil
}

func completeness(body string) domain.EvidenceCompleteness {
	if strings.TrimSpace(body) == "" {
		return domain.EvidenceCompletenessMetadataOnly
	}
	return domain.EvidenceCompletenessSummaryOnly
}
func unixTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	parsed := time.Unix(value, 0).UTC()
	return &parsed
}
func officialURL(value *url.URL) bool {
	return value != nil && value.Scheme == "https" && value.Hostname() == "member.bilibili.com" && (value.Port() == "" || value.Port() == "443") && value.User == nil
}
func officialContentURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && (parsed.Hostname() == "www.bilibili.com" || parsed.Hostname() == "b23.tv") && parsed.User == nil
}
func officialAssetURL(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && (strings.HasSuffix(parsed.Hostname(), ".hdslb.com") || strings.HasSuffix(parsed.Hostname(), ".bilibili.com")) && parsed.User == nil
}

func encodeCursor(value cursor) string {
	value.Version = cursorVersion
	payload, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(payload)
}
func decodeCursor(raw string) (cursor, error) {
	if strings.TrimSpace(raw) == "" {
		return cursor{Version: cursorVersion}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursor{}, err
	}
	var value cursor
	if json.Unmarshal(payload, &value) != nil || value.Version != cursorVersion || value.VideoPage < 0 || value.ArticlePage < 0 {
		return cursor{}, errors.New("invalid cursor")
	}
	return value, nil
}
func randomNonce() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(value)
}
func secureDialContext(resolver lookupIPAddrFunc, dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host != "member.bilibili.com" || port != "443" {
			return nil, errors.New("unsafe Bilibili destination")
		}
		addresses, err := resolver(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("resolve Bilibili destination")
		}
		for _, candidate := range addresses {
			parsed, ok := netip.AddrFromSlice(candidate.IP)
			if !ok || !parsed.Unmap().IsGlobalUnicast() || parsed.Unmap().IsPrivate() {
				return nil, errors.New("unsafe Bilibili address")
			}
		}
		return dial(ctx, network, address)
	}
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
func healthCode(err error) string {
	switch domain.ClassifyCollectionError(err) {
	case domain.CollectionErrorAuthentication:
		return "credential_unavailable"
	case domain.CollectionErrorRateLimited:
		return "upstream_status"
	case domain.CollectionErrorTemporary:
		return "request_failed"
	default:
		return "upstream_status"
	}
}

var _ domain.Connector = (*Connector)(nil)
