package rss

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

const maxRedirects = 3

var (
	errUnsafeDestination = errors.New("unsafe RSS destination")
	errRedirectLimit     = errors.New("RSS redirect limit exceeded")
)

type Connector struct {
	sourceID int64
	endpoint *url.URL
	client   *http.Client
	maxPages int
	now      func() time.Time
}

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

func (lookup lookupIPAddrFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return lookup(ctx, host)
}

type connectorOptions struct {
	resolver    lookupIPAddrFunc
	dialContext func(context.Context, string, string) (net.Conn, error)
	tlsConfig   *tls.Config
	now         func() time.Time
}

// New binds the RSS Connector to one immutable SourceConnection execution
// endpoint. Collection runs later supply only request state, never endpoints
// or credentials.
func New(connection domain.SourceConnection) (*Connector, error) {
	return newConnector(connection, connectorOptions{})
}

func newConnector(connection domain.SourceConnection, options connectorOptions) (*Connector, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid RSS source connection"))
	}
	if normalized.SourceType != domain.SourceTypeRSS {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS connector requires an RSS source connection"))
	}
	endpoint, err := validatedRSSURL(normalized.Endpoint)
	if err != nil {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid RSS endpoint"))
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
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if options.tlsConfig != nil {
		tlsConfig = options.tlsConfig.Clone()
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: time.Duration(normalized.Config.RequestTimeoutSeconds) * time.Second,
		DialContext:           secureDialContext(options.resolver, options.dialContext),
	}
	client := &http.Client{
		Timeout:   time.Duration(normalized.Config.RequestTimeoutSeconds) * time.Second,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errRedirectLimit
			}
			if _, err := validatedRSSURLForEndpoint(endpoint, request.URL.String()); err != nil {
				return errUnsafeDestination
			}
			return nil
		},
	}
	return &Connector{sourceID: normalized.ID, endpoint: endpoint, client: client, maxPages: normalized.Config.MaxPagesPerRun, now: options.now}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeRSS || (connector.sourceID > 0 && normalized.ID != connector.sourceID) || normalized.Endpoint != connector.endpoint.String() {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS source connection does not match connector"))
	}
	return nil
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	if err := request.Validate(); err != nil {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid RSS fetch request"))
	}
	if connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS fetch request source does not match connector"))
	}
	current, err := connector.fetchURL(request.RequestCursor)
	if err != nil {
		return domain.FetchResult{}, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid RSS request cursor"))
	}
	rootFeedRequest := strings.TrimSpace(request.RequestCursor) == ""
	result := domain.FetchResult{
		Items:        []domain.SourceItem{},
		ETag:         request.ETag,
		LastModified: request.LastModified,
		Diagnostics:  []domain.FetchDiagnostic{},
	}
	for page := 0; page < connector.maxPages; page++ {
		etag, lastModified := "", ""
		if rootFeedRequest && page == 0 {
			etag, lastModified = request.ETag, request.LastModified
		}
		response, err := connector.get(ctx, current, etag, lastModified)
		if err != nil {
			return result, connector.requestError(err)
		}
		if rootFeedRequest && page == 0 {
			if etag := response.Header.Get("ETag"); etag != "" {
				result.ETag = etag
			}
			if lastModified := response.Header.Get("Last-Modified"); lastModified != "" {
				result.LastModified = lastModified
			}
		}
		if response.StatusCode == http.StatusNotModified {
			closeResponse(response)
			return result, nil
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			result.RateLimit.RetryAfter = retryAfter(response.Header.Get("Retry-After"), connector.now())
			status := response.StatusCode
			closeResponse(response)
			return result, statusError(status)
		}
		payload, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("read RSS response"))
		}
		feed, err := parseFeed(payload, connector.now())
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("parse RSS response"))
		}
		if len(result.Items)+len(feed.Items) > request.Limit {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS response exceeds collection item limit"))
		}
		result.Items = append(result.Items, feed.Items...)
		for _, diagnostic := range feed.Diagnostics {
			result.Diagnostics = append(result.Diagnostics, domain.FetchDiagnostic{Code: diagnostic.Code, SourceExternalID: diagnostic.SourceExternalID})
		}
		next, err := connector.nextURL(current, response.Header.Get("Link"), feed.NextURL)
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid RSS pagination link"))
		}
		if next == nil {
			return result, nil
		}
		if page+1 == connector.maxPages {
			result.HasMore = true
			result.NextCursor = next.String()
			return result, nil
		}
		current = next
	}
	return result, nil
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.now()
	if err := connector.Validate(ctx, connection); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: "invalid_source_connection"}
	}
	response, err := connector.get(ctx, connector.endpoint, "", "")
	if err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(connector.requestError(err)), DiagnosticCode: "request_failed"}
	}
	status := response.StatusCode
	closeResponse(response)
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
	}
	return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(statusError(status)), DiagnosticCode: "upstream_status"}
}

