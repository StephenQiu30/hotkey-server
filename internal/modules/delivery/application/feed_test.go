package application

import (
	"strings"
	"testing"
	"time"
)

func TestRenderFeedsProduceStableValidators(t *testing.T) {
	feed := Feed{Title: "HotKey", Link: "https://example.test/feed", UpdatedAt: time.Unix(10, 0).UTC(), Items: []FeedItem{{ID: "evt-1", Title: "Event", URL: "https://example.test/e", PublishedAt: time.Unix(9, 0).UTC()}}}
	rss, rssETag, err := RenderRSS(feed)
	if err != nil || rssETag == "" || !strings.Contains(string(rss), "<guid>evt-1</guid>") {
		t.Fatalf("rss = %q/%s/%v", rss, rssETag, err)
	}
	atom, atomETag, err := RenderAtom(feed)
	if err != nil || atomETag == "" || !strings.Contains(string(atom), "http://www.w3.org/2005/Atom") {
		t.Fatalf("atom = %q/%s/%v", atom, atomETag, err)
	}
}
