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

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const (
	maxRedirects = 3
)

var (
	errUnsafeDestination = errors.New("unsafe RSS destination")
	errRedirectLimit     = errors.New("RSS redirect limit exceeded")
)

type Connector struct {
	sourceID         int64
	endpoint         *url.URL
	client           *http.Client
	maxPages         int
	now              func() time.Time
	retryWait        func(context.Context, int) error
	resourceLimits   ResourceLimitProfile
	requestBudget    domain.ExternalRequestBudget
	collectorProfile domain.CollectorProfileVersion
}

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

func (lookup lookupIPAddrFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return lookup(ctx, host)
}

type connectorOptions struct {
	resolver       lookupIPAddrFunc
	dialContext    func(context.Context, string, string) (net.Conn, error)
	tlsConfig      *tls.Config
	now            func() time.Time
	retryWait      func(context.Context, int) error
	resourceLimits ResourceLimitProfile
	requestBudget  domain.ExternalRequestBudget
}

type redirectTraceContextKey struct{}

// New binds the RSS Connector to one immutable SourceConnection execution
// endpoint. Collection runs later supply only request state, never endpoints
// or credentials.
func New(connection domain.SourceConnection, requestBudget domain.ExternalRequestBudget, resolvers ...sourcenet.Resolver) (*Connector, error) {
	options := connectorOptions{requestBudget: requestBudget}
	if len(resolvers) > 0 && resolvers[0] != nil {
		options.resolver = resolvers[0].LookupIPAddr
	}
	return newConnector(connection, options)
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
	if options.resourceLimits.Version == "" {
		options.resourceLimits = DefaultResourceLimitProfile()
	}
	if err := options.resourceLimits.Validate(); err != nil || options.requestBudget == nil {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid RSS resource limit profile"))
	}
	if options.retryWait == nil {
		options.retryWait = retryBackoff
	}
	collectorProfile, err := domain.NewCollectorProfileVersion(CollectorProfileVersion)
	if err != nil {
		return nil, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("invalid RSS collector profile version"))
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
		DisableKeepAlives:     true,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   options.resourceLimits.ConnectTimeout,
		ResponseHeaderTimeout: minimumDuration(time.Duration(normalized.Config.RequestTimeoutSeconds)*time.Second, options.resourceLimits.ReadTimeout),
		DialContext: secureDialContext(options.resolver, options.dialContext,
			options.resourceLimits.ConnectTimeout, options.resourceLimits.ReadTimeout),
	}
	reserveRedirect := func(ctx context.Context) error {
		decision, err := options.requestBudget.ReserveExternalRequest(ctx, domain.ExternalRequestBudgetReservation{
			SourceConnectionID: normalized.ID, ResourceProfileVersion: options.resourceLimits.Version,
			DailyLimit: options.resourceLimits.DailyRequestQuota, At: options.now().UTC(),
		})
		if err != nil {
			return err
		}
		if err := decision.Validate(options.resourceLimits.DailyRequestQuota); err != nil {
			return err
		}
		if !decision.Allowed {
			return requestQuotaError{resetAt: decision.ResetAt}
		}
		return nil
	}
	client := &http.Client{
		Timeout:   options.resourceLimits.WallClockTimeout,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errRedirectLimit
			}
			if _, err := validatedRSSURLForEndpoint(endpoint, request.URL.String()); err != nil {
				return errUnsafeDestination
			}
			if err := reserveRedirect(request.Context()); err != nil {
				return err
			}
			if trace, ok := request.Context().Value(redirectTraceContextKey{}).(*[]string); ok && trace != nil {
				*trace = append(*trace, request.URL.String())
			}
			return nil
		},
	}
	maxPages := normalized.Config.MaxPagesPerRun
	if maxPages > options.resourceLimits.MaxPages {
		maxPages = options.resourceLimits.MaxPages
	}
	return &Connector{
		sourceID: normalized.ID, endpoint: endpoint, client: client,
		maxPages: maxPages, now: options.now, retryWait: options.retryWait,
		resourceLimits: options.resourceLimits, requestBudget: options.requestBudget, collectorProfile: collectorProfile,
	}, nil
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
	ctx, cancel := context.WithTimeout(ctx, connector.resourceLimits.WallClockTimeout)
	defer cancel()
	result := domain.FetchResult{
		Items:        []domain.SourceItem{},
		Snapshots:    []domain.EvidenceSnapshot{},
		ETag:         request.ETag,
		LastModified: request.LastModified,
		Diagnostics:  []domain.FetchDiagnostic{},
	}
	maximumItems := request.Limit
	if maximumItems > connector.resourceLimits.MaxItems {
		maximumItems = connector.resourceLimits.MaxItems
	}
	var cumulativeBytes int64
	for page := 0; page < connector.maxPages; page++ {
		etag, lastModified := "", ""
		if rootFeedRequest && page == 0 {
			etag, lastModified = request.ETag, request.LastModified
		}
		response, redirectChain, err := connector.getWithRetry(ctx, current, etag, lastModified)
		if err != nil {
			var quota requestQuotaError
			if errors.As(err, &quota) {
				resetAt := quota.resetAt.UTC()
				result.RateLimit.RetryAfter = &resetAt
			}
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
		remainingBytes := connector.resourceLimits.MaxCumulativeResponseBytes - cumulativeBytes
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, remainingBytes+1))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("read RSS response"))
		}
		if int64(len(payload)) > remainingBytes {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS response exceeds body byte limit"))
		}
		cumulativeBytes += int64(len(payload))
		capturedAt := connector.now().UTC()
		feed, err := parseFeed(payload, capturedAt)
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("parse RSS response"))
		}
		mimeType := response.Header.Get("Content-Type")
		if strings.TrimSpace(mimeType) == "" {
			mimeType = http.DetectContentType(payload)
		}
		finalURL := current.String()
		if response.Request != nil && response.Request.URL != nil {
			finalURL = response.Request.URL.String()
		}
		responseHeaders, err := domain.NewRawResponseHeaders(map[string][]string(response.Header))
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("validate RSS response headers"))
		}
		snapshot, err := domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
			Payload: payload, MIMEType: mimeType, StatusCode: response.StatusCode,
			RequestedURL: current.String(), FinalURL: finalURL, RedirectChain: redirectChain,
			ResponseHeaders: responseHeaders, CapturedAt: capturedAt, CollectorProfileVersion: connector.collectorProfile,
		})
		if err != nil {
			return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("validate RSS response snapshot"))
		}
		for index := range feed.Items {
			feed.Items[index].SnapshotKey = snapshot.Key
			for referenceIndex := range feed.Items[index].EvidenceReferences {
				feed.Items[index].EvidenceReferences[referenceIndex].SnapshotKey = snapshot.Key
			}
			normalized, err := domain.NormalizeSourceItem(feed.Items[index])
			if err != nil {
				return result, domain.NewCollectionError(domain.CollectionErrorParse, errors.New("bind RSS item snapshot"))
			}
			feed.Items[index] = normalized
		}
		if len(result.Items)+len(feed.Items) > maximumItems {
			return result, domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("RSS response exceeds collection item limit"))
		}
		result.Snapshots = append(result.Snapshots, snapshot)
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
		if page+1 == connector.maxPages || len(result.Items) == maximumItems || cumulativeBytes == connector.resourceLimits.MaxCumulativeResponseBytes {
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
	ctx, cancel := context.WithTimeout(ctx, connector.resourceLimits.WallClockTimeout)
	defer cancel()
	response, _, err := connector.getWithRetry(ctx, connector.endpoint, "", "")
	if err != nil {
		diagnosticCode := "request_failed"
		if errors.Is(err, errUnsafeDestination) || errors.Is(err, errRedirectLimit) {
			diagnosticCode = "destination_not_permitted"
		}
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(connector.requestError(err)), DiagnosticCode: diagnosticCode}
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

func (connector *Connector) get(ctx context.Context, target *url.URL, etag, lastModified string) (*http.Response, []string, error) {
	decision, err := connector.requestBudget.ReserveExternalRequest(ctx, domain.ExternalRequestBudgetReservation{
		SourceConnectionID: connector.sourceID, ResourceProfileVersion: connector.resourceLimits.Version,
		DailyLimit: connector.resourceLimits.DailyRequestQuota, At: connector.now().UTC(),
	})
	if err != nil {
		return nil, nil, err
	}
	if err := decision.Validate(connector.resourceLimits.DailyRequestQuota); err != nil {
		return nil, nil, err
	}
	if !decision.Allowed {
		return nil, nil, requestQuotaError{resetAt: decision.ResetAt}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, errUnsafeDestination
	}
	redirectChain := []string{}
	request = request.WithContext(context.WithValue(request.Context(), redirectTraceContextKey{}, &redirectChain))
	request.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9")
	if strings.TrimSpace(etag) != "" {
		request.Header.Set("If-None-Match", strings.TrimSpace(etag))
	}
	if strings.TrimSpace(lastModified) != "" {
		request.Header.Set("If-Modified-Since", strings.TrimSpace(lastModified))
	}
	response, err := connector.client.Do(request)
	return response, redirectChain, err
}

func (connector *Connector) getWithRetry(ctx context.Context, target *url.URL, etag, lastModified string) (*http.Response, []string, error) {
	for attempt := 0; ; attempt++ {
		response, redirects, err := connector.get(ctx, target, etag, lastModified)
		retryable := false
		if err != nil {
			retryable = domain.ClassifyCollectionError(connector.requestError(err)) == domain.CollectionErrorTemporary
		} else if response.StatusCode >= http.StatusInternalServerError {
			retryable = true
		}
		if !retryable || attempt >= connector.resourceLimits.MaxRetries {
			return response, redirects, err
		}
		if response != nil {
			closeResponse(response)
		}
		if err := connector.retryWait(ctx, attempt+1); err != nil {
			return nil, nil, err
		}
	}
}

func (connector *Connector) requestError(err error) error {
	var quota requestQuotaError
	if errors.As(err, &quota) {
		return domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("RSS daily request quota exceeded"))
	}
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
	_, _ = io.CopyN(io.Discard, response.Body, 32<<10)
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

func secureDialContext(resolver lookupIPAddrFunc, dialContext func(context.Context, string, string) (net.Conn, error), connectTimeout, readTimeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, connectTimeout)
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
				return &deadlineConnection{Conn: connection, timeout: readTimeout}, nil
			}
			dialErr = err
		}
		if dialErr != nil {
			return nil, dialErr
		}
		return nil, errUnsafeDestination
	}
}

type deadlineConnection struct {
	net.Conn
	timeout time.Duration
}

func (connection *deadlineConnection) Read(buffer []byte) (int, error) {
	if err := connection.SetReadDeadline(time.Now().Add(connection.timeout)); err != nil {
		return 0, err
	}
	return connection.Conn.Read(buffer)
}

func (connection *deadlineConnection) Write(buffer []byte) (int, error) {
	if err := connection.SetWriteDeadline(time.Now().Add(connection.timeout)); err != nil {
		return 0, err
	}
	return connection.Conn.Write(buffer)
}

type requestQuotaError struct{ resetAt time.Time }

func (requestQuotaError) Error() string { return "RSS request quota exhausted" }

func retryBackoff(ctx context.Context, attempt int) error {
	delay := 250 * time.Millisecond * time.Duration(1<<uint(attempt-1))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
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
