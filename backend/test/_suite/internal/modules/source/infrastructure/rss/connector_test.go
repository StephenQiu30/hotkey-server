package rss

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestConnectorRejectsCompressionBombBeforeEvidence(t *testing.T) {
	var encoded bytes.Buffer
	compressor := gzip.NewWriter(&encoded)
	if _, err := compressor.Write(bytes.Repeat([]byte("A"), 1<<20)); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept-Encoding") != "gzip" {
			t.Errorf("Accept-Encoding = %q, want explicit guarded gzip", request.Header.Get("Accept-Encoding"))
		}
		writer.Header().Set("Content-Encoding", "gzip")
		writer.Header().Set("Content-Type", "application/rss+xml")
		_, _ = writer.Write(encoded.Bytes())
	}))
	defer server.Close()

	connector := newTestConnector(t, server, 1, publicResolver())
	result, err := connector.Fetch(context.Background(), testFetchRequest())
	if domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent ||
		domain.SafeCollectionErrorCause(err) != "RSS compressed response is not permitted" {
		t.Fatalf("compression bomb error = %v / %q", err, domain.SafeCollectionErrorCause(err))
	}
	if len(result.Items) != 0 || len(result.Snapshots) != 0 {
		t.Fatalf("compression bomb produced evidence: %#v", result)
	}
}

func TestConnectorFetchesRSSAndAtomWithConditionalRequests(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		fixture    string
		externalID string
	}{
		{"rss", "testdata/rss/news.xml", "rss-100"},
		{"atom", "testdata/atom/news.xml", "atom-200"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := readFixture(t, test.fixture)
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("If-None-Match") != `"prior-etag"` || request.Header.Get("If-Modified-Since") != "Wed, 16 Jul 2026 07:00:00 GMT" {
					t.Errorf("conditional headers = %#v", request.Header)
				}
				writer.Header().Set("ETag", `"next-etag"`)
				writer.Header().Set("Last-Modified", "Wed, 16 Jul 2026 08:00:00 GMT")
				_, _ = writer.Write(payload)
			}))
			defer server.Close()

			connector := newTestConnector(t, server, 1, publicResolver())
			result, err := connector.Fetch(context.Background(), testFetchRequest())
			if err != nil {
				t.Fatalf("Fetch(): %v", err)
			}
			if len(result.Items) == 0 || result.Items[0].ExternalID != test.externalID {
				t.Fatalf("items = %#v, want normalized item", result.Items)
			}
			if result.ETag != `"next-etag"` || result.LastModified != "Wed, 16 Jul 2026 08:00:00 GMT" {
				t.Fatalf("conditional metadata = %#v", result)
			}
			if len(result.Snapshots) != 1 || string(result.Snapshots[0].Payload) != string(payload) || !result.Snapshots[0].VerifyPayload() {
				t.Fatalf("snapshots = %#v, want one verifiable raw response", result.Snapshots)
			}
			if result.Items[0].SnapshotKey != result.Snapshots[0].Key || result.Items[0].ItemLocator == "" {
				t.Fatalf("item evidence reference = %#v, snapshot = %#v", result.Items[0], result.Snapshots[0])
			}
		})
	}
}

func TestConnectorSharesOnePageSnapshotAcrossItemsWithStableUniqueLocators(t *testing.T) {
	t.Parallel()

	payload := []byte(`<?xml version="1.0"?><rss><channel>
		<item><guid>first</guid><title>First</title></item>
		<item><guid>second</guid><title>Second</title></item>
	</channel></rss>`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = writer.Write(payload)
	}))
	defer server.Close()

	connector := newTestConnector(t, server, 1, publicResolver())
	result, err := connector.Fetch(context.Background(), testFetchRequest())
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if len(result.Snapshots) != 1 || len(result.Items) != 2 {
		t.Fatalf("result = %#v, want one response snapshot and two items", result)
	}
	if result.Items[0].SnapshotKey != result.Snapshots[0].Key || result.Items[1].SnapshotKey != result.Snapshots[0].Key {
		t.Fatalf("snapshot references = %#v, want shared key %q", result.Items, result.Snapshots[0].Key)
	}
	if result.Items[0].ItemLocator == "" || result.Items[0].ItemLocator == result.Items[1].ItemLocator {
		t.Fatalf("item locators = %q, %q; want stable unique locators", result.Items[0].ItemLocator, result.Items[1].ItemLocator)
	}
	if string(result.Snapshots[0].Payload) != string(payload) {
		t.Fatal("raw response was not preserved byte-for-byte in its snapshot")
	}
}

