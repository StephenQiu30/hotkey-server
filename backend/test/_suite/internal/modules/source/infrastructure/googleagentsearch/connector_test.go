package googleagentsearch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestFetchUsesOfficialSearchContractMapsSnippetsAndPaginates(t *testing.T) {
	now := time.Date(2026, time.August, 7, 8, 0, 0, 0, time.UTC)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/projects/hotkey-demo/locations/global/collections/default_collection/dataStores/news/servingConfigs/default_config:search" || request.URL.RawQuery != "" {
			t.Errorf("request = %s %s", request.Method, request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var input searchRequest
		if json.NewDecoder(request.Body).Decode(&input) != nil || input.Query != "人工智能 热点" || input.PageSize != 25 || input.PageToken != "" || !input.SafeSearch || !input.ContentSearchSpec.SnippetSpec.ReturnSnippet {
			t.Errorf("body = %#v", input)
		}
		_, _ = writer.Write([]byte(`{"results":[{"id":"doc-1","document":{"id":"doc-1","derivedStructData":{"link":"https://example.com/news/1","title":"官方标题","snippets":[{"snippet":"热点 <b>摘要</b>","snippetStatus":"SUCCESS"}]}}},{"id":"doc-2","document":{"derivedStructData":{"link":"http://127.0.0.1/private","title":"无效结果"}}}],"nextPageToken":"next-page"}`))
	}))
	defer server.Close()
	result, err := fixtureConnector(t, server, now, true).Fetch(context.Background(), fetchRequest())
	if err != nil || len(result.Items) != 1 || len(result.Diagnostics) != 1 || !result.HasMore || result.NextCursor == "" {
		t.Fatalf("Fetch() = %#v, %v", result, err)
	}
	item := result.Items[0]
	if item.SourceCode != "google_agent_search" || item.ExternalID != "doc-1" || item.Title != "官方标题" || item.Body != "热点 摘要" || item.URL != "https://example.com/news/1" || item.ObservedAt != now || item.EvidenceCompleteness != domain.EvidenceCompletenessSummaryOnly {
		t.Errorf("item = %#v", item)
	}
	if result.Diagnostics[0].Code != "invalid_google_agent_search_result" || result.Diagnostics[0].SourceExternalID != "doc-2" {
		t.Errorf("diagnostics = %#v", result.Diagnostics)
	}
	cursor, err := decodeCursor(result.NextCursor, strings.Repeat("a", 64))
	if err != nil || cursor.PageToken != "next-page" {
		t.Fatalf("cursor = %#v, %v", cursor, err)
	}
}

func TestHealthExercisesSearchPermissionWhileDisabled(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input searchRequest
		if json.NewDecoder(request.Body).Decode(&input) != nil || input.Query != "HotKey connectivity check" || input.PageSize != 1 || input.ContentSearchSpec.SnippetSpec.ReturnSnippet {
			t.Errorf("health body = %#v", input)
		}
		_, _ = writer.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	result := fixtureConnector(t, server, time.Now().UTC(), false).Health(context.Background(), googleConnection(false))
	if !result.Healthy || result.ErrorKind != "" || result.DiagnosticCode != "" {
		t.Fatalf("Health() = %#v", result)
	}
}

func TestCursorIsBoundToQuerySignature(t *testing.T) {
	signature := strings.Repeat("a", 64)
	raw := encodeCursor(searchCursor{PageToken: "fixture-page", QuerySignature: signature})
	if value, err := decodeCursor(raw, signature); err != nil || value.PageToken != "fixture-page" {
		t.Fatalf("decodeCursor() = %#v, %v", value, err)
	}
	if _, err := decodeCursor(raw, strings.Repeat("b", 64)); err == nil {
		t.Fatal("decodeCursor() accepted a different query signature")
	}
}

func TestFetchClassifiesStatusWithoutLeakingResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		kind   domain.CollectionErrorKind
	}{
		{"unauthorized", http.StatusUnauthorized, domain.CollectionErrorAuthentication},
		{"forbidden", http.StatusForbidden, domain.CollectionErrorAuthentication},
		{"rate limited", http.StatusTooManyRequests, domain.CollectionErrorRateLimited},
		{"temporary", http.StatusServiceUnavailable, domain.CollectionErrorTemporary},
		{"invalid contract", http.StatusBadRequest, domain.CollectionErrorPermanent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"error":{"message":"fixture-secret-response"}}`))
			}))
			defer server.Close()
			_, err := fixtureConnector(t, server, time.Now().UTC(), true).Fetch(context.Background(), fetchRequest())
			if err == nil || domain.ClassifyCollectionError(err) != test.kind || strings.Contains(err.Error(), "fixture-secret-response") {
				t.Fatalf("Fetch() error = %v, want safe %s", err, test.kind)
			}
		})
	}
}

func TestSecureDialRejectsPrivateDNSAnswers(t *testing.T) {
	dialed := false
	dial := secureDialContext("discoveryengine.googleapis.com", func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, nil
	})
	if _, err := dial(context.Background(), "tcp", "discoveryengine.googleapis.com:443"); err == nil || dialed {
		t.Fatalf("secureDialContext() error/dialed = %v/%v", err, dialed)
	}
}

func TestSecureDialPinsValidatedPublicAddress(t *testing.T) {
	dialedAddress := ""
	dial := secureDialContext("discoveryengine.googleapis.com", func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			t.Fatalf("network = %q", network)
		}
		dialedAddress = address
		return nil, errors.New("fixture dial stopped")
	})
	if _, err := dial(context.Background(), "tcp", "discoveryengine.googleapis.com:443"); err == nil {
		t.Fatal("secureDialContext() unexpectedly succeeded")
	}
	if dialedAddress != "8.8.8.8:443" {
		t.Fatalf("dialed address = %q", dialedAddress)
	}
}

func fixtureConnector(t *testing.T, server *httptest.Server, now time.Time, enabled bool) *Connector {
	t.Helper()
	parsed, _ := url.Parse(server.URL)
	connector, err := newConnector(googleConnection(enabled), connectorOptions{
		resolver: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		dialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, parsed.Host)
		},
		tlsConfig: fixtureTLSConfig(server), now: func() time.Time { return now },
		lookupEnv: func(name string) (string, bool) { return "fixture-token", name == "GOOGLE_AGENT_SEARCH_TOKEN" },
	})
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	return connector
}

func googleConnection(enabled bool) domain.SourceConnection {
	config := domain.DefaultSourceConfig()
	config.RequiresAttribution = true
	config.GoogleLocation = "global"
	config.GoogleServingConfig = "projects/hotkey-demo/locations/global/collections/default_collection/dataStores/news/servingConfigs/default_config"
	return domain.SourceConnection{
		ID: 15, SourceType: domain.SourceTypeGoogleAgentSearch, Name: "Google 限定域搜索", Endpoint: domain.GoogleAgentSearchGlobalEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: "env:GOOGLE_AGENT_SEARCH_TOKEN", Config: config,
		Enabled: enabled, HealthStatus: domain.HealthStatusHealthy, TermsPolicyURL: domain.GoogleCloudTerms,
	}
}

func fetchRequest() domain.FetchRequest {
	now := time.Now().UTC()
	return domain.FetchRequest{
		CollectionRunID: 1, SourceConnectionID: 15, QuerySignature: strings.Repeat("a", 64), Query: " 人工智能\t热点 ",
		WindowStart: now.Add(-time.Hour), WindowEnd: now, Limit: 100,
	}
}

func fixtureTLSConfig(server *httptest.Server) *tls.Config {
	config := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	config.InsecureSkipVerify = true
	return config
}

var _ = tls.VersionTLS12
