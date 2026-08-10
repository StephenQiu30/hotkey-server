package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeSourceItemRequiresExplicitEvidenceUsageAfterNormalization(t *testing.T) {
	item, err := NormalizeSourceItem(SourceItem{
		SourceCode: "rss", ExternalID: "item-1", ContentType: "article",
		ObservedAt: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC), EvidenceCompleteness: EvidenceCompletenessMetadataOnly,
		EvidenceReferences: []EvidenceReference{{
			SnapshotKey: strings.Repeat("a", 64), LocatorType: EvidenceLocatorWholePayload,
			LocatorValue: "/", SelectedPayloadSHA256: strings.Repeat("b", 64), SelectorVersion: WholePayloadSelectorVersion,
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeSourceItem() error = %v", err)
	}
	if got := item.EvidenceReferences[0].Usage; got != EvidenceUsageDocumentSource {
		t.Fatalf("default evidence usage = %q, want %q", got, EvidenceUsageDocumentSource)
	}

	item.EvidenceReferences[0].Usage = EvidenceUsageContext
	if _, err := NormalizeSourceItem(item); err != nil {
		t.Fatalf("NormalizeSourceItem(context) error = %v", err)
	}
	item.EvidenceReferences[0].Usage = "unknown"
	if _, err := NormalizeSourceItem(item); err == nil {
		t.Fatal("NormalizeSourceItem(unknown usage) succeeded")
	}
}