func TestConnectorReturnsNotModifiedAndClassifiesResponses(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		status     int
		retryAfter string
		wantKind   domain.CollectionErrorKind
	}{
		{"not_modified", http.StatusNotModified, "", ""},
		{"rate_limited", http.StatusTooManyRequests, "120", domain.CollectionErrorRateLimited},
		{"authentication", http.StatusUnauthorized, "", domain.CollectionErrorAuthentication},
		{"temporary", http.StatusBadGateway, "", domain.CollectionErrorTemporary},
		{"permanent", http.StatusBadRequest, "", domain.CollectionErrorPermanent},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if test.retryAfter != "" {
					writer.Header().Set("Retry-After", test.retryAfter)
				}
				writer.Header().Set("ETag", `"not-modified"`)
				writer.WriteHeader(test.status)
			}))
			defer server.Close()

			connector := newTestConnector(t, server, 1, publicResolver())
			result, err := connector.Fetch(context.Background(), testFetchRequest())
			if test.wantKind == "" {
				if err != nil || result.ETag != `"not-modified"` || len(result.Items) != 0 || len(result.Snapshots) != 0 {
					t.Fatalf("304 result, error = %#v, %v", result, err)
				}
				return
			}
			if len(result.Snapshots) != 0 {
				t.Fatalf("failed response fabricated snapshots: %#v", result.Snapshots)
			}
			if err == nil || domain.ClassifyCollectionError(err) != test.wantKind {
				t.Fatalf("Fetch() error = %v, class = %q; want %q", err, domain.ClassifyCollectionError(err), test.wantKind)
			}
			if test.wantKind == domain.CollectionErrorRateLimited {
				want := connector.now().Add(120 * time.Second)
				if result.RateLimit.RetryAfter == nil || !result.RateLimit.RetryAfter.Equal(want) {
					t.Fatalf("retry-after = %v, want %v", result.RateLimit.RetryAfter, want)
				}
			}
		})
	}
}

func TestConnectorTreatsEmptyRSSAndAtomFeedsAsSuccessfulZeroItems(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{
		`<?xml version="1.0"?><rss><channel><title>Empty</title></channel></rss>`,
		`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Empty</title></feed>`,
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(payload))
		}))
		connector := newTestConnector(t, server, 1, publicResolver())
		result, err := connector.Fetch(context.Background(), testFetchRequest())
		server.Close()
		if err != nil || len(result.Items) != 0 || len(result.Snapshots) != 1 {
			t.Fatalf("empty feed result/error = %#v/%v", result, err)
		}
	}
}