func (connector *Connector) fetchURL(cursor string) (*url.URL, error) {
	if strings.TrimSpace(cursor) == "" {
		copy := *connector.endpoint
		return &copy, nil
	}
	return validatedRSSURLForEndpoint(connector.endpoint, cursor)
}

func (connector *Connector) nextURL(current *url.URL, linkHeader, atomNext string) (*url.URL, error) {
	raw := nextLink(linkHeader)
	if raw == "" {
		raw = atomNext
	}
	if raw == "" {
		return nil, nil
	}
	next, err := current.Parse(raw)
	if err != nil {
		return nil, err
	}
	return validatedRSSURLForEndpoint(connector.endpoint, next.String())
}

func (connector *Connector) get(ctx context.Context, target *url.URL, etag, lastModified string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, errUnsafeDestination
	}
	request.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9")
	if strings.TrimSpace(etag) != "" {
		request.Header.Set("If-None-Match", strings.TrimSpace(etag))
	}
	if strings.TrimSpace(lastModified) != "" {
		request.Header.Set("If-Modified-Since", strings.TrimSpace(lastModified))
	}
	return connector.client.Do(request)
}

func (connector *Connector) requestError(err error) error {
	if errors.Is(err, errUnsafeDestination) || errors.Is(err, errRedirectLimit) {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS destination is not permitted"))
	}
	return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("RSS request failed"))
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("RSS authentication failed"))
	case http.StatusTooManyRequests:
		return domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("RSS rate limited"))
	}
	if status >= http.StatusInternalServerError {
		return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("RSS upstream unavailable"))
	}
	return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS upstream rejected request"))
}

func retryAfter(value string, now time.Time) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		result := now.Add(time.Duration(seconds) * time.Second).UTC()
		return &result
	}
	if parsed, err := http.ParseTime(value); err == nil {
		result := parsed.UTC()
		return &result
	}
	return nil
}

func closeResponse(response *http.Response) {
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

func validatedRSSURL(value string) (*url.URL, error) {
	normalized, err := domain.NormalizeEndpoint(domain.SourceTypeRSS, value)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid RSS URL")
	}
	return parsed, nil
}

func validatedRSSURLForEndpoint(endpoint *url.URL, value string) (*url.URL, error) {
	parsed, err := validatedRSSURL(value)
	if err != nil {
		return nil, err
	}
	if endpoint == nil || !strings.EqualFold(parsed.Hostname(), endpoint.Hostname()) {
		return nil, fmt.Errorf("RSS URL host does not match source endpoint")
	}
	return parsed, nil
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
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func nextLink(header string) string {
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		if len(parts) < 2 {
			continue
		}
		link := strings.Trim(strings.TrimSpace(parts[0]), "<>")
		for _, parameter := range parts[1:] {
			name, relation, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if !found || !strings.EqualFold(strings.TrimSpace(name), "rel") {
				continue
			}
			for _, item := range strings.Fields(strings.Trim(strings.TrimSpace(relation), `"`)) {
				if strings.EqualFold(item, "next") {
					return link
				}
			}
		}
	}
	return ""
}
