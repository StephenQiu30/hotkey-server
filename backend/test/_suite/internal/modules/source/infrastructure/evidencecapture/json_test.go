package evidencecapture

import (
	"net/http"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestJSONEvidenceSnapshotAndSelectorPreserveUnknownFields(t *testing.T) {
	payload := []byte(`{"data":[{"id":"1","text":"body","unknown":{"n":1.0}}]}`)
	snapshot, err := NewJSONSnapshot(payload, "x-api-v1", "https://api.x.com/2/tweets/search/recent", "https://api.x.com/2/tweets/search/recent", nil, 200, http.Header{
		"Content-Type":  []string{"application/json; charset=UTF-8"},
		"Authorization": []string{"secret"},
		"Set-Cookie":    []string{"secret=1"},
	}, time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewJSONSnapshot() error = %v", err)
	}
	item := domain.SourceItem{}
	if err := BindJSONPointer(&item, snapshot, "/data/0", domain.EvidenceUsageDocumentSource); err != nil {
		t.Fatalf("BindJSONPointer() error = %v", err)
	}
	selected, err := SelectJSONPointer(snapshot.Payload, item.EvidenceReferences[0].LocatorValue)
	if err != nil || string(selected) != `{"id":"1","text":"body","unknown":{"n":1.0}}` {
		t.Fatalf("SelectJSONPointer() = %s, %v", selected, err)
	}
	if headers := snapshot.ResponseHeaders.Values(); headers["Authorization"] != nil || headers["Set-Cookie"] != nil || headers["Content-Type"][0] != "application/json; charset=utf-8" {
		t.Fatalf("allowlisted headers = %#v", headers)
	}
}

func TestSelectJSONPointerRejectsAmbiguousOrMissingLocations(t *testing.T) {
	for _, pointer := range []string{"", "data/0", "/data/00", "/data/2", "/data/~2"} {
		if _, err := SelectJSONPointer([]byte(`{"data":[1]}`), pointer); err == nil {
			t.Fatalf("SelectJSONPointer(%q) succeeded", pointer)
		}
	}
}

func TestHTTPSnapshotUsesSemanticDefaultMIMEWithoutPretendingSSEIsJSON(t *testing.T) {
	payload := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\"}\n\n")
	snapshot, err := NewHTTPSnapshot(payload, "text/event-stream", "bing-mcp-v1",
		"https://search.example.test/mcp", "https://search.example.test/mcp", nil, 200, http.Header{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewHTTPSnapshot() error = %v", err)
	}
	if snapshot.MIMEType != "text/event-stream" || string(snapshot.Payload) != string(payload) {
		t.Fatalf("SSE snapshot = %#v", snapshot)
	}
	if _, err := NewHTTPSnapshot(payload, "", "bing-mcp-v1",
		"https://search.example.test/mcp", "https://search.example.test/mcp", nil, 200, http.Header{}, time.Now().UTC()); err == nil {
		t.Fatal("NewHTTPSnapshot() accepted an empty MIME contract")
	}
}