func TestConnectorSnapshotRecordsRequestedFinalAndRedirectChain(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rss":
			http.Redirect(writer, request, "https://feeds.example.test/intermediate", http.StatusFound)
		case "/intermediate":
			http.Redirect(writer, request, "https://feeds.example.test/final", http.StatusTemporaryRedirect)
		case "/final":
			writer.Header().Set("Content-Type", "application/atom+xml")
			_, _ = writer.Write([]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><id>redirected</id><title>Redirected</title></entry></feed>`))
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	connector := newTestConnector(t, server, 1, publicResolver())
	result, err := connector.Fetch(context.Background(), testFetchRequest())
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if len(result.Snapshots) != 1 {
		t.Fatalf("snapshots = %#v, want one", result.Snapshots)
	}
	snapshot := result.Snapshots[0]
	wantRedirects := []string{"https://feeds.example.test/intermediate", "https://feeds.example.test/final"}
	if snapshot.RequestedURL != "https://feeds.example.test/rss" || snapshot.FinalURL != wantRedirects[1] || len(snapshot.RedirectChain) != 2 || snapshot.RedirectChain[0] != wantRedirects[0] || snapshot.RedirectChain[1] != wantRedirects[1] {
		t.Fatalf("redirect provenance = %#v, want requested/final/chain", snapshot)
	}
}

func TestConnectorClassifiesTimeoutAndInvalidXML(t *testing.T) {
	t.Parallel()

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		defer server.Close()
		connector := newTestConnector(t, server, 1, publicResolver())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		if _, err := connector.Fetch(ctx, testFetchRequest()); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary {
			t.Fatalf("timeout error = %v, class = %q; want temporary", err, domain.ClassifyCollectionError(err))
		}
	})

	t.Run("invalid_xml", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte("<rss><channel><item>"))
		}))
		defer server.Close()
		connector := newTestConnector(t, server, 1, publicResolver())
		result, err := connector.Fetch(context.Background(), testFetchRequest())
		if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorParse || len(result.Snapshots) != 0 {
			t.Fatalf("invalid XML error = %v, class = %q; want parse", err, domain.ClassifyCollectionError(err))
		}
	})
}

func TestConnectorRejectsResponseAboveHardByteLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/rss+xml")
		_, _ = writer.Write([]byte(strings.Repeat("x", maxResponseBodyBytes+1)))
	}))
	defer server.Close()

	connector := newTestConnector(t, server, 1, publicResolver())
	if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("oversized response error = %v, class = %q; want permanent", err, domain.ClassifyCollectionError(err))
	}
}

func TestConnectorBoundsPaginationAndRejectsUnsafeDestinations(t *testing.T) {
	t.Parallel()

	t.Run("follows_safe_next_page_without_reusing_validators", func(t *testing.T) {
		var paths []string
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			paths = append(paths, request.URL.Path)
			switch request.URL.Path {
			case "/rss":
				if request.Header.Get("If-None-Match") == "" || request.Header.Get("If-Modified-Since") == "" {
					t.Errorf("initial request omitted conditional validators")
				}
				writer.Header().Set("Link", `<https://feeds.example.test/page-2>; rel="next"`)
			case "/page-2":
				if request.Header.Get("If-None-Match") != "" || request.Header.Get("If-Modified-Since") != "" {
					t.Errorf("next page reused initial conditional validators")
				}
			default:
				t.Fatalf("unexpected path %q", request.URL.Path)
			}
			_, _ = writer.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>` + request.URL.Path + `</guid><title>Page</title></item></channel></rss>`))
		}))
		defer server.Close()

		connector := newTestConnector(t, server, 2, publicResolver())
		result, err := connector.Fetch(context.Background(), testFetchRequest())
		if err != nil {
			t.Fatalf("Fetch(): %v", err)
		}
		if len(paths) != 2 || len(result.Items) != 2 || result.HasMore || result.NextCursor != "" {
			t.Fatalf("paths/result = %#v, %#v; want two completed pages", paths, result)
		}
		if len(result.Snapshots) != 2 || result.Items[0].SnapshotKey == result.Items[1].SnapshotKey {
			t.Fatalf("page evidence = %#v, %#v; want one distinct snapshot per page", result.Snapshots, result.Items)
		}
	})

	t.Run("continuation_cursor_keeps_root_validators_without_sending_them", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/page-2" {
				t.Errorf("unexpected continuation path %q", request.URL.Path)
			}
			if request.Header.Get("If-None-Match") != "" || request.Header.Get("If-Modified-Since") != "" {
				t.Errorf("continuation request reused root conditional validators")
			}
			writer.Header().Set("ETag", `"continuation-etag"`)
			writer.Header().Set("Last-Modified", "Wed, 16 Jul 2026 08:30:00 GMT")
			_, _ = writer.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>page-2</guid><title>Page</title></item></channel></rss>`))
		}))
		defer server.Close()

		connector := newTestConnector(t, server, 2, publicResolver())
		request := testFetchRequest()
		request.RequestCursor = "https://feeds.example.test/page-2"
		result, err := connector.Fetch(context.Background(), request)
		if err != nil {
			t.Fatalf("Fetch(): %v", err)
		}
		if result.ETag != request.ETag || result.LastModified != request.LastModified {
			t.Fatalf("continuation validators = %#v, want preserved root validators", result)
		}
	})

	t.Run("pagination_limit", func(t *testing.T) {
		var requests int
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests++
			if request.URL.Path == "/rss" {
				writer.Header().Set("Link", `<https://feeds.example.test/page-2>; rel="next"`)
			}
			_, _ = writer.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>` + request.URL.Path + `</guid><title>Page</title></item></channel></rss>`))
		}))
		defer server.Close()

		connector := newTestConnector(t, server, 1, publicResolver())
		result, err := connector.Fetch(context.Background(), testFetchRequest())
		if err != nil {
			t.Fatalf("Fetch(): %v", err)
		}
		if requests != 1 || !result.HasMore || result.NextCursor != "https://feeds.example.test/page-2" {
			t.Fatalf("requests/result = %d, %#v; want one page and safe next cursor", requests, result)
		}
	})

	t.Run("cross_host_redirect_cursor_and_link", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/redirect":
				http.Redirect(writer, request, "https://other.example.test/feed", http.StatusFound)
			case "/link":
				writer.Header().Set("Link", `<https://other.example.test/page-2>; rel="next"`)
				_, _ = writer.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>link</guid><title>Link</title></item></channel></rss>`))
			default:
				_, _ = writer.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>cursor</guid><title>Cursor</title></item></channel></rss>`))
			}
		}))
		defer server.Close()
		connector := newTestConnector(t, server, 1, publicResolver())
		for _, test := range []struct {
			name    string
			request domain.FetchRequest
		}{
			{"redirect", requestWithCursor("https://feeds.example.test/redirect")},
			{"cursor", requestWithCursor("https://other.example.test/page-2")},
			{"link", requestWithCursor("https://feeds.example.test/link")},
		} {
			t.Run(test.name, func(t *testing.T) {
				if _, err := connector.Fetch(context.Background(), test.request); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
					t.Fatalf("cross-host %s error = %v, class = %q; want permanent", test.name, err, domain.ClassifyCollectionError(err))
				}
			})
		}
	})

	t.Run("unsafe_redirect", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "https://127.0.0.1/private", http.StatusFound)
		}))
		defer server.Close()
		connector := newTestConnector(t, server, 1, publicResolver())
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("unsafe redirect error = %v, class = %q; want permanent", err, domain.ClassifyCollectionError(err))
		}
	})

	t.Run("credential_shaped_redirect", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "https://feeds.example.test/next?token=secret", http.StatusFound)
		}))
		defer server.Close()
		connector := newTestConnector(t, server, 1, publicResolver())
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("credential-shaped redirect error = %v, class = %q; want permanent", err, domain.ClassifyCollectionError(err))
		}
	})

	t.Run("private_dns", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer server.Close()
		connector := newTestConnector(t, server, 1, func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		})
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("private DNS error = %v, class = %q; want permanent", err, domain.ClassifyCollectionError(err))
		}
	})

	t.Run("dns_rebinding_before_new_connection", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Connection", "close")
			_, _ = writer.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>safe</guid><title>Safe</title></item></channel></rss>`))
		}))
		defer server.Close()

		lookups := 0
		connector := newTestConnector(t, server, 1, func(context.Context, string) ([]net.IPAddr, error) {
			lookups++
			if lookups == 1 {
				return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		})

		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err != nil {
			t.Fatalf("first Fetch(): %v", err)
		}
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("rebound DNS error = %v, class = %q; want permanent", err, domain.ClassifyCollectionError(err))
		}
		if lookups != 2 {
			t.Fatalf("DNS lookups = %d, want one validation per connection", lookups)
		}
	})
}

func TestConnectorHealthReportsBlockedDestination(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	connector := newTestConnector(t, server, 1, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.23")}}, nil
	})

	result := connector.Health(context.Background(), domain.SourceConnection{
		ID: 7, SourceType: domain.SourceTypeRSS, Name: "Fixture RSS", Endpoint: "https://feeds.example.test/rss",
		AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true,
	})
	if result.Healthy || result.DiagnosticCode != "destination_not_permitted" || result.ErrorKind != domain.CollectionErrorPermanent {
		t.Fatalf("Health() = %#v, want permanent destination_not_permitted", result)
	}
}

func newTestConnector(t *testing.T, server *httptest.Server, maxPages int, resolver lookupIPAddrFunc) *Connector {
	t.Helper()
	profile := DefaultResourceLimitProfile()
	return newTestConnectorWithLimits(t, server, maxPages, resolver, profile, allowingRequestBudget{}, nil)
}

func newTestConnectorWithLimits(t *testing.T, server *httptest.Server, maxPages int, resolver lookupIPAddrFunc, profile ResourceLimitProfile, budget domain.ExternalRequestBudget, wait func(context.Context, int) error) *Connector {
	t.Helper()
	config := domain.DefaultSourceConfig()
	config.MaxPagesPerRun = maxPages
	connector, err := newConnector(domain.SourceConnection{
		ID: 7, SourceType: domain.SourceTypeRSS, Name: "Fixture RSS", Endpoint: "https://feeds.example.test/rss",
		AuthType: domain.AuthTypeNone, Config: config, Enabled: true,
	}, connectorOptions{
		resolver:       resolver,
		requestBudget:  budget,
		resourceLimits: profile,
		retryWait:      wait,
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		tlsConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local httptest transport only
		now:       func() time.Time { return time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	return connector
}

type allowingRequestBudget struct{}

func (allowingRequestBudget) ReserveExternalRequest(_ context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	at := reservation.At.UTC()
	resetAt := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return domain.ExternalRequestBudgetDecision{Allowed: true, Used: 1, ResetAt: resetAt}, nil
}

func publicResolver() lookupIPAddrFunc {
	return func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
}

func testFetchRequest() domain.FetchRequest {
	return domain.FetchRequest{
		CollectionRunID: 8, SourceConnectionID: 7, QuerySignature: strings.Repeat("a", 64), Query: "climate",
		WindowStart: time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC),
		ETag: `"prior-etag"`, LastModified: "Wed, 16 Jul 2026 07:00:00 GMT", Limit: 100,
	}
}

func requestWithCursor(cursor string) domain.FetchRequest {
	request := testFetchRequest()
	request.RequestCursor = cursor
	return request
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return payload
}
