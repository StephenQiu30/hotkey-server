package rss

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseFeedNormalizesRSSAndAtomItems(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		path       string
		externalID string
		title      string
		wantItems  int
	}{
		{"rss", "testdata/rss/news.xml", "rss-100", "RSS headline", 2},
		{"atom", "testdata/atom/news.xml", "atom-200", "Atom headline", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := os.ReadFile(filepath.Clean(test.path))
			if err != nil {
				t.Fatalf("ReadFile(): %v", err)
			}
			feed, err := parseFeed(payload, observedAt)
			if err != nil {
				t.Fatalf("parseFeed(): %v", err)
			}
			if len(feed.Items) != test.wantItems {
				t.Fatalf("item count = %d, want %d", len(feed.Items), test.wantItems)
			}
			if feed.Items[0].SourceCode != "rss" || feed.Items[0].ExternalID != test.externalID || feed.Items[0].Title != test.title {
				t.Fatalf("first item = %#v", feed.Items[0])
			}
		})
	}
}

func TestParseFeedSkipsBadOrDuplicateItemsWithSafeDiagnostics(t *testing.T) {
	t.Parallel()

	payload := []byte(`<?xml version="1.0"?><rss><channel>
		<item><guid>kept</guid><title>Kept</title><link>https://news.example.test/kept</link></item>
		<item><title>No stable ID</title></item>
		<item><guid>kept</guid><title>Duplicate</title><link>https://news.example.test/duplicate</link></item>
		<item><guid>bad-date</guid><title>Bad date</title><pubDate>not-a-date</pubDate></item>
	</channel></rss>`)
	feed, err := parseFeed(payload, time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parseFeed(): %v", err)
	}
	if len(feed.Items) != 1 || feed.Items[0].ExternalID != "kept" {
		t.Fatalf("items = %#v, want only the valid stable item", feed.Items)
	}
	if got := diagnosticCodes(feed.Diagnostics); got["missing_external_id"] != 1 || got["duplicate_external_id"] != 1 || got["invalid_published_at"] != 1 {
		t.Fatalf("diagnostics = %#v, want stable safe validation codes", feed.Diagnostics)
	}
}

func TestParseFeedRejectsMalformedXML(t *testing.T) {
	t.Parallel()

	if _, err := parseFeed([]byte("<rss><channel><item>"), time.Now().UTC()); err == nil {
		t.Fatal("parseFeed() error = nil, want malformed XML rejection")
	}
}

func diagnosticCodes(diagnostics []fetchDiagnostic) map[string]int {
	codes := make(map[string]int, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes[diagnostic.Code]++
	}
	return codes
}
