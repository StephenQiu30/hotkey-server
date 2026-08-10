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

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
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
	if result.Items[0].ParentExternalID != "" || result.Items[1].ParentExternalID != "101" {
		t.Fatalf("thread parents = %q / %q, want story root and comment parent 101", result.Items[0].ParentExternalID, result.Items[1].ParentExternalID)
	}
	if result.Items[0].Metrics.LikeCount == nil || *result.Items[0].Metrics.LikeCount != 17 || result.Items[0].Metrics.CommentCount == nil || *result.Items[0].Metrics.CommentCount != 4 {
		t.Fatalf("story metrics = %#v, want official score and descendants", result.Items[0].Metrics)
	}
	if result.Items[1].Metrics.LikeCount != nil || result.Items[1].Metrics.CommentCount != nil {
		t.Fatalf("comment metrics = %#v, want absent official metrics to remain unknown", result.Items[1].Metrics)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "dead_item" {
		t.Fatalf("safe capture result = %#v", result)
	}
	if peak.Load() > maxItemWorkers {
		t.Fatalf("peak item concurrency = %d, want <= %d", peak.Load(), maxItemWorkers)
	}
	if len(result.Snapshots) != 2 || len(result.Items[0].EvidenceReferences) != 1 || len(result.Items[1].EvidenceReferences) != 1 ||
		result.Items[0].EvidenceReferences[0].Usage != domain.EvidenceUsageDocumentSource {
		t.Fatalf("HN item evidence = %#v / %#v", result.Snapshots, result.Items)
	}
	if result.Items[0].DiscussionURL != "https://news.ycombinator.com/item?id=101" || len(result.Items[0].Parties) != 1 || result.Items[0].Parties[0].Role != domain.SourcePartyRoleDistributor {
		t.Fatalf("HN dual URL/parties = %#v", result.Items[0])
	}
}

