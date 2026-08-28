package rss

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

type ssrfFixture struct {
	DirectDNS           []ssrfDNSCase `json:"direct_dns"`
	ObfuscatedEndpoints []string      `json:"obfuscated_endpoints"`
}

type ssrfDNSCase struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func TestSSRFAcceptanceRejectsDirectAndObfuscatedDestinationsBeforeDial(t *testing.T) {
	fixture := readSSRFFixture(t)

	for _, test := range fixture.DirectDNS {
		t.Run(test.Name, func(t *testing.T) {
			var lookups atomic.Int64
			var dials atomic.Int64
			connector := newSSRFTestConnector(t,
				func(context.Context, string) ([]net.IPAddr, error) {
					lookups.Add(1)
					return []net.IPAddr{{IP: net.ParseIP(test.Address)}}, nil
				},
				func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)
					return nil, errors.New("fixture dial must not run")
				},
				nil,
			)

			for attempt := 0; attempt < 2; attempt++ {
				result, err := connector.Fetch(context.Background(), testFetchRequest())
				if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
					t.Fatalf("attempt %d error = %v, class = %q; want permanent rejection", attempt+1, err, domain.ClassifyCollectionError(err))
				}
				if len(result.Items) != 0 || len(result.Snapshots) != 0 {
					t.Fatalf("attempt %d produced business facts: %#v", attempt+1, result)
				}
			}
			if got := dials.Load(); got != 0 {
				t.Fatalf("network dial count = %d, want 0", got)
			}
			if got := lookups.Load(); got != 2 {
				t.Fatalf("DNS lookup count = %d, want one decision per repeated attempt", got)
			}
		})
	}

	for _, endpoint := range fixture.ObfuscatedEndpoints {
		t.Run("obfuscated_"+endpoint, func(t *testing.T) {
			connection := ssrfSourceConnection()
			connection.Endpoint = endpoint
			if _, err := newConnector(connection, connectorOptions{requestBudget: allowingRequestBudget{}}); err == nil {
				t.Fatalf("newConnector(%q) accepted an obfuscated IP endpoint", endpoint)
			}
		})
	}
}

func TestSSRFAcceptanceRevalidatesRedirectDNSBeforeFollowingPrivateHop(t *testing.T) {
	var pathMu sync.Mutex
	pathCounts := map[string]int{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		pathMu.Lock()
		pathCounts[request.URL.Path]++
		pathMu.Unlock()
		if request.URL.Path == "/rss" {
			http.Redirect(writer, request, "https://feeds.example.test/private", http.StatusFound)
			return
		}
		_, _ = writer.Write([]byte(`<?xml version="1.0"?><rss><channel><item><guid>must-not-exist</guid></item></channel></rss>`))
	}))
	defer server.Close()

	var lookups atomic.Int64
	var dials atomic.Int64
	decisions := make([]string, 0, 2)
	var decisionMu sync.Mutex
	connector := newSSRFTestConnector(t,
		func(context.Context, string) ([]net.IPAddr, error) {
			lookup := lookups.Add(1)
			address := "8.8.8.8"
			decision := "public"
			if lookup > 1 {
				address = "127.0.0.1"
				decision = "loopback"
			}
			decisionMu.Lock()
			decisions = append(decisions, decision)
			decisionMu.Unlock()
			return []net.IPAddr{{IP: net.ParseIP(address)}}, nil
		},
		func(ctx context.Context, network, _ string) (net.Conn, error) {
			dials.Add(1)
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		server,
	)

	result, err := connector.Fetch(context.Background(), testFetchRequest())
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("redirect error = %v, class = %q; want permanent rejection", err, domain.ClassifyCollectionError(err))
	}
	if len(result.Items) != 0 || len(result.Snapshots) != 0 {
		t.Fatalf("private redirect produced facts: %#v", result)
	}
	pathMu.Lock()
	startRequests, privateRequests := pathCounts["/rss"], pathCounts["/private"]
	pathMu.Unlock()
	if startRequests != 1 || privateRequests != 0 {
		t.Fatalf("redirect fixture path counts = /rss:%d /private:%d, want 1/0", startRequests, privateRequests)
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("dial count = %d, want only the public first hop", got)
	}
	if got := lookups.Load(); got != 2 {
		t.Fatalf("DNS lookup count = %d, want one per redirect hop", got)
	}
	decisionMu.Lock()
	gotDecisions := append([]string(nil), decisions...)
	decisionMu.Unlock()
	if len(gotDecisions) != 2 || gotDecisions[0] != "public" || gotDecisions[1] != "loopback" {
		t.Fatalf("sanitized DNS decisions = %#v, want [public loopback]", gotDecisions)
	}
}

func newSSRFTestConnector(t *testing.T, resolver lookupIPAddrFunc, dial func(context.Context, string, string) (net.Conn, error), server *httptest.Server) *Connector {
	t.Helper()
	options := connectorOptions{
		resolver: resolver, dialContext: dial, requestBudget: allowingRequestBudget{},
		now: func() time.Time { return time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC) },
	}
	if server != nil {
		options.tlsConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // local redirect fixture only
	}
	connector, err := newConnector(ssrfSourceConnection(), options)
	if err != nil {
		t.Fatalf("newConnector(): %v", err)
	}
	return connector
}

func ssrfSourceConnection() domain.SourceConnection {
	return domain.SourceConnection{
		ID: 7, SourceType: domain.SourceTypeRSS, Name: "SSRF Fixture", Endpoint: "https://feeds.example.test/rss",
		AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true,
	}
}

func readSSRFFixture(t *testing.T) ssrfFixture {
	t.Helper()
	payload, err := os.ReadFile("testdata/security/ssrf.json")
	if err != nil {
		t.Fatalf("read SSRF fixture: %v", err)
	}
	var fixture ssrfFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode SSRF fixture: %v", err)
	}
	if len(fixture.DirectDNS) == 0 || len(fixture.ObfuscatedEndpoints) == 0 {
		t.Fatal("SSRF fixture must contain direct DNS and obfuscated endpoint cases")
	}
	return fixture
}
