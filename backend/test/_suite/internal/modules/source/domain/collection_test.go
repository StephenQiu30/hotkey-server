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
		SourceCode:           "rss",
		ExternalID:           "  https://feeds.example.test/posts/42  ",
		ParentExternalID:     "  parent-41  ",
		ContentType:          "article",
		Title:                "A safe title",
		Body:                 "body that is not retained when policy forbids it",
		Language:             "en",
		URL:                  "https://feeds.example.test/posts/42",
		Author:               "Example Author",
		ObservedAt:           observedAt,
		EvidenceCompleteness: EvidenceCompletenessSummaryOnly,
		Attachments:          []SourceAttachment{{URL: "https://cdn.example.test/report.pdf", MIMEType: "application/pdf", SizeBytes: int64Pointer(1024)}},
		Metrics:              SourceMetrics{ViewCount: int64Pointer(12), CommentCount: int64Pointer(3)},
		RawPayload:           []byte(`{"authorization":"must-never-persist"}`),
	})
	if err != nil {
		t.Fatalf("NormalizeSourceItem(): %v", err)
	}
	if item.ExternalID != "https://feeds.example.test/posts/42" {
		t.Fatalf("normalized external ID = %q", item.ExternalID)
	}
	if item.ParentExternalID != "parent-41" {
		t.Fatalf("normalized parent external ID = %q, want parent-41", item.ParentExternalID)
	}
	if _, err := NormalizeSourceItem(SourceItem{SourceCode: "rss", ContentType: "article", ObservedAt: observedAt}); err == nil {
		t.Fatal("NormalizeSourceItem() = nil error without a stable external ID")
	}

	captured, err := (CapturePolicy{Version: CapturedItemVersionV2, AllowBodyStorage: false, RawPayloadDisposition: RawPayloadDiscarded}).Capture(item)
	if err != nil {
		t.Fatalf("Capture(): %v", err)
	}
	if captured.Version != CapturedItemVersionV2 || captured.Body != "" || captured.RawPayloadDisposition != RawPayloadDiscarded {
		t.Fatalf("captured item = %#v, want versioned body-redacted discarded payload", captured)
	}
	if captured.ParentExternalID != "parent-41" {
		t.Fatalf("captured parent external ID = %q, want parent-41", captured.ParentExternalID)
	}
	if captured.EvidenceCompleteness != EvidenceCompletenessMetadataOnly || len(captured.Attachments) != 1 || captured.Attachments[0].URL != "https://cdn.example.test/report.pdf" {
		t.Fatalf("captured evidence metadata = %#v / %#v, want metadata-only with preserved attachment", captured.EvidenceCompleteness, captured.Attachments)
	}
	if captured.Metrics.ViewCount == nil || *captured.Metrics.ViewCount != 12 || captured.Metrics.CommentCount == nil || *captured.Metrics.CommentCount != 3 || captured.Metrics.LikeCount != nil || captured.Metrics.ShareCount != nil {
		t.Fatalf("captured metrics = %#v, want safe normalized metrics", captured.Metrics)
	}
	if string(captured.RawPayload) != "" {
		t.Fatalf("captured raw payload = %q, want no transient source bytes", captured.RawPayload)
	}
}

func TestCapturePolicyV2PreservesUnknownAndExplicitZeroMetrics(t *testing.T) {
	t.Parallel()

	zero := int64(0)
	observedAt := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	captured, err := (CapturePolicy{
		Version:               CapturedItemVersionV2,
		RawPayloadDisposition: RawPayloadDiscarded,
	}).Capture(SourceItem{
		SourceCode:  "rss",
		ExternalID:  "metric-presence",
		ContentType: "article",
		Title:       "Metric presence",
		URL:         "https://feeds.example.test/metric-presence",
		ObservedAt:  observedAt,
		Metrics: SourceMetrics{
			ViewCount:    &zero,
			CommentCount: int64Pointer(7),
		},
	})
	if err != nil {
		t.Fatalf("Capture(): %v", err)
	}
	if captured.Version != CapturedItemVersionV2 {
		t.Fatalf("captured version = %q, want %q", captured.Version, CapturedItemVersionV2)
	}
	if captured.Metrics.ViewCount == nil || *captured.Metrics.ViewCount != 0 {
		t.Fatalf("captured explicit zero view count = %#v, want pointer to 0", captured.Metrics.ViewCount)
	}
	if captured.Metrics.LikeCount != nil || captured.Metrics.ShareCount != nil {
		t.Fatalf("captured unknown metrics = %#v, want nil", captured.Metrics)
	}
	if captured.Metrics.CommentCount == nil || *captured.Metrics.CommentCount != 7 {
		t.Fatalf("captured comment count = %#v, want pointer to 7", captured.Metrics.CommentCount)
	}
}

func TestNormalizeSourceItemRejectsContradictoryOrUnsafeEvidenceMetadata(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	base := SourceItem{SourceCode: "rss", ExternalID: "evidence-validation", ContentType: "article", Title: "Evidence", ObservedAt: observedAt}
	tests := []struct {
		name   string
		mutate func(*SourceItem)
	}{
		{"summary without body", func(item *SourceItem) { item.EvidenceCompleteness = EvidenceCompletenessSummaryOnly }},
		{"metadata with body", func(item *SourceItem) {
			item.Body = "body"
			item.EvidenceCompleteness = EvidenceCompletenessMetadataOnly
		}},
		{"invalid completeness", func(item *SourceItem) { item.EvidenceCompleteness = "guessed" }},
		{"credential attachment", func(item *SourceItem) {
			item.Attachments = []SourceAttachment{{URL: "https://user:secret@cdn.example.test/file"}}
		}},
		{"negative attachment size", func(item *SourceItem) {
			item.Attachments = []SourceAttachment{{URL: "https://cdn.example.test/file", SizeBytes: int64Pointer(-1)}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := base
			test.mutate(&item)
			if _, err := NormalizeSourceItem(item); err == nil {
				t.Fatalf("NormalizeSourceItem(%s) error = nil", test.name)
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }

func TestCompileCollectionQueryIsCanonicalQuotedAndBounded(t *testing.T) {
	t.Parallel()

	terms := []CollectionTerm{
		{Value: " job listing ", Excluded: true},
		{Value: "OpenAI"},
		{Value: "artificial intelligence"},
	}
	query, err := CompileCollectionQuery("", terms)
	if err != nil {
		t.Fatalf("CompileCollectionQuery(): %v", err)
	}
	if query != `OpenAI "artificial intelligence" -"job listing"` {
		t.Fatalf("compiled query = %q", query)
	}
	reordered, err := CompileCollectionQuery("", []CollectionTerm{terms[1], terms[2], terms[0]})
	if err != nil || reordered != query {
		t.Fatalf("reordered query = %q, err=%v; want stable %q", reordered, err, query)
	}
	if _, err := CompileCollectionQuery(strings.Repeat("x", MaxCollectionQueryBytes+1), terms); err == nil {
		t.Fatal("CompileCollectionQuery() accepted an override above the hard byte limit")
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
