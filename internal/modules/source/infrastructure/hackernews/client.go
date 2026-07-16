package hackernews

import (
	"context"
	"crypto/tls"
	"errors"
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
	errUnsafeDestination = errors.New("unsafe Hacker News destination")
	errRedirectLimit     = errors.New("Hacker News redirect limit exceeded")
)

type clientOptions struct {
	resolver    func(context.Context, string) ([]net.IPAddr, error)
	dialContext func(context.Context, string, string) (net.Conn, error)
	tlsConfig   *tls.Config
	now         func() time.Time
}

type client struct {
	endpoint *url.URL
	http     *http.Client
	now      func() time.Time
}

func newClient(endpoint *url.URL, timeout time.Duration, options clientOptions) *client {
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
		ResponseHeaderTimeout: timeout,
		DialContext:           secureDialContext(options.resolver, options.dialContext),
	}
	return &client{
		endpoint: endpoint,
		now:      options.now,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return errRedirectLimit
				}
				if !sameOfficialHost(endpoint, request.URL) {
					return errUnsafeDestination
				}
				return nil
			},
		},
	}
}

func (client *client) get(ctx context.Context, path string) ([]byte, *time.Time, error) {
	target := *client.endpoint
	target.Path = strings.TrimSuffix(client.endpoint.Path, "/") + "/" + strings.TrimPrefix(path, "/")
	target.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, errUnsafeDestination
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, nil, requestError(err)
	}
	retry := retryAfter(response.Header.Get("Retry-After"), client.now())
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		status := response.StatusCode
		closeResponse(response)
		return nil, retry, statusError(status)
	}
	payload, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, nil, domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("read Hacker News response"))
	}
	return payload, retry, nil
}

func sameOfficialHost(endpoint, candidate *url.URL) bool {
	return endpoint != nil && candidate != nil && candidate.Scheme == "https" && strings.EqualFold(candidate.Hostname(), endpoint.Hostname()) && (candidate.Port() == "" || candidate.Port() == "443")
}

func requestError(err error) error {
	if errors.Is(err, errUnsafeDestination) || errors.Is(err, errRedirectLimit) {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("Hacker News destination is not permitted"))
	}
	return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("Hacker News request failed"))
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("Hacker News authentication failed"))
	case http.StatusTooManyRequests:
		return domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("Hacker News rate limited"))
	}
	if status >= http.StatusInternalServerError {
		return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("Hacker News upstream unavailable"))
	}
	return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("Hacker News upstream rejected request"))
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

func secureDialContext(resolver func(context.Context, string) ([]net.IPAddr, error), dialContext func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
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
