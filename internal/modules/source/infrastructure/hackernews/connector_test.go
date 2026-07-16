package hackernews

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

func TestConnectorFetchesBoundedFirstHNRangeInStableOrder(t *testing.T) {
	t.Parallel()

	story := readFixture(t, "testdata/item-story.json")
	comment := readFixture(t, "testdata/item-comment.json")
	dead := readFixture(t, "testdata/item-dead.json")
	var active, peak atomic.Int32
	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/maxitem.json":
			_, _ = writer.Write([]byte("103"))
		case "/v0/item/101.json":
			_, _ = writer.Write(story)
		case "/v0/item/102.json":
			current := active.Add(1)
			for {
				previous := peak.Load()
				if current <= previous || peak.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
			_, _ = writer.Write(comment)
		case "/v0/item/103.json":
			_, _ = writer.Write(dead)
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(3, ""))
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if result.NextCursor != "103" || result.HasMore {
		t.Fatalf("cursor/result = %#v, want finished high-watermark 103", result)
	}
	if len(result.Items) != 2 || result.Items[0].ExternalID != "101" || result.Items[1].ExternalID != "102" || result.Items[0].ContentType != "article" || result.Items[1].ContentType != "comment" {
		t.Fatalf("items = %#v, want ordered story/comment SourceItems", result.Items)
	}
	if len(result.Items[0].RawPayload) != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "dead_item" {
		t.Fatalf("safe capture result = %#v", result)
	}
	if peak.Load() > maxItemWorkers {
		t.Fatalf("peak item concurrency = %d, want <= %d", peak.Load(), maxItemWorkers)
	}
}

func TestConnectorUsesMonotonicCursorAndDoesNotRefetchSeenIDs(t *testing.T) {
	t.Parallel()

	var itemRequests atomic.Int32
	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/maxitem.json":
			_, _ = writer.Write([]byte("105"))
		case "/v0/item/104.json", "/v0/item/105.json":
			itemRequests.Add(1)
			id, _ := strconv.ParseInt(strings.TrimSuffix(filepath.Base(request.URL.Path), ".json"), 10, 64)
			_, _ = writer.Write([]byte(`{"type":"story","id":` + strconv.FormatInt(id, 10) + `,"title":"Increment","time":1784192400}`))
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(10, "103"))
	if err != nil {
		t.Fatalf("Fetch(increment): %v", err)
	}
	if result.NextCursor != "105" || len(result.Items) != 2 || result.Items[0].ExternalID != "104" || result.Items[1].ExternalID != "105" {
		t.Fatalf("increment result = %#v", result)
	}
	result, err = connector.Fetch(context.Background(), testFetchRequest(10, "105"))
	if err != nil {
		t.Fatalf("Fetch(seen cursor): %v", err)
	}
	if result.NextCursor != "105" || len(result.Items) != 0 || itemRequests.Load() != 2 {
		t.Fatalf("seen result/requests = %#v, %d; want no item refetch", result, itemRequests.Load())
	}
}

func TestConnectorBoundsInitialRangeToFetchLimit(t *testing.T) {
	t.Parallel()

	requested := make(map[string]bool)
	var requestedMu sync.Mutex
	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/maxitem.json":
			_, _ = writer.Write([]byte("105"))
		case "/v0/item/104.json", "/v0/item/105.json":
			requestedMu.Lock()
			requested[request.URL.Path] = true
			requestedMu.Unlock()
			id, _ := strconv.ParseInt(strings.TrimSuffix(filepath.Base(request.URL.Path), ".json"), 10, 64)
			_, _ = writer.Write([]byte(`{"type":"story","id":` + strconv.FormatInt(id, 10) + `,"title":"Bounded","time":1784192400}`))
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(2, ""))
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	requestedMu.Lock()
	defer requestedMu.Unlock()
	if result.NextCursor != "105" || result.HasMore || len(requested) != 2 || !requested["/v0/item/104.json"] || !requested["/v0/item/105.json"] {
		t.Fatalf("bounded first range = result:%#v requests:%#v", result, requested)
	}
}

