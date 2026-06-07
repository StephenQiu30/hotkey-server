package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WeChatMPFetcher struct {
	client *http.Client
}

func NewWeChatMPFetcher(client *http.Client) *WeChatMPFetcher {
	return &WeChatMPFetcher{client: httpClient(client)}
}

func (f *WeChatMPFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeWeChatMP {
		return nil, errors.New("wechat_mp fetcher requires wechat_mp source")
	}
	if strings.TrimSpace(source.ComplianceNote) == "" {
		return nil, errors.New("wechat_mp compliance note is required")
	}

	body, err := fetchBody(ctx, f.client, source.URL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()

	payload, err := io.ReadAll(io.LimitReader(body, 5<<20))
	if err != nil {
		return nil, err
	}
	html := string(payload)

	title := extractMetaContent(html, "og:title")
	if title == "" {
		title = extractWeChatTitle(html)
	}
	if title == "" {
		title = source.URL
	}

	snippet := extractMetaContent(html, "og:description")
	if snippet == "" {
		snippet = extractWeChatBody(html)
	}

	author := extractMetaByName(html, "author")
	if author == "" {
		author = extractRichMediaMeta(html, "copyright")
	}

	coverURL := extractMetaContent(html, "og:image")

	var publishedAt *time.Time
	if ts := extractMetaContent(html, "article:published_time"); ts != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, ts); parseErr == nil {
			publishedAt = &parsed
		}
	}

	item := Item{
		Title:         strings.TrimSpace(title),
		URL:           source.URL,
		ExternalID:    source.URL,
		Snippet:       strings.TrimSpace(snippet),
		Author:        strings.TrimSpace(author),
		CoverImageURL: strings.TrimSpace(coverURL),
		PublishedAt:   publishedAt,
	}
	return []Item{item}, nil
}

// extractMetaContent reads <meta property="prop" content="value" />
func extractMetaContent(html, property string) string {
	return extractMeta(html, `property="`+property+`"`)
}

// extractMetaByName reads <meta name="name" content="value" />
func extractMetaByName(html, name string) string {
	return extractMeta(html, `name="`+name+`"`)
}

// extractRichMediaMeta reads <meta name="xxx" content="value" /> with data-role
func extractRichMediaMeta(html, name string) string {
	return extractMeta(html, `name="`+name+`"`)
}

func extractMeta(html, attr string) string {
	idx := strings.Index(html, attr)
	if idx == -1 {
		return ""
	}
	// Find the enclosing <meta tag
	tagStart := strings.LastIndex(html[:idx], "<meta")
	if tagStart == -1 {
		return ""
	}
	tagEnd := strings.Index(html[idx:], "/>")
	if tagEnd == -1 {
		tagEnd = strings.Index(html[idx:], ">")
	}
	if tagEnd == -1 {
		return ""
	}
	tag := html[tagStart : idx+tagEnd+2]

	contentIdx := strings.Index(tag, `content="`)
	if contentIdx == -1 {
		contentIdx = strings.Index(tag, `content='`)
		if contentIdx == -1 {
			return ""
		}
		quote := "'"
		start := contentIdx + len(`content='`)
		end := strings.Index(tag[start:], quote)
		if end == -1 {
			return ""
		}
		return strings.TrimSpace(tag[start : start+end])
	}
	start := contentIdx + len(`content="`)
	end := strings.Index(tag[start:], `"`)
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(tag[start : start+end])
}

// extractWeChatTitle tries to find the article title from WeChat-specific HTML structure.
func extractWeChatTitle(html string) string {
	// Try rich_media_title class
	idx := strings.Index(html, `class="rich_media_title"`)
	if idx == -1 {
		idx = strings.Index(html, `id="activity-name"`)
	}
	if idx == -1 {
		return extractTitleTag(html)
	}
	// Find the > after the tag
	tagEnd := strings.Index(html[idx:], ">")
	if tagEnd == -1 {
		return ""
	}
	start := idx + tagEnd + 1
	end := strings.Index(html[start:], "</")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(html[start : start+end])
}

// extractWeChatBody tries to extract article body text from js_content div.
func extractWeChatBody(html string) string {
	idx := strings.Index(html, `id="js_content"`)
	if idx == -1 {
		return ""
	}
	tagEnd := strings.Index(html[idx:], ">")
	if tagEnd == -1 {
		return ""
	}
	start := idx + tagEnd + 1
	end := strings.Index(html[start:], "</div>")
	if end == -1 {
		return ""
	}
	body := html[start : start+end]
	// Strip HTML tags for plain text snippet
	body = stripTags(body)
	body = strings.TrimSpace(body)
	if len(body) > 500 {
		body = body[:500]
	}
	return body
}

func extractTitleTag(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title>")
	if start == -1 {
		return ""
	}
	start += len("<title>")
	end := strings.Index(lower[start:], "</title>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(html[start : start+end])
}

func stripTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}
	return result.String()
}
