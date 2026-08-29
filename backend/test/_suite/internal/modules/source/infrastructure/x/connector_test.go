package x

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
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
	server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept-Encoding") != "gzip" {
			t.Errorf("Accept-Encoding = %q, want explicit guarded gzip", request.Header.Get("Accept-Encoding"))
		}
		writer.Header().Set("Content-Encoding", "gzip")
		_, _ = writer.Write(encoded.Bytes())
	}))
	defer server.Close()
	connector := newFixtureConnector(t, server, time.Now().UTC(), tokenLookup)
	result, err := connector.Fetch(context.Background(), xFetchRequest())
	if domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent ||
		domain.SafeCollectionErrorCause(err) != "X compressed response is not permitted" {
		t.Fatalf("compression bomb error = %v / %q", err, domain.SafeCollectionErrorCause(err))
	}
	if len(result.Items) != 0 || len(result.Snapshots) != 0 {
		t.Fatalf("compression bomb produced evidence: %#v", result)
	}
}

func TestConnectorMapsOfficialFieldsAndAdvancesHighWaterOnlyAfterPagination(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 7, 3, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer fixture-secret" {
			t.Errorf("Authorization = %q", got)
		}
		if request.URL.Path != "/2/tweets/search/recent" || request.Method != http.MethodGet {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if got, want := request.URL.Query().Get("query"), `("AI news" -rumor) (lang:en OR lang:zh-CN) (place_country:CN OR place_country:US)`; got != want {
			t.Errorf("query = %q, want %q", got, want)
		}
		fields := request.URL.Query().Get("tweet.fields")
		if request.URL.Query().Get("max_results") != "100" || fields == "" ||
			!strings.Contains(fields, "note_tweet") || !strings.Contains(fields, "referenced_tweets") ||
			request.URL.Query().Get("post.fields") != "" || request.URL.Query().Get("expansions") == "" {
			t.Errorf("official fields query = %v", request.URL.Query())
		}
		switch calls.Add(1) {
		case 1:
			if request.URL.Query().Get("sort_order") != "relevancy" {
				t.Errorf("initial discovery sort = %q", request.URL.Query().Get("sort_order"))
			}
			if request.URL.Query().Get("since_id") != "" || request.URL.Query().Get("next_token") != "" {
				t.Errorf("initial checkpoint query = %v", request.URL.Query())
			}
			_, _ = writer.Write([]byte(`{
				"data":[
					{"id":"202","text":"restricted","author_id":"u1","possibly_sensitive":true},
					{"id":"201","text":"withheld","author_id":"u1","withheld":{"country_codes":["CN"]}},
					{"id":"200","text":"Public launch…","note_tweet":{"text":"Public launch long-form body"},"author_id":"u1","created_at":"2026-08-07T02:55:00Z","conversation_id":"190","lang":"en","referenced_tweets":[{"type":"quoted","id":"199"}],"attachments":{"media_keys":["m1"]},"public_metrics":{"impression_count":50,"like_count":7,"reply_count":3,"repost_count":2,"quote_count":1}}
				],
				"errors":[{"resource_type":"post","resource_id":"198","title":"Not Found","detail":"must not persist"}],
				"includes":{"users":[{"id":"u1","username":"official"}],"media":[{"media_key":"m1","type":"photo","url":"https://pbs.twimg.com/media/fixture.jpg"}]},
				"meta":{"newest_id":"202","oldest_id":"200","next_token":"page-2","result_count":3}
			}`))
		case 2:
			if request.URL.Query().Get("sort_order") != "relevancy" {
				t.Errorf("backfill pagination sort = %q", request.URL.Query().Get("sort_order"))
			}
			if request.URL.Query().Get("since_id") != "" || request.URL.Query().Get("next_token") != "page-2" {
				t.Errorf("pagination checkpoint query = %v", request.URL.Query())
			}
			_, _ = writer.Write([]byte(`{"data":[{"id":"197","text":"Older result","author_id":"u1","created_at":"2026-08-07T02:50:00Z","lang":"en"}],"includes":{"users":[{"id":"u1","username":"official"}]},"meta":{"newest_id":"197","oldest_id":"197","result_count":1}}`))
		case 3:
			if request.URL.Query().Get("sort_order") != "recency" {
				t.Errorf("daily checkpoint sort = %q", request.URL.Query().Get("sort_order"))
			}
			if request.URL.Query().Get("since_id") != "202" || request.URL.Query().Get("next_token") != "" {
				t.Errorf("poll checkpoint query = %v", request.URL.Query())
			}
			_, _ = writer.Write([]byte(`{"meta":{"result_count":0}}`))
		default:
			t.Fatalf("unexpected request %d", calls.Load())
		}
	}))
	defer server.Close()

	connector := newFixtureConnector(t, server, now, func(name string) (string, bool) {
		return map[string]string{"X_BEARER_TOKEN": "fixture-secret"}[name], name == "X_BEARER_TOKEN"
	})
	request := xFetchRequest()
	request.Query = `"AI news" -rumor`
	request.Languages = []string{"zh-CN", "en"}
	request.Regions = []string{"US", "CN"}
	first, err := connector.Fetch(context.Background(), request)
	if err != nil {
		t.Fatalf("Fetch(first): %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].ExternalID != "200" || first.Items[0].ParentExternalID != "199" || first.Items[0].Author != "official" || first.Items[0].ContentType != "post" {
		t.Fatalf("first items = %#v", first.Items)
	}
	item := first.Items[0]
	if item.URL != "https://x.com/official/status/200" || item.Body != "Public launch long-form body" || item.Language != "en" || item.EvidenceCompleteness != domain.EvidenceCompletenessFullBody {
		t.Errorf("mapped item = %#v", item)
	}
	if len(item.Attachments) != 1 || item.Attachments[0].URL != "https://pbs.twimg.com/media/fixture.jpg" {
		t.Errorf("attachments = %#v", item.Attachments)
	}
	if metric(item.Metrics.ViewCount) != 50 || metric(item.Metrics.LikeCount) != 7 || metric(item.Metrics.CommentCount) != 3 || metric(item.Metrics.ShareCount) != 3 {
		t.Errorf("metrics = %#v", item.Metrics)
	}
	if !first.HasMore || first.NextCursor == "" || diagnosticCodes(first.Diagnostics) != "possibly_sensitive_post,unavailable_post,withheld_post" {
		t.Errorf("first result = %#v", first)
	}
	if len(first.Snapshots) != 1 || first.Snapshots[0].CollectorProfileVersion.String() != "x-recent-search-json-v2" || !first.Snapshots[0].VerifyPayload() || len(item.EvidenceReferences) != 1 ||
		item.EvidenceReferences[0].SnapshotKey != first.Snapshots[0].Key || item.EvidenceReferences[0].LocatorValue != "/data/2" {
		t.Fatalf("raw evidence = snapshots %#v, references %#v", first.Snapshots, item.EvidenceReferences)
	}
	if len(item.Parties) != 3 || item.Parties[0].Role != domain.SourcePartyRoleContentOrigin ||
		item.Parties[1].Role != domain.SourcePartyRoleDistributor || item.Parties[2].Role != domain.SourcePartyRoleAuthor {
		t.Fatalf("explicit X parties = %#v", item.Parties)
	}

	request.RequestCursor = first.NextCursor
	second, err := connector.Fetch(context.Background(), request)
	if err != nil || second.HasMore || second.NextCursor == "" || len(second.Items) != 1 {
		t.Fatalf("Fetch(second) = %#v, %v", second, err)
	}
	cursor, err := decodeCursor(second.NextCursor)
	if err != nil || cursor.SinceID != "202" || cursor.NextToken != "" || cursor.HighWaterID != "" {
		t.Fatalf("final cursor = %#v, %v", cursor, err)
	}

	request.RequestCursor = second.NextCursor
	third, err := connector.Fetch(context.Background(), request)
	if err != nil || len(third.Items) != 0 || third.NextCursor != second.NextCursor {
		t.Fatalf("Fetch(third) = %#v, %v", third, err)
	}
}

