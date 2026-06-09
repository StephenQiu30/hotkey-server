package fetcher

import (
	"regexp"
	"strings"
	"time"
)

// ArticleMetadata holds extracted metadata from an HTML article page.
type ArticleMetadata struct {
	CanonicalURL    string
	Language        string
	PublishedAt     *time.Time
	Description     string
	PaywallDetected bool
}

// ExtractArticleMetadata parses HTML and extracts article metadata including
// canonical URL, language, publish time, description, and paywall indicators.
func ExtractArticleMetadata(html string, fallbackURL string) ArticleMetadata {
	result := ArticleMetadata{
		CanonicalURL: fallbackURL,
	}

	// Canonical URL: <link rel="canonical" href="..." />
	if href := extractMetaAttr(html, `link[^>]+rel\s*=\s*"canonical"`, "href"); href != "" {
		result.CanonicalURL = href
	}

	// Language: <html lang="zh-CN"> or <meta http-equiv="content-language" content="ja" />
	if lang := extractHTMLLang(html); lang != "" {
		result.Language = lang
	} else if lang := extractMetaAttr(html, `meta[^>]+http-equiv\s*=\s*"content-language"`, "content"); lang != "" {
		result.Language = normalizeLang(lang)
	}

	// Published time: <meta property="article:published_time" content="..." />
	if ts := extractMetaAttr(html, `meta[^>]+property\s*=\s*"article:published_time"`, "content"); ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			result.PublishedAt = &parsed
		}
	}
	// Fallback: <time datetime="..." />
	if result.PublishedAt == nil {
		if ts := extractTimeTag(html); ts != "" {
			if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
				result.PublishedAt = &parsed
			}
		}
	}

	// Description: <meta name="description" content="..." />
	if desc := extractMetaAttr(html, `meta[^>]+name\s*=\s*"description"`, "content"); desc != "" {
		result.Description = desc
	}
	// Fallback: <meta property="og:description" content="..." />
	if result.Description == "" {
		if desc := extractMetaAttr(html, `meta[^>]+property\s*=\s*"og:description"`, "content"); desc != "" {
			result.Description = desc
		}
	}

	// Paywall detection
	result.PaywallDetected = detectPaywall(html)

	return result
}

// extractMetaAttr finds an element matching the tagPattern regex and extracts
// the named attribute from it.
func extractMetaAttr(html string, tagPattern string, attr string) string {
	re := regexp.MustCompile(`(?i)<` + tagPattern + `[^>]*>`)
	match := re.FindString(html)
	if match == "" {
		return ""
	}
	return extractAttr(match, attr)
}

// extractAttr extracts a quoted attribute value from a tag string.
func extractAttr(tag string, attr string) string {
	// Match attr="value" or attr='value'
	re := regexp.MustCompile(`(?i)` + attr + `\s*=\s*"([^"]*)"`)
	if m := re.FindStringSubmatch(tag); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	re = regexp.MustCompile(`(?i)` + attr + `\s*=\s*'([^']*)'`)
	if m := re.FindStringSubmatch(tag); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractHTMLLang extracts the lang attribute from <html lang="...">.
func extractHTMLLang(html string) string {
	re := regexp.MustCompile(`(?i)<html[^>]+lang\s*=\s*"([^"]*)"`)
	if m := re.FindStringSubmatch(html); len(m) > 1 {
		return normalizeLang(m[1])
	}
	re = regexp.MustCompile(`(?i)<html[^>]+lang\s*=\s*'([^']*)'`)
	if m := re.FindStringSubmatch(html); len(m) > 1 {
		return normalizeLang(m[1])
	}
	return ""
}

// normalizeLang extracts the primary language subtag (e.g. "zh-CN" → "zh").
func normalizeLang(lang string) string {
	lang = strings.TrimSpace(lang)
	if idx := strings.Index(lang, "-"); idx > 0 {
		return lang[:idx]
	}
	if idx := strings.Index(lang, "_"); idx > 0 {
		return lang[:idx]
	}
	return strings.ToLower(lang)
}

// extractTimeTag extracts the datetime attribute from the first <time datetime="..."> tag.
func extractTimeTag(html string) string {
	re := regexp.MustCompile(`(?i)<time[^>]+datetime\s*=\s*"([^"]*)"`)
	if m := re.FindStringSubmatch(html); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	re = regexp.MustCompile(`(?i)<time[^>]+datetime\s*=\s*'([^']*)'`)
	if m := re.FindStringSubmatch(html); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// paywallPatterns are generic indicators of paywall content.
var paywallPatterns = []string{
	`class="paywall"`,
	`class='paywall'`,
	`id="paywall"`,
	`id='paywall'`,
	`class="subscriber-only"`,
	`class="premium-content"`,
	`class="metered-paywall"`,
}

// paywallTextPatterns are text-based paywall indicators found in body content.
var paywallTextPatterns = []string{
	"subscribe to continue reading",
	"available only to subscribers",
	"please log in to read",
	"premium content",
	"this article is for subscribers",
	"subscribe to read",
	"members only",
	"此内容仅限订阅者",
	"请登录后阅读",
	"订阅后可查看全文",
}

// detectPaywall checks for common paywall indicators in HTML.
func detectPaywall(html string) bool {
	lower := strings.ToLower(html)

	// Check structural paywall markers
	for _, pattern := range paywallPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}

	// Check text-based paywall indicators
	for _, text := range paywallTextPatterns {
		if strings.Contains(lower, text) {
			return true
		}
	}

	return false
}
