package weibo

import (
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

func TestConnectorDiscoversOfficialSearchCapabilityAndMapsPosts(t *testing.T) {
	now := time.Date(2026, time.August, 7, 7, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		switch calls.Add(1) {
		case 1:
			if request.Method != http.MethodGet || request.URL.Path != "/cli/api/cli/commands" || request.URL.Query().Get("group") != "search" || request.URL.Query().Get("access") != "available" {
				t.Errorf("capability request = %s %s", request.Method, request.URL.String())
			}
			_, _ = writer.Write([]byte(`{"commands":[{"group":"search","action":"statuses/limited","access":"allowed"}]}`))
		case 2:
			if request.Method != http.MethodPost || request.URL.Path != "/cli/api/cli/invoke" {
				t.Errorf("invoke request = %s %s", request.Method, request.URL.String())
			}
			var input invokeRequest
			if json.NewDecoder(request.Body).Decode(&input) != nil || input.Group != "search" || input.Action != "statuses/limited" || input.Args["q"] != "人工智能 热点" || input.Args["page"] != "1" || input.Args["count"] != "50" {
				t.Errorf("invoke body = %#v", input)
			}
			_, _ = writer.Write([]byte(`{"result":{"statuses":[{"id":1234567890123456,"idstr":"1234567890123456","text_raw":"官方微博正文","created_at":"Fri Aug 07 14:30:00 +0800 2026","user":{"idstr":"42","screen_name":"微博开放平台"},"reposts_count":8,"comments_count":5,"attitudes_count":13,"retweeted_status":{"idstr":"1234567890123000"}},{"idstr":"1234567890123999","text_raw":"不可见正文","deleted":true,"user":{"screen_name":"已删除"}}]}}`))
		default:
			t.Fatalf("unexpected request %d", calls.Load())
		}
	}))
	defer server.Close()
	connector := fixtureConnector(t, server, now, true)
	result, err := connector.Fetch(context.Background(), fetchRequest())
	if err != nil || len(result.Items) != 1 || result.HasMore || len(result.Diagnostics) != 1 {
		t.Fatalf("Fetch() = %#v, %v", result, err)
	}
	item := result.Items[0]
	if item.ExternalID != "1234567890123456" || item.ParentExternalID != "1234567890123000" || item.Author != "微博开放平台" || item.URL != "https://weibo.com/detail/1234567890123456" || item.PublishedAt == nil {
		t.Errorf("item = %#v", item)
	}
	if item.Metrics.LikeCount == nil || *item.Metrics.LikeCount != 13 || item.Metrics.ShareCount == nil || *item.Metrics.ShareCount != 8 {
		t.Errorf("metrics = %#v", item.Metrics)
	}
	if result.Diagnostics[0].Code != "unavailable_weibo_post" || result.Diagnostics[0].SourceExternalID != "1234567890123999" {
		t.Errorf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestHealthRejectsLockedSearchCapability(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cli/api/cli/whoami":
			_, _ = writer.Write([]byte(`{"uid":"42","developer_verified":true}`))
		case "/cli/api/cli/commands":
			_, _ = writer.Write([]byte(`{"commands":[]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	connection := weiboConnection(true)
	result := fixtureConnector(t, server, time.Now(), true).Health(context.Background(), connection)
	if result.Healthy || result.ErrorKind != domain.CollectionErrorAuthentication || result.DiagnosticCode != "capability_unavailable" {
		t.Fatalf("Health() = %#v", result)
	}
}

func TestCursorIsBoundToQuerySignature(t *testing.T) {
	signature := strings.Repeat("a", 64)
	raw := encodeCursor(searchCursor{Page: 3, QuerySignature: signature})
	if value, err := decodeCursor(raw, signature); err != nil || value.Page != 3 {
		t.Fatalf("decodeCursor() = %#v, %v", value, err)
	}
	if _, err := decodeCursor(raw, strings.Repeat("b", 64)); err == nil {
		t.Fatal("decodeCursor() accepted a different query signature")
	}
}

func TestFetchClassifiesOfficialAPIStatusWithoutLeakingResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		kind   domain.CollectionErrorKind
	}{
		{"unauthorized", http.StatusUnauthorized, domain.CollectionErrorAuthentication},
		{"subscription", http.StatusPaymentRequired, domain.CollectionErrorAuthentication},
		{"rate limited", http.StatusTooManyRequests, domain.CollectionErrorRateLimited},
		{"temporary", http.StatusServiceUnavailable, domain.CollectionErrorTemporary},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				if calls == 1 {
					_, _ = writer.Write([]byte(`{"commands":[{"group":"search","action":"statuses/limited","access":"allowed"}]}`))
					return
				}
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"error":{"code":"fixture-secret-response","message":"must not escape"}}`))
			}))
			defer server.Close()
			_, err := fixtureConnector(t, server, time.Now(), true).Fetch(context.Background(), fetchRequest())
			if err == nil || domain.ClassifyCollectionError(err) != test.kind || strings.Contains(err.Error(), "fixture-secret-response") {
				t.Fatalf("Fetch() error = %v, want safe %s", err, test.kind)
			}
		})
	}
}

func TestSecureDialRejectsPrivateDNSAnswers(t *testing.T) {
	dialed := false
	dial := secureDialContext(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, nil
	})
	if _, err := dial(context.Background(), "tcp", "open.weibo.com:443"); err == nil || dialed {
		t.Fatalf("secureDialContext() error/dialed = %v/%v", err, dialed)
	}
}

func fixtureConnector(t *testing.T, server *httptest.Server, now time.Time, enabled bool) *Connector {
	t.Helper()
	parsed, _ := url.Parse(server.URL)
	connector, err := newConnector(weiboConnection(enabled), connectorOptions{
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, parsed.Host)
		},
		tlsConfig: fixtureTLSConfig(server), now: func() time.Time { return now },
		lookupEnv: func(name string) (string, bool) { return "fixture-token", name == "WEIBO_TOKEN" },
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	return connector
}

func weiboConnection(enabled bool) domain.SourceConnection {
	config := domain.DefaultSourceConfig()
	config.RequiresAttribution = true
	config.RequiresDeletionSync = true
	return domain.SourceConnection{
		ID: 14, SourceType: domain.SourceTypeWeibo, Name: "微博关键词", Endpoint: domain.WeiboCLIApiEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: "env:WEIBO_TOKEN", Config: config,
		Enabled: enabled, HealthStatus: domain.HealthStatusHealthy, TermsPolicyURL: domain.WeiboDeveloperTerms,
	}
}

func fetchRequest() domain.FetchRequest {
	now := time.Now().UTC()
	return domain.FetchRequest{
		CollectionRunID: 1, SourceConnectionID: 14, QuerySignature: strings.Repeat("a", 64), Query: " 人工智能\t热点 ",
		WindowStart: now.Add(-time.Hour), WindowEnd: now, Limit: 100,
	}
}

func fixtureTLSConfig(server *httptest.Server) *tls.Config {
	config := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	config.InsecureSkipVerify = true
	return config
}

var _ = tls.VersionTLS12
