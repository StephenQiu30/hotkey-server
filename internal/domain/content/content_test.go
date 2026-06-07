package content

import "testing"

func TestCanonicalURLRemovesTrackingAndNormalizesHostPath(t *testing.T) {
	got, err := CanonicalURL("HTTPS://Example.COM:443/news/../News/?utm_source=rss&id=42&fbclid=abc#section")
	if err != nil {
		t.Fatalf("canonical URL failed: %v", err)
	}
	want := "https://example.com/News?id=42"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCanonicalURLRejectsInvalidURL(t *testing.T) {
	if _, err := CanonicalURL("not a url"); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestContentHashIsStableAcrossWhitespace(t *testing.T) {
	first := ContentHash(HashInput{
		Title:        "  AI   新闻 ",
		Snippet:      "正文\n片段",
		CanonicalURL: "https://example.com/a",
	})
	second := ContentHash(HashInput{
		Title:        "AI 新闻",
		Snippet:      "正文 片段",
		CanonicalURL: "https://example.com/a",
	})
	if first == "" || first != second {
		t.Fatalf("expected stable hash, got %q and %q", first, second)
	}
}

func TestSourceItemMetadataOnlyDefaultFalse(t *testing.T) {
	item := SourceItem{ID: "item-1", Title: "test"}
	if item.MetadataOnly {
		t.Fatal("expected MetadataOnly to default to false")
	}
}

func TestSourceItemMetadataOnlyCanBeSet(t *testing.T) {
	item := SourceItem{ID: "item-2", Title: "paywall article", MetadataOnly: true}
	if !item.MetadataOnly {
		t.Fatal("expected MetadataOnly to be true after setting")
	}
}

func TestCloneItemCopiesChannelIDs(t *testing.T) {
	original := SourceItem{ID: "item-1", ChannelIDs: []string{"chn_ai_models"}}
	cloned := cloneItem(original)
	cloned.ChannelIDs[0] = "mutated"

	if original.ChannelIDs[0] != "chn_ai_models" {
		t.Fatalf("expected original channel IDs to be isolated, got %#v", original.ChannelIDs)
	}
}
