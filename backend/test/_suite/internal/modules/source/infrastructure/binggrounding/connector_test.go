package binggrounding

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestConnectorUsesOfficialStreamingMCPContractAndMapsDerivedEvidence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 7, 8, 0, 0, 0, time.UTC)
	var methods []string
	server := newFoundryTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Errorf("Authorization header was not supplied")
		}
		if request.Header.Get("Foundry-Features") != featureHeader {
			t.Errorf("Foundry-Features = %q", request.Header.Get("Foundry-Features"))
		}
		var call struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Errorf("decode request: %v", err)
		}
		methods = append(methods, call.Method)
		switch call.Method {
		case "initialize":
			writer.Header().Set("Mcp-Session-Id", "session-011")
			writeRPC(writer, call.ID, map[string]any{"protocolVersion": protocolVersion})
		case "notifications/initialized":
			if request.Header.Get("Mcp-Session-Id") != "session-011" {
				t.Errorf("initialized session = %q", request.Header.Get("Mcp-Session-Id"))
			}
			writer.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPC(writer, call.ID, validTools())
		case "tools/call":
			if !strings.Contains(request.Header.Get("Accept"), "text/event-stream") {
				t.Errorf("Accept = %q", request.Header.Get("Accept"))
			}
			var params struct {
				Name      string            `json:"name"`
				Arguments map[string]string `json:"arguments"`
			}
			if json.Unmarshal(call.Params, &params) != nil || params.Name != toolName || params.Arguments["search_query"] != "HotKey 发布" {
				t.Errorf("tools/call params = %#v", params)
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			payload, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": call.ID,
				"result": map[string]any{"content": []any{map[string]any{
					"type": "resource", "resource": map[string]any{
						"text": "HotKey 已发布。[来源](https://example.com/news)",
						"_meta": map[string]any{
							"action": map[string]any{"query": "HotKey 发布", "queries": []string{"HotKey 发布"}},
							"annotations": []any{
								map[string]any{"type": "url_citation", "url": "https://example.com/news", "title": "News"},
								map[string]any{"type": "url_citation", "url": "https://example.org/report", "title": "Report"},
							},
						},
					},
				}}},
			})
			_, _ = writer.Write([]byte("event: message\ndata: " + string(payload) + "\n\n"))
		default:
			http.Error(writer, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	connector := newFixtureConnector(t, server, groundingConnection(true, true), now, tokenLookup)
	result, err := connector.Fetch(context.Background(), groundingFetchRequest("HotKey 发布"))
	if err != nil {
		t.Fatalf("Fetch(): %v", err)
	}
	if got, want := strings.Join(methods, ","), "initialize,notifications/initialized,tools/list,tools/call"; got != want {
		t.Fatalf("methods = %q, want %q", got, want)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %#v", result.Items)
	}
	item := result.Items[0]
	if item.SourceCode != sourceCode || item.ContentType != "bulletin" || item.Title != modelGeneratedTitle+" · 查询：HotKey 发布" || item.Author != modelGeneratedAuthor || item.EvidenceCompleteness != domain.EvidenceCompletenessMetadataOnly {
		t.Fatalf("derived evidence markers = %#v", item)
	}
	if item.Body != "" || item.URL != "https://example.com/news" || len(item.Attachments) != 2 {
		t.Fatalf("citations = body %q URL %q attachments %#v", item.Body, item.URL, item.Attachments)
	}
	if len(result.Snapshots) != 1 || !result.Snapshots[0].VerifyPayload() || len(item.EvidenceReferences) != 1 ||
		item.EvidenceReferences[0].LocatorType != domain.EvidenceLocatorByteRange || item.EvidenceReferences[0].Usage != domain.EvidenceUsageDocumentSource {
		t.Fatalf("Bing raw evidence = %#v / %#v", result.Snapshots, item.EvidenceReferences)
	}
	if len(item.Parties) != 1 || item.Parties[0].Role != domain.SourcePartyRoleDistributor || item.Parties[0].ExternalID != "microsoft-bing-grounding" {
		t.Fatalf("Bing explicit parties = %#v", item.Parties)
	}
	if item.Metrics != (domain.SourceMetrics{}) || result.HasMore || result.NextCursor != "" {
		t.Fatalf("unsupported raw facts leaked: %#v / %#v", item.Metrics, result)
	}
}