func TestConnectorMapsAllSupportedHNItemTypesAndPollRelationship(t *testing.T) {
	t.Parallel()

	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/maxitem.json":
			_, _ = writer.Write([]byte("103"))
		case "/v0/item/101.json":
			_, _ = writer.Write([]byte(`{"id":101,"type":"job","title":"Hiring","time":1784192400,"score":0}`))
		case "/v0/item/102.json":
			_, _ = writer.Write([]byte(`{"id":102,"type":"poll","title":"Choose","text":"Question","time":1784192460,"score":3,"descendants":2}`))
		case "/v0/item/103.json":
			_, _ = writer.Write([]byte(`{"id":103,"type":"pollopt","text":"Option A","poll":102,"time":1784192520,"score":0}`))
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(3, "100"))
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if len(result.Items) != 3 || result.Items[0].ContentType != "article" || result.Items[1].ContentType != "article" || result.Items[2].ContentType != "comment" {
		t.Fatalf("mapped item types = %#v, want job/poll articles and poll option comment", result.Items)
	}
	if result.Items[2].ParentExternalID != "102" {
		t.Fatalf("poll option parent = %q, want poll 102", result.Items[2].ParentExternalID)
	}
	if result.Items[0].Metrics.LikeCount == nil || *result.Items[0].Metrics.LikeCount != 0 || result.Items[0].Metrics.CommentCount != nil {
		t.Fatalf("job metrics = %#v, want explicit zero score and unknown descendants", result.Items[0].Metrics)
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

func TestConnectorFetchesTopStoriesInRankingOrderOnEveryPoll(t *testing.T) {
	t.Parallel()

	var poll atomic.Int32
	connector := newTestConnectorWithMode(t, domain.HackerNewsModeTop, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/topstories.json":
			poll.Add(1)
			_, _ = writer.Write([]byte(`[205,203,205,204]`))
		case "/v0/item/205.json", "/v0/item/203.json", "/v0/item/204.json":
			id, _ := strconv.ParseInt(strings.TrimSuffix(filepath.Base(request.URL.Path), ".json"), 10, 64)
			score := id - 190 + int64(poll.Load())
			_, _ = writer.Write([]byte(`{"id":` + strconv.FormatInt(id, 10) + `,"type":"story","title":"Ranked","time":1784192400,"score":` + strconv.FormatInt(score, 10) + `,"descendants":2}`))
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	first, err := connector.Fetch(context.Background(), testFetchRequest(3, "ignored-cursor"))
	if err != nil {
		t.Fatalf("Fetch(first): %v", err)
	}
	second, err := connector.Fetch(context.Background(), testFetchRequest(3, first.NextCursor))
	if err != nil {
		t.Fatalf("Fetch(second): %v", err)
	}
	for name, result := range map[string]domain.FetchResult{"first": first, "second": second} {
		if result.NextCursor != "" || result.HasMore || len(result.Items) != 3 || result.Items[0].ExternalID != "205" || result.Items[1].ExternalID != "203" || result.Items[2].ExternalID != "204" {
			t.Fatalf("%s ranked result = %#v", name, result)
		}
		if len(result.Snapshots) != 4 || len(result.Items[0].EvidenceReferences) != 2 ||
			result.Items[0].EvidenceReferences[1].LocatorValue != "/0" || result.Items[0].EvidenceReferences[1].Usage != domain.EvidenceUsageContext {
			t.Fatalf("%s ranked evidence = %#v / %#v", name, result.Snapshots, result.Items[0].EvidenceReferences)
		}
	}
	if *second.Items[0].Metrics.LikeCount <= *first.Items[0].Metrics.LikeCount {
		t.Fatalf("repeated observation metrics = %d -> %d, want refresh", *first.Items[0].Metrics.LikeCount, *second.Items[0].Metrics.LikeCount)
	}
}

func TestConnectorUsesConfiguredBestStoriesEndpointAndPreservesPartialSuccess(t *testing.T) {
	t.Parallel()

	connector := newTestConnectorWithMode(t, domain.HackerNewsModeBest, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/beststories.json":
			_, _ = writer.Write([]byte(`[302,301]`))
		case "/v0/item/302.json":
			writer.WriteHeader(http.StatusBadGateway)
		case "/v0/item/301.json":
			_, _ = writer.Write([]byte(`{"id":301,"type":"story","title":"Available","time":1784192400,"score":9}`))
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(2, ""))
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ExternalID != "301" || len(result.Diagnostics) != 1 || result.Diagnostics[0].SourceExternalID != "302" || result.Diagnostics[0].Code != "item_temporary_failure" {
		t.Fatalf("partial ranked result = %#v", result)
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

func TestConnectorUsesBoundedHNItemConcurrency(t *testing.T) {
	t.Parallel()

	var active, peak atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v0/maxitem.json" {
			_, _ = writer.Write([]byte("108"))
			return
		}
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		if current == maxItemWorkers {
			releaseOnce.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		id, err := strconv.ParseInt(strings.TrimSuffix(filepath.Base(request.URL.Path), ".json"), 10, 64)
		if err != nil {
			t.Errorf("parse item ID: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte(`{"id":` + strconv.FormatInt(id, 10) + `,"type":"story","title":"Concurrent","time":1784192400}`))
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(8, "100"))
	if err != nil || len(result.Items) != 8 {
		t.Fatalf("Fetch() result/error = %#v / %v, want eight items", result, err)
	}
	if peak.Load() != maxItemWorkers {
		t.Fatalf("peak item concurrency = %d, want bounded parallelism %d", peak.Load(), maxItemWorkers)
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

func TestConnectorPartiallySucceedsAndRetriesOnlyUnfinishedHNWindow(t *testing.T) {
	t.Parallel()

	var recovered atomic.Bool
	var requests sync.Map
	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v0/maxitem.json" {
			_, _ = writer.Write([]byte("103"))
			return
		}
		requests.LoadOrStore(request.URL.Path, new(atomic.Int32))
		counter, _ := requests.Load(request.URL.Path)
		counter.(*atomic.Int32).Add(1)
		switch request.URL.Path {
		case "/v0/item/101.json":
			_, _ = writer.Write([]byte(`{"id":101,"type":"story","title":"First","time":1784192400}`))
		case "/v0/item/102.json":
			if !recovered.Load() {
				writer.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = writer.Write([]byte(`{"id":102,"type":"comment","text":"Recovered","parent":101,"time":1784192460}`))
		case "/v0/item/103.json":
			_, _ = writer.Write([]byte(`{"id":103,"type":"story","title":"Last","time":1784192520}`))
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	partial, err := connector.Fetch(context.Background(), testFetchRequest(3, "100"))
	if err != nil {
		t.Fatalf("Fetch(partial): %v", err)
	}
	if partial.NextCursor != "101" || !partial.HasMore || len(partial.Items) != 1 || partial.Items[0].ExternalID != "101" {
		t.Fatalf("partial result = %#v, want only completed prefix through 101", partial)
	}
	if len(partial.Diagnostics) != 1 || partial.Diagnostics[0].Code != "item_temporary_failure" || partial.Diagnostics[0].SourceExternalID != "102" {
		t.Fatalf("partial diagnostics = %#v, want classified failed item 102", partial.Diagnostics)
	}

	recovered.Store(true)
	completed, err := connector.Fetch(context.Background(), testFetchRequest(3, partial.NextCursor))
	if err != nil {
		t.Fatalf("Fetch(retry): %v", err)
	}
	if completed.NextCursor != "103" || completed.HasMore || len(completed.Items) != 2 || completed.Items[0].ExternalID != "102" || completed.Items[1].ExternalID != "103" {
		t.Fatalf("retry result = %#v, want unfinished window 102-103", completed)
	}
	if count := requestCount(&requests, "/v0/item/101.json"); count != 1 {
		t.Fatalf("item 101 request count = %d, want completed prefix not retried", count)
	}
}

func TestConnectorFailsWhenEveryHNItemRequestFails(t *testing.T) {
	t.Parallel()

	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v0/maxitem.json" {
			_, _ = writer.Write([]byte("102"))
			return
		}
		writer.WriteHeader(http.StatusBadGateway)
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(2, "100"))
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary || result.NextCursor != "" {
		t.Fatalf("all-failed result/error = %#v / %v, want temporary failure without cursor", result, err)
	}
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

	t.Run("cross_host_redirect", func(t *testing.T) {
		connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "https://example.test/v0/maxitem.json", http.StatusFound)
		}))
		result, err := connector.Fetch(context.Background(), testFetchRequest(1, ""))
		if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent || result.NextCursor != "" {
			t.Fatalf("cross-host redirect result/error = %#v / %v, want permanent rejection", result, err)
		}
	})

	t.Run("private_dns", func(t *testing.T) {
		config := domain.DefaultSourceConfig()
		var dialed atomic.Bool
		connector, err := newConnector(domain.SourceConnection{
			ID: 9, SourceType: domain.SourceTypeHackerNews, Name: "HN", Endpoint: domain.HackerNewsEndpoint,
			AuthType: domain.AuthTypeNone, Config: config, Enabled: true,
		}, clientOptions{
			resolver: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			},
			dialContext: func(context.Context, string, string) (net.Conn, error) {
				dialed.Store(true)
				return nil, nil
			},
		})
		if err != nil {
			t.Fatalf("newConnector(): %v", err)
		}
		result, err := connector.Fetch(context.Background(), testFetchRequest(1, ""))
		if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent || result.NextCursor != "" || dialed.Load() {
			t.Fatalf("private DNS result/error/dialed = %#v / %v / %t, want pre-dial permanent rejection", result, err, dialed.Load())
		}
	})
}

func TestFetchItemsTreatsParentCancellationAsPageFailure(t *testing.T) {
	t.Parallel()

	connector := newTestConnector(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("canceled parent must not issue an item request")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, failure := connector.fetchItems(ctx, 101, 101); failure == nil || domain.ClassifyCollectionError(failure.err) != domain.CollectionErrorTemporary {
		t.Fatalf("fetchItems(canceled parent) failure = %#v, want temporary page failure", failure)
	}
}

func TestConnectorPreservesRateLimitWhenConcurrentWorkerCancellationRaces(t *testing.T) {
	t.Parallel()

	startedFirst := make(chan struct{})
	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/maxitem.json":
			_, _ = writer.Write([]byte("102"))
		case "/v0/item/101.json":
			close(startedFirst)
			<-request.Context().Done()
		case "/v0/item/102.json":
			<-startedFirst
			writer.Header().Set("Retry-After", "90")
			writer.WriteHeader(http.StatusTooManyRequests)
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(2, "100"))
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || result.RateLimit.RetryAfter == nil || result.NextCursor != "" {
		t.Fatalf("concurrent 429 result/error = %#v, %v; want rate-limited failure without cursor", result, err)
	}
}

func TestConnectorPreservesPermanentFailureWhenCancellationRaces(t *testing.T) {
	t.Parallel()

	startedFirst := make(chan struct{})
	connector := newTestConnector(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v0/maxitem.json":
			_, _ = writer.Write([]byte("102"))
		case "/v0/item/101.json":
			close(startedFirst)
			<-request.Context().Done()
		case "/v0/item/102.json":
			<-startedFirst
			writer.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected request path %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	result, err := connector.Fetch(context.Background(), testFetchRequest(2, "100"))
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent || result.NextCursor != "" {
		t.Fatalf("concurrent permanent result/error = %#v / %v, want permanent failure without cursor", result, err)
	}
}

func newTestConnector(t *testing.T, handler http.Handler) *Connector {
	return newTestConnectorWithMode(t, domain.HackerNewsModeNew, handler)
}

func newTestConnectorWithMode(t *testing.T, mode domain.HackerNewsMode, handler http.Handler) *Connector {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	config := domain.DefaultSourceConfig()
	config.HackerNewsMode = mode
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

func requestCount(requests *sync.Map, path string) int32 {
	value, ok := requests.Load(path)
	if !ok {
		return 0
	}
	return value.(*atomic.Int32).Load()
}
