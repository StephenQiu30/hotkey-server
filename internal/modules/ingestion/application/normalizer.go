package application

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"golang.org/x/text/unicode/norm"
)

var (
	scriptOrStylePattern = regexp.MustCompile(`(?is)<\s*(?:script|style)\b[^>]*>.*?<\s*/\s*(?:script|style)\s*>`)
	htmlCommentPattern   = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTagPattern       = regexp.MustCompile(`(?s)<[^>]*>`)
)

// NormalizeCapturedItem converts a Source-owned, already-persisted capture to
// Content facts. It never calls a connector and it does not retain an upstream
// response beyond the capture fields Source has already allowed to persist.
func NormalizeCapturedItem(item sourcedomain.CapturedItem, sourceConnectionID int64) (ingestiondomain.NormalizedContent, error) {
	if sourceConnectionID <= 0 || (item.Version != sourcedomain.CapturedItemVersionV1 && item.Version != sourcedomain.CapturedItemVersionV2) || strings.TrimSpace(item.SourceCode) == "" || strings.TrimSpace(item.ExternalID) == "" || item.ObservedAt.IsZero() {
		return ingestiondomain.NormalizedContent{}, ingestiondomain.NewError(ingestiondomain.ErrorCodeInvalidCapturedItem)
	}

	title := normalizeText(item.Title)
	body := normalizeText(item.Body)
	if title == "" && body == "" {
		return ingestiondomain.NormalizedContent{}, ingestiondomain.NewError(ingestiondomain.ErrorCodeEmptyContent)
	}
	contentType, err := normalizeContentType(item.SourceCode, item.ContentType)
	if err != nil {
		return ingestiondomain.NormalizedContent{}, err
	}
	canonicalURL, err := normalizeCanonicalURL(item.URL)
	if err != nil {
		return ingestiondomain.NormalizedContent{}, err
	}
	metrics, err := cloneMetrics(item.Metrics)
	if err != nil {
		return ingestiondomain.NormalizedContent{}, err
	}
	publishedAt := item.ObservedAt.UTC()
	if item.PublishedAt != nil && !item.PublishedAt.IsZero() {
		publishedAt = item.PublishedAt.UTC()
	}
	language := strings.TrimSpace(norm.NFC.String(item.Language))
	if language == "" {
		language = "und"
	}
	content := ingestiondomain.NormalizedContent{
		SourceConnectionID: sourceConnectionID,
		ExternalID:         normalizeText(item.ExternalID),
		ContentType:        contentType,
		Title:              title,
		Excerpt:            body,
		Body:               body,
		CanonicalURL:       canonicalURL,
		Language:           language,
		Author:             normalizedAuthor(sourceConnectionID, item.Author),
		PublishedAt:        publishedAt,
		FetchedAt:          item.ObservedAt.UTC(),
		ContentHash:        contentHash(title, body),
		Metrics:            metrics,
	}
	if err := content.Validate(); err != nil {
		return ingestiondomain.NormalizedContent{}, err
	}
	return content, nil
}

func normalizeContentType(sourceCode, contentType string) (string, error) {
	sourceCode = strings.ToLower(strings.TrimSpace(sourceCode))
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if sourceCode == "hacker_news" && contentType == "comment" {
		contentType = "post"
	}
	switch contentType {
	case "article", "post", "video", "podcast", "bulletin":
		return contentType, nil
	default:
		return "", ingestiondomain.NewError(ingestiondomain.ErrorCodeInvalidContentType)
	}
}

