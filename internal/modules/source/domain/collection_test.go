package domain

import (
	"strings"
	"testing"
	"time"
)

func TestCollectionFetchRequestRequiresStableWindowAndLimit(t *testing.T) {
	t.Parallel()

	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	request := FetchRequest{
		CollectionRunID:    11,
		SourceConnectionID: 17,
		QuerySignature:     strings.Repeat("a", 64),
		Query:              "climate",
		WindowStart:        windowStart,
		WindowEnd:          windowStart.Add(time.Hour),
		Limit:              25,
		RequestCursor:      "cursor-1",
		ETag:               "etag-1",
		LastModified:       "Wed, 16 Jul 2026 08:00:00 GMT",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("FetchRequest.Validate(): %v", err)
	}
	for _, invalid := range []FetchRequest{
		{CollectionRunID: 11, SourceConnectionID: 17, QuerySignature: strings.Repeat("a", 64), Query: "climate", WindowStart: windowStart, WindowEnd: windowStart.Add(time.Hour)},
		{CollectionRunID: 11, SourceConnectionID: 17, QuerySignature: strings.Repeat("a", 64), Query: "climate", WindowStart: windowStart, WindowEnd: windowStart, Limit: 1},
		{CollectionRunID: 11, SourceConnectionID: 17, QuerySignature: "not-a-signature", Query: "climate", WindowStart: windowStart, WindowEnd: windowStart.Add(time.Hour), Limit: 1},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("FetchRequest.Validate(%#v) = nil error, want required window/limit rejection", invalid)
		}
	}
}

func TestCollectionSourceItemRequiresStableExternalIDAndCapturePolicyRedacts(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	item, err := NormalizeSourceItem(SourceItem{
		SourceCode:  "rss",
		ExternalID:  "  https://feeds.example.test/posts/42  ",
		ContentType: "article",
		Title:       "A safe title",
		Body:        "body that is not retained when policy forbids it",
		Language:    "en",
		URL:         "https://feeds.example.test/posts/42",
		Author:      "Example Author",
		ObservedAt:  observedAt,
		Metrics:     SourceMetrics{ViewCount: 12, CommentCount: 3},
		RawPayload:  []byte(`{"authorization":"must-never-persist"}`),
	})
	if err != nil {
		t.Fatalf("NormalizeSourceItem(): %v", err)
	}
	if item.ExternalID != "https://feeds.example.test/posts/42" {
		t.Fatalf("normalized external ID = %q", item.ExternalID)
	}
	if _, err := NormalizeSourceItem(SourceItem{SourceCode: "rss", ContentType: "article", ObservedAt: observedAt}); err == nil {
		t.Fatal("NormalizeSourceItem() = nil error without a stable external ID")
	}

	captured, err := (CapturePolicy{Version: CapturedItemVersionV1, AllowBodyStorage: false, RawPayloadDisposition: RawPayloadDiscarded}).Capture(item)
	if err != nil {
		t.Fatalf("Capture(): %v", err)
	}
	if captured.Version != CapturedItemVersionV1 || captured.Body != "" || captured.RawPayloadDisposition != RawPayloadDiscarded {
		t.Fatalf("captured item = %#v, want versioned body-redacted discarded payload", captured)
	}
	if captured.Metrics != (SourceMetrics{ViewCount: 12, CommentCount: 3}) {
		t.Fatalf("captured metrics = %#v, want safe normalized metrics", captured.Metrics)
	}
	if string(captured.RawPayload) != "" {
		t.Fatalf("captured raw payload = %q, want no transient source bytes", captured.RawPayload)
	}
}

func TestPublishedCollectionTargetBindsCheckpointToImmutableConfiguration(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	target := PublishedCollectionTarget{
		MonitorSourceID:        31,
		MonitorConfigVersionID: 41,
		SourceConnectionID:     51,
		QuerySignature:         strings.Repeat("b", 64),
		Terms:                  []CollectionTerm{{Value: "climate"}},
		Languages:              []string{"en"},
		CollectionInterval:     5 * time.Minute,
		Checkpoint: CollectionCheckpoint{
			MonitorSourceID: 31,
			QueryHash:       strings.Repeat("b", 64),
			NextPollAt:      now,
		},
	}
	if err := target.Validate(); err != nil {
		t.Fatalf("PublishedCollectionTarget.Validate(): %v", err)
	}
	target.Checkpoint.MonitorSourceID = 32
	if err := target.Validate(); err == nil {
		t.Fatal("PublishedCollectionTarget.Validate() = nil error for a checkpoint owned by another MonitorSource")
	}
	if err := (CollectionTarget{CollectionRunID: 61, MonitorSourceID: 31, MonitorConfigVersionID: 0}).Validate(); err == nil {
		t.Fatal("CollectionTarget.Validate() = nil error without immutable published config ownership")
	}
}