func TestConnectorPreservesMissingAndExplicitZeroMetrics(t *testing.T) {
	t.Parallel()
	server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"data":[{"id":"2","text":"missing"},{"id":"1","text":"zero","public_metrics":{"impression_count":0,"like_count":0,"reply_count":0,"repost_count":0,"quote_count":0}}],"meta":{"newest_id":"2","result_count":2}}`))
	}))
	defer server.Close()
	connector := newFixtureConnector(t, server, time.Now().UTC(), tokenLookup)
	result, err := connector.Fetch(context.Background(), xFetchRequest())
	if err != nil || len(result.Items) != 2 {
		t.Fatalf("Fetch() = %#v, %v", result, err)
	}
	if result.Items[0].Metrics.ViewCount != nil || result.Items[0].Metrics.LikeCount != nil || result.Items[0].Metrics.CommentCount != nil || result.Items[0].Metrics.ShareCount != nil {
		t.Errorf("missing metrics = %#v", result.Items[0].Metrics)
	}
	if result.Items[1].Metrics.ViewCount == nil || result.Items[1].Metrics.LikeCount == nil || result.Items[1].Metrics.CommentCount == nil || result.Items[1].Metrics.ShareCount == nil {
		t.Errorf("explicit zero metrics = %#v", result.Items[1].Metrics)
	}
}

func TestConnectorLooksUpKnownPostMetricsInOneStableOfficialBatch(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 12, 8, 0, 0, 0, time.UTC)
	server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/2/tweets" || request.URL.Query().Get("ids") != "9,10" ||
			request.URL.Query().Get("tweet.fields") != "public_metrics" || request.URL.Query().Get("post.fields") != "" {
			t.Errorf("lookup request = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte(`{
			"data":[
				{"id":"10","public_metrics":{"impression_count":100,"like_count":12,"reply_count":3,"repost_count":2,"quote_count":1}},
				{"id":"9","public_metrics":{"impression_count":0,"like_count":0,"reply_count":0,"repost_count":0,"quote_count":0}}
			],
			"errors":[{"resource_type":"tweet","resource_id":"8","title":"Not Found"}]
		}`))
	}))
	defer server.Close()
	connector := newFixtureConnector(t, server, now, tokenLookup)
	result, err := connector.LookupPostMetrics(context.Background(), domain.XPostMetricLookupRequest{
		SourceConnectionID: 10, PostIDs: []string{"10", "9", "10"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 2 || result.Observations[0].PostID != "9" || result.Observations[1].PostID != "10" ||
		metric(result.Observations[0].Metrics.ViewCount) != 0 || metric(result.Observations[1].Metrics.ShareCount) != 3 {
		t.Fatalf("lookup observations = %#v", result.Observations)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].SourceExternalID != "8" ||
		len(result.Snapshots) != 1 || result.Snapshots[0].CollectorProfileVersion.String() != "x-post-lookup-json-v1" || !result.Snapshots[0].VerifyPayload() {
		t.Fatalf("lookup evidence/diagnostics = %#v / %#v", result.Snapshots, result.Diagnostics)
	}
}

func TestConnectorClassifiesRateLimitAndNeverLeaksCredential(t *testing.T) {
	t.Parallel()
	reset := time.Date(2026, time.August, 7, 4, 0, 0, 0, time.UTC)
	server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("x-rate-limit-limit", "450")
		writer.Header().Set("x-rate-limit-remaining", "0")
		writer.Header().Set("x-rate-limit-reset", "1786075200")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"detail":"fixture-secret"}`))
	}))
	defer server.Close()
	connector := newFixtureConnector(t, server, reset.Add(-time.Hour), tokenLookup)
	result, err := connector.Fetch(context.Background(), xFetchRequest())
	if domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.RateLimit.Remaining != 0 || result.RateLimit.ResetAt == nil || !result.RateLimit.ResetAt.Equal(reset) || result.RateLimit.RetryAfter == nil || !result.RateLimit.RetryAfter.Equal(reset) {
		t.Fatalf("rate limit = %#v", result.RateLimit)
	}
}

func TestConnectorClassifiesOfficialAPIResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		status    int
		body      string
		errorKind domain.CollectionErrorKind
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"detail":"fixture-secret"}`, errorKind: domain.CollectionErrorAuthentication},
		{name: "forbidden", status: http.StatusForbidden, body: `{"detail":"fixture-secret"}`, errorKind: domain.CollectionErrorAuthentication},
		{name: "bad request", status: http.StatusBadRequest, body: `{"detail":"fixture-secret"}`, errorKind: domain.CollectionErrorPermanent},
		{name: "server error", status: http.StatusServiceUnavailable, body: `{"detail":"fixture-secret"}`, errorKind: domain.CollectionErrorTemporary},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			connector := newFixtureConnector(t, server, time.Now().UTC(), tokenLookup)
			_, err := connector.Fetch(context.Background(), xFetchRequest())
			if domain.ClassifyCollectionError(err) != test.errorKind || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatalf("Fetch() error = %v", err)
			}
		})
	}
}

func TestConnectorRejectsMalformedAndOversizedResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		body      string
		errorKind domain.CollectionErrorKind
	}{
		{name: "malformed JSON", body: `{`, errorKind: domain.CollectionErrorParse},
		{name: "oversized body", body: strings.Repeat("x", maxResponseBodyBytes+1), errorKind: domain.CollectionErrorPermanent},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			connector := newFixtureConnector(t, server, time.Now().UTC(), tokenLookup)
			_, err := connector.Fetch(context.Background(), xFetchRequest())
			if domain.ClassifyCollectionError(err) != test.errorKind {
				t.Fatalf("Fetch() error = %v", err)
			}
		})
	}
}

func TestConnectorHealthUsesMinimalOfficialQuery(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 7, 4, 30, 0, 0, time.UTC)
	server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("query") != "from:XDevelopers" || request.URL.Query().Get("max_results") != "10" {
			t.Errorf("health query = %v", request.URL.Query())
		}
		_, _ = writer.Write([]byte(`{"meta":{"result_count":0}}`))
	}))
	defer server.Close()
	connector := newFixtureConnector(t, server, now, tokenLookup)
	result := connector.Health(context.Background(), xConnection())
	if !result.Healthy || !result.CheckedAt.Equal(now) || result.ErrorKind != "" || result.DiagnosticCode != "" {
		t.Fatalf("Health() = %#v", result)
	}
}

func TestConnectorRejectsMissingCredentialBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := newXTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	connector := newFixtureConnector(t, server, time.Now().UTC(), func(string) (string, bool) { return "", false })
	_, err := connector.Fetch(context.Background(), xFetchRequest())
	if domain.ClassifyCollectionError(err) != domain.CollectionErrorAuthentication || requests.Load() != 0 {
		t.Fatalf("Fetch() error/requests = %v / %d", err, requests.Load())
	}
	probe := connector.Health(context.Background(), xConnection())
	if probe.Healthy || probe.ErrorKind != domain.CollectionErrorAuthentication || probe.DiagnosticCode != "credential_unavailable" || requests.Load() != 0 {
		t.Fatalf("Health() = %#v, requests=%d", probe, requests.Load())
	}
}

func TestConnectorRejectsDisabledAndDeletedConnectionsBeforeCredentialOrNetwork(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		connection domain.SourceConnection
	}{
		{name: "disabled", connection: func() domain.SourceConnection {
			connection := xConnection()
			connection.Enabled = false
			return connection
		}()},
		{name: "deleted", connection: func() domain.SourceConnection {
			connection := xConnection()
			connection.Enabled = false
			connection.Deleted = true
			return connection
		}()},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			var credentialReads atomic.Int32
			server := newXTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
			defer server.Close()
			connector := newFixtureConnectorWithConnection(t, server, test.connection, time.Now().UTC(), func(string) (string, bool) {
				credentialReads.Add(1)
				return "fixture-secret", true
			})
			_, err := connector.Fetch(context.Background(), xFetchRequest())
			if domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent || requests.Load() != 0 || credentialReads.Load() != 0 {
				t.Fatalf("Fetch() error/requests/credentialReads = %v / %d / %d", err, requests.Load(), credentialReads.Load())
			}
		})
	}
}

func TestConnectorRejectsUnsafeDestinationAndInvalidQuery(t *testing.T) {
	t.Parallel()
	for _, connection := range []domain.SourceConnection{
		{ID: 10, SourceType: domain.SourceTypeX, Name: "X", Endpoint: "https://example.test/2/tweets/search/recent", AuthType: domain.AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN", Config: domain.DefaultSourceConfig()},
		{ID: 10, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint, AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig()},
	} {
		if _, err := New(connection, allowingRequestBudget{}); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("New(%#v) error = %v", connection, err)
		}
	}
	if _, err := compileSearchQuery(strings.Repeat("界", maxQueryCharacters+1), nil, nil); err == nil {
		t.Fatal("compileSearchQuery accepted an over-limit query")
	}
	if _, err := compileSearchQuery("safe", []string{"invalid"}, nil); err == nil {
		t.Fatal("compileSearchQuery accepted an unsupported language")
	}
}

func TestConnectorRejectsPrivateDNSBeforeDial(t *testing.T) {
	t.Parallel()
	var dialed atomic.Bool
	connector, err := newConnector(xConnection(), connectorOptions{
		lookupEnv: tokenLookup, requestBudget: allowingRequestBudget{},
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, net.ErrClosed
		},
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	_, fetchErr := connector.Fetch(context.Background(), xFetchRequest())
	if domain.ClassifyCollectionError(fetchErr) != domain.CollectionErrorPermanent || dialed.Load() {
		t.Fatalf("Fetch() error/dialed = %v / %v", fetchErr, dialed.Load())
	}
}

func TestConnectorRejectsCrossHostRedirect(t *testing.T) {
	t.Parallel()
	server := newXTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", "https://example.test/steal")
		writer.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	connector := newFixtureConnector(t, server, time.Now().UTC(), tokenLookup)
	_, err := connector.Fetch(context.Background(), xFetchRequest())
	if domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("Fetch() error = %v", err)
	}
}

func newFixtureConnector(t *testing.T, server *httptest.Server, now time.Time, lookup func(string) (string, bool)) *Connector {
	return newFixtureConnectorWithConnection(t, server, xConnection(), now, lookup)
}

func newFixtureConnectorWithConnection(t *testing.T, server *httptest.Server, connection domain.SourceConnection, now time.Time, lookup func(string) (string, bool)) *Connector {
	t.Helper()
	address := server.Listener.Addr().String()
	connector, err := newConnector(connection, connectorOptions{
		lookupEnv: lookup, requestBudget: allowingRequestBudget{}, retryWait: func(context.Context, int) error { return nil },
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
		tlsConfig: &tls.Config{InsecureSkipVerify: true}, // fixture certificate
		now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	return connector
}

func newXTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.StartTLS()
	return server
}

func xConnection() domain.SourceConnection {
	return domain.SourceConnection{
		ID: 10, SourceType: domain.SourceTypeX, Name: "X Recent Search", Endpoint: domain.XRecentSearchEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN", Config: domain.DefaultSourceConfig(), Enabled: true,
	}
}

func xFetchRequest() domain.FetchRequest {
	return domain.FetchRequest{
		CollectionRunID: 1, SourceConnectionID: 10, QuerySignature: strings.Repeat("a", 64), Query: "hotkey",
		WindowStart: time.Date(2026, time.August, 7, 2, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, time.August, 7, 3, 0, 0, 0, time.UTC), Limit: 100,
	}
}

func tokenLookup(name string) (string, bool) { return "fixture-secret", name == "X_BEARER_TOKEN" }

type allowingRequestBudget struct{}

func (allowingRequestBudget) ReserveExternalRequest(_ context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	return domain.ExternalRequestBudgetDecision{Allowed: true, Used: 1, RateUsed: 1, ResetAt: reservation.At.UTC().Add(24 * time.Hour)}, nil
}

func metric(value *int64) int64 {
	if value == nil {
		return -1
	}
	return *value
}

func diagnosticCodes(diagnostics []domain.FetchDiagnostic) string {
	codes := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes = append(codes, diagnostic.Code)
	}
	return strings.Join(codes, ",")
}

func TestCursorRejectsMalformedState(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"not-base64", encodeFixtureCursor(t, searchCursor{Version: 2}), encodeFixtureCursor(t, searchCursor{Version: 1, SinceID: "bad"})} {
		if _, err := decodeCursor(value); err == nil {
			t.Errorf("decodeCursor(%q) accepted malformed state", value)
		}
	}
}

func encodeFixtureCursor(t *testing.T, cursor searchCursor) string {
	t.Helper()
	payload, err := json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	return encodeCursorPayload(payload)
}

func TestOfficialEndpointHost(t *testing.T) {
	t.Parallel()
	parsed, err := url.Parse(domain.XRecentSearchEndpoint)
	if err != nil || parsed.Hostname() != "api.x.com" {
		t.Fatalf("XRecentSearchEndpoint = %q, %v", domain.XRecentSearchEndpoint, err)
	}
}