func normalizeCanonicalURL(rawURL string) (string, error) {
	parsed, err := url.Parse(norm.NFC.String(strings.TrimSpace(rawURL)))
	if err != nil || parsed == nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", ingestiondomain.NewError(ingestiondomain.ErrorCodeInvalidCanonicalURL)
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port == "" {
		if strings.Contains(hostname, ":") {
			parsed.Host = "[" + hostname + "]"
		} else {
			parsed.Host = hostname
		}
	} else {
		parsed.Host = net.JoinHostPort(hostname, port)
	}
	parsed.Scheme = scheme
	parsed.User = nil
	parsed.Fragment = ""
	parsed.ForceQuery = false
	if parsed.Path == "" {
		parsed.Path = "/"
	} else if parsed.Path != "/" {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		if parsed.Path == "" {
			parsed.Path = "/"
		}
	}
	parsed.RawPath = ""
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", ingestiondomain.NewError(ingestiondomain.ErrorCodeInvalidCanonicalURL)
	}
	for key := range query {
		if isTrackingQueryKey(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func isTrackingQueryKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if strings.HasPrefix(key, "utm_") {
		return true
	}
	switch key {
	case "fbclid", "gclid", "dclid", "msclkid", "mc_cid", "mc_eid", "igshid", "yclid", "vero_conv", "vero_id":
		return true
	default:
		return false
	}
}

func normalizeText(raw string) string {
	withoutMarkup := scriptOrStylePattern.ReplaceAllString(norm.NFC.String(raw), " ")
	withoutMarkup = htmlCommentPattern.ReplaceAllString(withoutMarkup, " ")
	withoutMarkup = htmlTagPattern.ReplaceAllStringFunc(withoutMarkup, htmlTagSeparator)
	decoded := html.UnescapeString(withoutMarkup)
	var cleaned strings.Builder
	cleaned.Grow(len(decoded))
	for _, character := range decoded {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			cleaned.WriteByte(' ')
			continue
		}
		cleaned.WriteRune(character)
	}
	return strings.Join(strings.Fields(cleaned.String()), " ")
}

func htmlTagSeparator(tag string) string {
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(tag, "<"), ">"))
	name = strings.TrimLeft(name, "/")
	if index := strings.IndexFunc(name, unicode.IsSpace); index >= 0 {
		name = name[:index]
	}
	name = strings.TrimRight(strings.ToLower(name), "/")
	switch name {
	case "address", "article", "blockquote", "br", "div", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "ol", "p", "pre", "section", "table", "td", "th", "tr", "ul":
		return " "
	default:
		return ""
	}
}

func normalizedAuthor(sourceConnectionID int64, raw string) ingestiondomain.NormalizedAuthor {
	displayName := normalizeText(raw)
	if displayName == "" {
		return ingestiondomain.NormalizedAuthor{}
	}
	identifier := strings.ToLower(displayName)
	hash := sha256.Sum256([]byte("source:" + strconv.FormatInt(sourceConnectionID, 10) + "\x00" + identifier))
	return ingestiondomain.NormalizedAuthor{ExternalID: hex.EncodeToString(hash[:]), DisplayName: displayName}
}

func contentHash(title, body string) string {
	hash := sha256.Sum256([]byte(title + "\x00" + body))
	return hex.EncodeToString(hash[:])
}

func cloneMetrics(metrics sourcedomain.SourceMetrics) (sourcedomain.SourceMetrics, error) {
	clone := func(value *int64) (*int64, error) {
		if value == nil {
			return nil, nil
		}
		if *value < 0 {
			return nil, ingestiondomain.NewError(ingestiondomain.ErrorCodeInvalidCapturedItem)
		}
		copied := *value
		return &copied, nil
	}
	viewCount, err := clone(metrics.ViewCount)
	if err != nil {
		return sourcedomain.SourceMetrics{}, err
	}
	likeCount, err := clone(metrics.LikeCount)
	if err != nil {
		return sourcedomain.SourceMetrics{}, err
	}
	commentCount, err := clone(metrics.CommentCount)
	if err != nil {
		return sourcedomain.SourceMetrics{}, err
	}
	shareCount, err := clone(metrics.ShareCount)
	if err != nil {
		return sourcedomain.SourceMetrics{}, err
	}
	return sourcedomain.SourceMetrics{ViewCount: viewCount, LikeCount: likeCount, CommentCount: commentCount, ShareCount: shareCount}, nil
}
