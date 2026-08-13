package hotspot

import (
	"testing"
	"time"
)

func TestNormalizeURLIsSharedAndTrackingSafe(t *testing.T) {
	got, err := NormalizeURL(" HTTPS://BÜCHER.example:443/news/item/?utm_source=feed&b=2&fbclid=ignored#read ")
	if err != nil {
		t.Fatalf("NormalizeURL() error = %v", err)
	}
	if got != "https://xn--bcher-kva.example/news/item?b=2" {
		t.Fatalf("NormalizeURL() = %q", got)
	}
	for _, invalid := range []string{"", "ftp://example.test/file", "https://user:pass@example.test/private"} {
		if _, err := NormalizeURL(invalid); err == nil {
			t.Fatalf("NormalizeURL(%q) accepted invalid URL", invalid)
		}
	}
}

func TestCardHeatDedupeAndOrdering(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	likes, comments := int64(12), int64(5)
	hot := Card{SourceType: "hacker_news", ExternalID: "1", CanonicalURL: "https://example.test/a", DiscoveredAt: now, Metrics: Metrics{LikeCount: &likes, CommentCount: &comments}}
	hot.HeatScore, hot.Importance = HeatScore(hot.Metrics), Importance(HeatScore(hot.Metrics))
	duplicateID := hot
	duplicateID.CanonicalURL = "https://example.test/other"
	duplicateURL := hot
	duplicateURL.SourceType, duplicateURL.ExternalID = "rss", "2"
	cold := Card{SourceType: "rss", ExternalID: "3", CanonicalURL: "https://example.test/c", DiscoveredAt: now.Add(time.Minute)}

	cards := Dedupe([]Card{cold, hot, duplicateID, duplicateURL})
	if len(cards) != 2 || hot.HeatScore <= 0 || hot.Importance != ImportanceMedium {
		t.Fatalf("dedupe/heat result = %#v, hot = %#v", cards, hot)
	}
	SortByHeat(cards)
	if cards[0].ExternalID != "1" {
		t.Fatalf("sorted cards = %#v, want hot card first", cards)
	}
}