func TestHealthOnlyValidatesToolboxAndDoesNotCallSearch(t *testing.T) {
	t.Parallel()
	var searchCalls atomic.Int32
	server := newFoundryTLSServer(t, mcpFixtureHandler(t, validTools(), &searchCalls))
	defer server.Close()
	connection := groundingConnection(true, false)
	connector := newFixtureConnector(t, server, connection, time.Now().UTC(), tokenLookup)
	result := connector.Health(context.Background(), connection)
	if !result.Healthy || result.DiagnosticCode != "" || searchCalls.Load() != 0 {
		t.Fatalf("Health() = %#v, search calls = %d", result, searchCalls.Load())
	}
}

func TestConnectorBlocksReviewCredentialAndSensitiveQueryBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := newFoundryTLSServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()

	unapproved := groundingConnection(false, true)
	unapprovedConnector := newFixtureConnector(t, server, unapproved, time.Now().UTC(), func(string) (string, bool) {
		t.Fatal("credential lookup must not run before review approval")
		return "", false
	})
	if result := unapprovedConnector.Health(context.Background(), unapproved); result.DiagnosticCode != "data_boundary_review_required" {
		t.Fatalf("Health() = %#v", result)
	}
	if _, err := unapprovedConnector.Fetch(context.Background(), groundingFetchRequest("HotKey")); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("unapproved Fetch() = %v", err)
	}
	disabled := groundingConnection(true, false)
	disabledConnector := newFixtureConnector(t, server, disabled, time.Now().UTC(), func(string) (string, bool) {
		t.Fatal("disabled fetch must not read a credential")
		return "", false
	})
	if _, err := disabledConnector.Fetch(context.Background(), groundingFetchRequest("HotKey")); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("disabled Fetch() = %v", err)
	}

	approved := groundingConnection(true, true)
	missingCredential := newFixtureConnector(t, server, approved, time.Now().UTC(), func(string) (string, bool) { return "", false })
	if result := missingCredential.Health(context.Background(), approved); result.DiagnosticCode != "credential_unavailable" {
		t.Fatalf("Health() = %#v", result)
	}
	connector := newFixtureConnector(t, server, approved, time.Now().UTC(), tokenLookup)
	for _, query := range []string{"person@example.com", "api_key=secret", "Bearer: private", "eyJabcdefgh.abcdefgh.abcdefgh"} {
		if _, err := connector.Fetch(context.Background(), groundingFetchRequest(query)); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
			t.Fatalf("Fetch(%q) = %v", query, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("blocked requests reached the network: %d", requests.Load())
	}
}

func TestHealthRejectsInvalidToolboxContract(t *testing.T) {
	t.Parallel()
	invalidTools := map[string]any{"tools": []any{map[string]any{
		"name": "web_search", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}},
	}}}
	server := newFoundryTLSServer(t, mcpFixtureHandler(t, invalidTools, nil))
	defer server.Close()
	connection := groundingConnection(true, false)
	connector := newFixtureConnector(t, server, connection, time.Now().UTC(), tokenLookup)
	result := connector.Health(context.Background(), connection)
	if result.Healthy || result.DiagnosticCode != "toolbox_contract_invalid" || result.ErrorKind != domain.CollectionErrorParse {
		t.Fatalf("Health() = %#v", result)
	}
}

func TestConnectorClassifiesSafeUpstreamFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		status int
		kind   domain.CollectionErrorKind
	}{
		{http.StatusUnauthorized, domain.CollectionErrorAuthentication},
		{http.StatusTooManyRequests, domain.CollectionErrorRateLimited},
		{http.StatusServiceUnavailable, domain.CollectionErrorTemporary},
		{http.StatusBadRequest, domain.CollectionErrorPermanent},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := newFoundryTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				http.Error(writer, "fixture-token must not appear in an error", test.status)
			}))
			defer server.Close()
			connection := groundingConnection(true, false)
			connector := newFixtureConnector(t, server, connection, time.Now().UTC(), tokenLookup)
			result := connector.Health(context.Background(), connection)
			if result.ErrorKind != test.kind || result.Healthy {
				t.Fatalf("Health() = %#v", result)
			}
		})
	}
}

