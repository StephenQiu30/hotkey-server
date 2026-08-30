package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testToken = "live-smoke-token-that-must-never-enter-report"
	testQuery = "private live smoke query canary"
)

func TestExecuteUsesRealAPIBoundariesAndWritesOnlySanitizedEvidence(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+testToken {
			t.Errorf("authorization header was not forwarded to the local API")
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/source-connections":
			writeSourceList(t, writer)
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/source-connections/"):
			id, sourceType := sourceFixture(request.URL.Path)
			credentialConfigured := sourceType == "x"
			writeEnvelope(t, writer, map[string]any{
				"id": id, "source_type": sourceType, "enabled": true, "deleted": false,
				"credential_configured": credentialConfigured,
				"name":                  "sensitive source name", "endpoint": "https://private.example.invalid/path",
			})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/health"):
			writeEnvelope(t, writer, map[string]any{"healthy": true, "checked_at": time.Now().UTC()})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/search":
			body, err := io.ReadAll(request.Body)
			if err != nil || !strings.Contains(string(body), testQuery) {
				t.Errorf("search query was not sent to the API")
			}
			writeEnvelope(t, writer, map[string]any{
				"query":   testQuery,
				"results": []map[string]any{{"title": "private result payload canary"}},
				"source_statuses": []map[string]any{
					{"source_type": "rss", "source_name": "private-rss", "state": "success", "result_count": 3},
					{"source_type": "hacker_news", "source_name": "private-hn", "state": "success", "result_count": 2},
					{"source_type": "x", "source_name": "private-x", "state": "success", "result_count": 1},
				},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	cfg := validConfig(t, server.URL)
	clock := steppedClock(time.Date(2026, 8, 29, 13, 0, 0, 0, time.UTC), 5*time.Millisecond)
	result, passed := execute(t.Context(), server.Client(), cfg, clock)
	if !passed || result.Status != "passed" {
		t.Fatalf("expected a passing live smoke report: %+v", result)
	}
	if len(result.Sources) != 3 {
		t.Fatalf("expected three source results, got %d", len(result.Sources))
	}
	if result.Sources[2].SourceType != "x" || !result.Sources[2].CredentialConfigured {
		t.Fatalf("expected safe X credential readiness metadata: %+v", result.Sources[2])
	}
	if err := writeReport(cfg.outputPath, result); err != nil {
		t.Fatalf("write report: %v", err)
	}
	content, err := os.ReadFile(cfg.outputPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	for _, forbidden := range []string{
		testToken, testQuery, "private result payload canary", "sensitive source name",
		"private.example.invalid", "private-rss", "private-hn", "private-x",
	} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("sanitized report contains forbidden value %q", forbidden)
		}
	}
	if result.QueryBytes != len(testQuery) {
		t.Fatalf("expected query metadata without query text: %+v", result)
	}
}

func TestLoadConfigFailsClosedBeforeNetworkOrOutput(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if _, err := loadConfig(func(string) string { return "" }); err == nil {
		t.Fatal("expected missing admin token to fail closed")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("configuration rejection must not create evidence: %v, entries=%d", err, len(entries))
	}
}

func TestLoadConfigUsesFixedLocalValuesAndOnlyReadsAdminToken(t *testing.T) {
	t.Parallel()
	overrides := map[string]string{
		"HOTKEY_SOURCE_LIVE_SMOKE_ADMIN_TOKEN": testToken,
		"HOTKEY_SOURCE_LIVE_SMOKE_BASE_URL":    "http://external.example.com",
		"HOTKEY_SOURCE_LIVE_SMOKE_QUERY":       "override query",
		"HOTKEY_SOURCE_LIVE_SMOKE_ENVIRONMENT": "override-environment",
		"HOTKEY_SOURCE_LIVE_SMOKE_REVISION":    "override-revision",
		"HOTKEY_SOURCE_LIVE_SMOKE_OUTPUT":      filepath.Join(t.TempDir(), "override.json"),
	}
	cfg, err := loadConfig(func(key string) string {
		return overrides[key]
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.baseURL.String() != "http://127.0.0.1:8866/" || cfg.query != defaultQuery || cfg.environment != defaultEnvironment {
		t.Fatalf("unexpected local defaults: %+v", cfg)
	}
	if cfg.revision == "" || cfg.revision == overrides["HOTKEY_SOURCE_LIVE_SMOKE_REVISION"] || filepath.Ext(cfg.outputPath) != ".json" || cfg.outputPath == overrides["HOTKEY_SOURCE_LIVE_SMOKE_OUTPUT"] || len(cfg.sources) != 0 {
		t.Fatalf("derived configuration is incomplete: %+v", cfg)
	}
}

func TestExecuteRedactsAPIFailuresAndDoesNotFollowRedirect(t *testing.T) {
	t.Parallel()
	var redirected atomic.Int64
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	t.Cleanup(redirectTarget.Close)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/source-connections/11/health" {
			writer.Header().Set("Location", redirectTarget.URL+"/credential-canary")
			writer.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/source-connections":
			writeSourceList(t, writer)
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/api/v1/source-connections/"):
			id, sourceType := sourceFixture(request.URL.Path)
			writeEnvelope(t, writer, map[string]any{"id": id, "source_type": sourceType, "enabled": true, "deleted": false, "credential_configured": sourceType == "x"})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/health"):
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"message":"` + testToken + ` upstream private response"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/search":
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte(`{"message":"` + testQuery + `"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	cfg := validConfig(t, server.URL)
	result, passed := execute(t.Context(), client, cfg, time.Now)
	if passed || result.Status != "failed" {
		t.Fatalf("expected failed report: %+v", result)
	}
	if redirected.Load() != 0 {
		t.Fatalf("redirect target received %d requests", redirected.Load())
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for _, forbidden := range []string{testToken, testQuery, "upstream private response", redirectTarget.URL} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("failed report contains forbidden value %q", forbidden)
		}
	}
	if result.Sources[0].Health.ErrorCode != "http_307" {
		t.Fatalf("expected only stable redirect status, got %+v", result.Sources[0].Health)
	}
}

func TestExecuteStopsBeforeExternalProbeWhenSourceIdentityIsWrong(t *testing.T) {
	t.Parallel()
	var externalRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/source-connections" {
			writeSourceList(t, writer)
			return
		}
		if request.Method != http.MethodGet {
			externalRequests.Add(1)
			http.Error(writer, "unexpected external request", http.StatusInternalServerError)
			return
		}
		id, sourceType := sourceFixture(request.URL.Path)
		if id == 12 {
			sourceType = "x"
		}
		writeEnvelope(t, writer, map[string]any{"id": id, "source_type": sourceType, "enabled": true, "deleted": false, "credential_configured": true})
	}))
	t.Cleanup(server.Close)
	cfg := validConfig(t, server.URL)
	result, passed := execute(t.Context(), server.Client(), cfg, time.Now)
	if passed || result.Status != "failed" {
		t.Fatalf("expected mismatched source identity to fail: %+v", result)
	}
	if externalRequests.Load() != 0 {
		t.Fatalf("source identity mismatch triggered %d external requests", externalRequests.Load())
	}
	if result.Sources[1].Preflight.ErrorCode != "source_precondition_failed" || result.Sources[1].Health.ErrorCode != "preflight_not_passed" {
		t.Fatalf("expected stable preflight rejection: %+v", result.Sources[1])
	}
}

func TestExecuteStopsBeforeHealthWhenSourceSelectionIsAmbiguous(t *testing.T) {
	t.Parallel()
	var externalRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/source-connections" {
			writeEnvelope(t, writer, map[string]any{
				"items": []map[string]any{
					{"id": 10, "source_type": "rss", "enabled": true, "deleted": false},
					{"id": 11, "source_type": "rss", "enabled": true, "deleted": false},
					{"id": 12, "source_type": "hacker_news", "enabled": true, "deleted": false},
					{"id": 13, "source_type": "x", "enabled": true, "deleted": false, "credential_configured": true},
				},
				"next_cursor": "",
			})
			return
		}
		externalRequests.Add(1)
		http.Error(writer, "unexpected external request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	result, passed := execute(t.Context(), server.Client(), validConfig(t, server.URL), time.Now)
	if passed || result.ErrorCode != "source_selection_ambiguous" {
		t.Fatalf("ambiguous sources must fail with a stable code: %+v", result)
	}
	if externalRequests.Load() != 0 {
		t.Fatalf("ambiguous source selection triggered %d external requests", externalRequests.Load())
	}
}

func TestWriteReportNeverOverwritesEvidence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("prepare evidence: %v", err)
	}
	if err := writeReport(path, report{Version: reportVersion}); err == nil {
		t.Fatal("expected evidence overwrite to be rejected")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "original" {
		t.Fatalf("existing evidence changed: %q, %v", content, err)
	}
}

func TestStableProviderCodeAllowsOnlyApplicationCodes(t *testing.T) {
	t.Parallel()
	if got := stableProviderCode("rate_limited"); got != "rate_limited" {
		t.Fatalf("expected approved stable code, got %q", got)
	}
	for _, untrusted := range []string{testToken, "private.provider.message", "UPSTREAM_SECRET"} {
		if got := stableProviderCode(untrusted); got != "upstream_error" {
			t.Fatalf("expected untrusted provider code to be collapsed, got %q", got)
		}
	}
}

func validConfig(t *testing.T, serverURL string) config {
	t.Helper()
	baseURL, err := validateBaseURL(serverURL)
	if err != nil {
		t.Fatalf("validate server URL: %v", err)
	}
	return config{
		baseURL: baseURL, token: testToken, query: testQuery,
		environment: "trusted-test", revision: strings.Repeat("a", 40),
		outputPath: filepath.Join(t.TempDir(), "source-live-smoke.json"),
	}
}

func writeSourceList(t *testing.T, writer http.ResponseWriter) {
	t.Helper()
	writeEnvelope(t, writer, map[string]any{
		"items": []map[string]any{
			{"id": 11, "source_type": "rss", "enabled": true, "deleted": false, "credential_configured": false},
			{"id": 12, "source_type": "hacker_news", "enabled": true, "deleted": false, "credential_configured": false},
			{"id": 13, "source_type": "x", "enabled": true, "deleted": false, "credential_configured": true},
		},
		"next_cursor": "",
	})
}

func sourceFixture(path string) (int64, string) {
	trimmed := strings.TrimSuffix(path, "/health")
	idText := trimmed[strings.LastIndex(trimmed, "/")+1:]
	switch idText {
	case "11":
		return 11, "rss"
	case "12":
		return 12, "hacker_news"
	case "13":
		return 13, "x"
	default:
		return 0, "unknown"
	}
}

func writeEnvelope(t *testing.T, writer http.ResponseWriter, data any) {
	t.Helper()
	if err := json.NewEncoder(writer).Encode(map[string]any{"code": 0, "message": "success", "data": data}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func steppedClock(start time.Time, step time.Duration) func() time.Time {
	current := start.Add(-step)
	return func() time.Time {
		current = current.Add(step)
		return current
	}
}
