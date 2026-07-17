package rss

import (
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

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

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
			if len(result.Items) == 0 || result.Items[0].ExternalID != test.externalID || len(result.Items[0].RawPayload) != 0 {
				t.Fatalf("items = %#v, want normalized item without raw response", result.Items)
			}
			if result.ETag != `"next-etag"` || result.LastModified != "Wed, 16 Jul 2026 08:00:00 GMT" {
				t.Fatalf("conditional metadata = %#v", result)
			}
		})
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
				if err != nil || result.ETag != `"not-modified"` || len(result.Items) != 0 {
					t.Fatalf("304 result, error = %#v, %v", result, err)
				}
				return
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
		if _, err := connector.Fetch(context.Background(), testFetchRequest()); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorParse {
			t.Fatalf("invalid XML error = %v, class = %q; want parse", err, domain.ClassifyCollectionError(err))
		}
	})
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
}

func newTestConnector(t *testing.T, server *httptest.Server, maxPages int, resolver lookupIPAddrFunc) *Connector {
	t.Helper()
	config := domain.DefaultSourceConfig()
	config.MaxPagesPerRun = maxPages
	connector, err := newConnector(domain.SourceConnection{
		ID: 7, SourceType: domain.SourceTypeRSS, Name: "Fixture RSS", Endpoint: "https://feeds.example.test/rss",
		AuthType: domain.AuthTypeNone, Config: config, Enabled: true,
	}, connectorOptions{
		resolver: resolver,
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