func TestConnectorRejectsPrivateDNSBeforeDial(t *testing.T) {
	t.Parallel()
	var dialed atomic.Bool
	connection := groundingConnection(true, true)
	connector, err := newConnector(connection, connectorOptions{
		lookupEnv: tokenLookup,
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
	if _, err := connector.Fetch(context.Background(), groundingFetchRequest("HotKey")); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent || dialed.Load() {
		t.Fatalf("Fetch() error/dialed = %v/%v", err, dialed.Load())
	}
}

func TestConnectorRejectsCrossHostRedirectAndOversizedResponse(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		handler http.Handler
	}{
		{
			name: "cross-host redirect",
			handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Location", "https://example.com/steal")
				writer.WriteHeader(http.StatusFound)
			}),
		},
		{
			name: "oversized response",
			handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(strings.Repeat("x", maxResponseBodyBytes+1)))
			}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newFoundryTLSServer(t, test.handler)
			defer server.Close()
			connection := groundingConnection(true, true)
			connector := newFixtureConnector(t, server, connection, time.Now().UTC(), tokenLookup)
			if _, err := connector.Fetch(context.Background(), groundingFetchRequest("HotKey")); domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
				t.Fatalf("Fetch() = %v", err)
			}
		})
	}
}

func mcpFixtureHandler(t *testing.T, tools any, searchCalls *atomic.Int32) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Errorf("decode request: %v", err)
		}
		switch call.Method {
		case "initialize":
			writer.Header().Set("Mcp-Session-Id", "session")
			writeRPC(writer, call.ID, map[string]any{"protocolVersion": protocolVersion})
		case "notifications/initialized":
			writer.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPC(writer, call.ID, tools)
		case "tools/call":
			if searchCalls != nil {
				searchCalls.Add(1)
			}
			writeRPC(writer, call.ID, map[string]any{"content": []any{}})
		}
	})
}

func validTools() map[string]any {
	return map[string]any{"tools": []any{map[string]any{
		"name": toolName,
		"inputSchema": map[string]any{
			"type": "object", "properties": map[string]any{"search_query": map[string]any{"type": "string"}}, "required": []string{"search_query"},
		},
	}}}
}

func writeRPC(writer http.ResponseWriter, id int, result any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func newFixtureConnector(t *testing.T, server *httptest.Server, connection domain.SourceConnection, now time.Time, lookup func(string) (string, bool)) *Connector {
	t.Helper()
	address := server.Listener.Addr().String()
	connector, err := newConnector(connection, connectorOptions{
		lookupEnv: lookup,
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

func newFoundryTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.StartTLS()
	return server
}

func groundingConnection(approved, enabled bool) domain.SourceConnection {
	config := domain.DefaultSourceConfig()
	config.RequiresAttribution = true
	config.GroundingDataBoundaryApproved = approved
	return domain.SourceConnection{
		ID: 11, SourceType: domain.SourceTypeBingGrounding, Name: "Microsoft Foundry Web Search",
		Endpoint: "https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=v1",
		AuthType: domain.AuthTypeBearer, CredentialRef: "env:AZURE_FOUNDRY_TOKEN", Config: config, Enabled: enabled,
		TermsPolicyURL: "https://learn.microsoft.com/azure/foundry/web-search",
	}
}

func groundingFetchRequest(query string) domain.FetchRequest {
	return domain.FetchRequest{
		CollectionRunID: 1, SourceConnectionID: 11, QuerySignature: strings.Repeat("b", 64), Query: query,
		WindowStart: time.Date(2026, time.August, 7, 7, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, time.August, 7, 8, 0, 0, 0, time.UTC), Limit: 100,
	}
}

func tokenLookup(name string) (string, bool) {
	return "fixture-token", name == "AZURE_FOUNDRY_TOKEN"
}