func TestConnectorIsolatesBadItemsButDoesNotAdvanceCursorOnPageFailure(t *testing.T) {
	t.Parallel()

	t.Run("missing_item", func(t *testing.T) {
		connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/v0/maxitem.json":
				_, _ = writer.Write([]byte("102"))
			case "/v0/item/101.json":
				_, _ = writer.Write([]byte("null"))
			case "/v0/item/102.json":
				_, _ = writer.Write([]byte(`{"type":"comment","id":102,"text":"kept","time":1784192460}`))
			}
		}))
		result, err := connector.Fetch(context.Background(), testFetchRequest(2, "100"))
		if err != nil {
			t.Fatalf("Fetch(): %v", err)
		}
		if result.NextCursor != "102" || len(result.Items) != 1 || result.Items[0].ExternalID != "102" || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "missing_item" {
			t.Fatalf("bad item isolation = %#v", result)
		}
	})

	t.Run("upstream_page_failure", func(t *testing.T) {
		connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/v0/maxitem.json":
				_, _ = writer.Write([]byte("101"))
			case "/v0/item/101.json":
				writer.WriteHeader(http.StatusBadGateway)
			}
		}))
		result, err := connector.Fetch(context.Background(), testFetchRequest(1, "100"))
		if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary || result.NextCursor != "" {
			t.Fatalf("page failure result/error = %#v, %v; want temporary without cursor", result, err)
		}
	})
}

func TestConnectorClassifiesHNTransportAndParseFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		status     int
		body       string
		retryAfter string
		want       domain.CollectionErrorKind
	}{
		{"rate_limited", http.StatusTooManyRequests, "", "60", domain.CollectionErrorRateLimited},
		{"temporary", http.StatusBadGateway, "", "", domain.CollectionErrorTemporary},
		{"invalid_json", http.StatusOK, "{", "", domain.CollectionErrorParse},
	} {
		t.Run(test.name, func(t *testing.T) {
			connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if test.retryAfter != "" {
					writer.Header().Set("Retry-After", test.retryAfter)
				}
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			result, err := connector.Fetch(context.Background(), testFetchRequest(1, ""))
			if err == nil || domain.ClassifyCollectionError(err) != test.want || result.NextCursor != "" {
				t.Fatalf("Fetch() result/error = %#v, %v; want %q without cursor", result, err, test.want)
			}
			if test.want == domain.CollectionErrorRateLimited && result.RateLimit.RetryAfter == nil {
				t.Fatal("rate-limited result did not preserve Retry-After")
			}
		})
	}
}

func TestConnectorHonorsContextTimeoutAndOfficialEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("timeout", func(t *testing.T) {
		connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		result, err := connector.Fetch(ctx, testFetchRequest(1, ""))
		if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary || result.NextCursor != "" {
			t.Fatalf("timeout result/error = %#v, %v; want temporary without cursor", result, err)
		}
	})

	t.Run("official_endpoint", func(t *testing.T) {
		if _, err := New(domain.SourceConnection{SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: "https://example.test/v0", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig()}); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("New(non-official endpoint) error = %v, class = %q; want permanent", err, domain.ClassifyCollectionError(err))
		}
	})
}

func newTestConnector(t *testing.T, handler http.Handler) *Connector {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	config := domain.DefaultSourceConfig()
	connector, err := newConnector(domain.SourceConnection{
		ID: 9, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint,
		AuthType: domain.AuthTypeNone, Config: config, Enabled: true,
	}, clientOptions{
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
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

func testFetchRequest(limit int, cursor string) domain.FetchRequest {
	return domain.FetchRequest{
		CollectionRunID: 10, SourceConnectionID: 9, QuerySignature: strings.Repeat("b", 64), Query: "climate",
		WindowStart: time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC),
		Limit: limit, RequestCursor: cursor,
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return payload
}
