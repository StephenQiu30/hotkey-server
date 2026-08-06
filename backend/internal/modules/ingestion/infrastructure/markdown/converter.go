// Package markdown adapts captured, authorized Feed HTML into a bounded
// Markdown projection. It never fetches URLs or renders remote resources.
package markdown

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	markdownconverter "github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

// Converter is stateless and safe to reuse. A fresh upstream converter is
// built for each call because the dependency keeps conversion-local state.
type Converter struct{}

func NewConverter() *Converter { return &Converter{} }

// Convert sanitizes one already-captured HTML field and projects it to
// CommonMark plus GFM tables. baseURL is used only for resolving relative
// links; no network request is performed.
func (*Converter) Convert(input, baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	parsedBase, err := url.Parse(baseURL)
	if err != nil || parsedBase.User != nil || !allowedAbsoluteScheme(parsedBase.Scheme) || parsedBase.Host == "" {
		return "", fmt.Errorf("markdown base URL must be an absolute HTTP(S) URL")
	}

	document, err := html.Parse(strings.NewReader(input))
	if err != nil {
		return "", fmt.Errorf("parse captured HTML: %w", err)
	}
	sanitize(document, parsedBase)

	var sanitized bytes.Buffer
	if err := html.Render(&sanitized, document); err != nil {
		return "", fmt.Errorf("render sanitized HTML: %w", err)
	}
	converter := markdownconverter.NewConverter(markdownconverter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(table.WithCellPaddingBehavior(table.CellPaddingBehaviorMinimal)),
	))
	projection, err := converter.ConvertString(sanitized.String(), markdownconverter.WithDomain(parsedBase.String()))
	if err != nil {
		return "", fmt.Errorf("convert captured HTML to Markdown: %w", err)
	}
	projection = strings.TrimSpace(projection)
	if projection == "" {
		return "", fmt.Errorf("captured HTML has no safe Markdown content")
	}
	return projection, nil
}

func sanitize(parent *html.Node, baseURL *url.URL) {
	for child := parent.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode {
			switch strings.ToLower(child.Data) {
			case "script", "style", "iframe", "form", "img", "picture", "svg", "object", "embed":
				parent.RemoveChild(child)
				child = next
				continue
			case "a":
				sanitizeLink(child, baseURL)
			}
		}
		sanitize(child, baseURL)
		child = next
	}
}

func sanitizeLink(node *html.Node, baseURL *url.URL) {
	attributes := node.Attr[:0]
	for _, attribute := range node.Attr {
		if !strings.EqualFold(attribute.Key, "href") {
			continue
		}
		target, ok := safeLink(attribute.Val, baseURL)
		if ok {
			attributes = append(attributes, html.Attribute{Key: "href", Val: target})
		}
	}
	node.Attr = attributes
}

func safeLink(value string, baseURL *url.URL) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil {
		return "", false
	}
	if parsed.IsAbs() {
		if strings.EqualFold(parsed.Scheme, "mailto") {
			return parsed.String(), true
		}
		if !allowedAbsoluteScheme(parsed.Scheme) || parsed.Host == "" {
			return "", false
		}
		return parsed.String(), true
	}
	resolved := baseURL.ResolveReference(parsed)
	if !allowedAbsoluteScheme(resolved.Scheme) || resolved.Host == "" {
		return "", false
	}
	return resolved.String(), true
}

func allowedAbsoluteScheme(scheme string) bool {
	return strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")
}
