package rss

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseFeedNormalizesRSS1RDFItems(t *testing.T) {
	t.Parallel()

	payload := []byte(`<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <item rdf:about="https://journal.example.test/articles/paper-1">
    <title>Representative journal paper</title>
    <link>https://journal.example.test/articles/paper-1</link>
    <content:encoded>Paper abstract</content:encoded>
    <dc:creator>Researcher One</dc:creator>
    <dc:date>2026-07-17</dc:date>
  </item>
</rdf:RDF>`)

	feed, err := parseFeed(payload, time.Date(2026, time.July, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parseFeed(): %v", err)
	}
	if len(feed.Items) != 1 {
		t.Fatalf("item count = %d, want 1", len(feed.Items))
	}
	item := feed.Items[0]
	if item.ExternalID != "https://journal.example.test/articles/paper-1" || item.Title != "Representative journal paper" || item.Body != "Paper abstract" || item.Author != "Researcher One" {
		t.Fatalf("item = %#v", item)
	}
	if item.PublishedAt == nil || item.PublishedAt.Format(time.DateOnly) != "2026-07-17" {
		t.Fatalf("published_at = %v, want 2026-07-17", item.PublishedAt)
	}
}

func TestParseFeedPrefersContentAcrossRSSRDFAndAtom(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 18, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "rss2 content encoded",
			payload: `<?xml version="1.0"?><rss xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><item>
				<guid>rss-content-first</guid><link>https://example.test/rss</link><title>RSS</title>
				<description>short RSS description</description><content:encoded><![CDATA[<p>full RSS content</p>]]></content:encoded>
			</item></channel></rss>`,
		},
		{
			name: "rdf content encoded",
			payload: `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:content="http://purl.org/rss/1.0/modules/content/">
				<item rdf:about="https://example.test/rdf"><link>https://example.test/rdf</link><title>RDF</title>
				<description>short RDF description</description><content:encoded><![CDATA[<p>full RDF content</p>]]></content:encoded></item>
			</rdf:RDF>`,
		},
		{
			name: "atom content",
			payload: `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry>
				<id>atom-content-first</id><title>Atom</title><link rel="alternate" href="https://example.test/atom"/>
				<summary>short Atom summary</summary><content type="html"><![CDATA[<p>full Atom content</p>]]></content>
			</entry></feed>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			feed, err := parseFeed([]byte(test.payload), observedAt)
			if err != nil {
				t.Fatalf("parseFeed() error = %v", err)
			}
			if len(feed.Items) != 1 || !strings.Contains(feed.Items[0].Body, "full") || strings.Contains(feed.Items[0].Body, "short") {
				t.Fatalf("parsed body = %#v, want full content instead of summary/description", feed.Items)
			}
		})
	}
}

func diagnosticCodes(diagnostics []fetchDiagnostic) map[string]int {
	codes := make(map[string]int, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes[diagnostic.Code]++
	}
	return codes
}
