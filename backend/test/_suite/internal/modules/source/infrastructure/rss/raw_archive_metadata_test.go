package rss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestConnectorProducesArchiveReadyEvidenceAndAllowlistedHeaders(t *testing.T) {
	t.Parallel()

	payload := []byte(`<?xml version="1.0"?><rss><channel><item><guid>one</guid><title>One</title></item></channel></rss>`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		writer.Header().Set("ETag", `"v1"`)
		writer.Header().Set("Set-Cookie", "upstream=must-not-survive")
		writer.Header().Set("X-Upstream-Secret", "must-not-survive")
		_, _ = writer.Write(payload)
	}))
	defer server.Close()

	result, err := newTestConnector(t, server, 1, publicResolver()).Fetch(context.Background(), testFetchRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Snapshots) != 1 || len(result.Items) != 1 || len(result.Items[0].EvidenceReferences) != 1 {
		t.Fatalf("archive metadata = %#v", result)
	}
	if result.Snapshots[0].CollectorProfileVersion.String() != CollectorProfileVersion {
		t.Fatalf("collector profile = %q, want %q", result.Snapshots[0].CollectorProfileVersion.String(), CollectorProfileVersion)
	}
	reference := result.Items[0].EvidenceReferences[0]
	if reference.SnapshotKey != result.Snapshots[0].Key || reference.LocatorType != domain.EvidenceLocatorXMLPath ||
		reference.LocatorValue == "" || reference.SelectedPayloadSHA256 == "" || reference.SelectorVersion == "" {
		t.Fatalf("evidence reference = %#v", reference)
	}
	headers := result.Snapshots[0].ResponseHeaders.Values()
	if headers["ETag"][0] != `"v1"` || headers["Content-Type"][0] != "application/rss+xml; charset=utf-8" {
		t.Fatalf("allowlisted headers = %#v", headers)
	}
	for name := range headers {
		switch name {
		case "Content-Type", "ETag", "Last-Modified", "Date", "Link", "Retry-After":
		default:
			t.Fatalf("snapshot retained non-allowlisted header %q", name)
		}
	}
	if err := NewEvidenceSelectorVerifier().Verify(evidenceSelectorInput(t, result.Snapshots[0], reference)); err != nil {
		t.Fatalf("connector emitted unverifiable evidence reference: %v", err)
	}
}
