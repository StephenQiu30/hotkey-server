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
