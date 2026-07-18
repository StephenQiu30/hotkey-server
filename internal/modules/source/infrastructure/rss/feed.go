package rss

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

const sourceCode = "rss"

type parsedFeed struct {
	Items       []domain.SourceItem
	Diagnostics []fetchDiagnostic
	NextURL     string
}

type fetchDiagnostic struct {
	Code             string
	SourceExternalID string
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Link        string `xml:"link"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Content     string `xml:"encoded"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author"`
}

type rdfDocument struct {
	Items []rdfItem `xml:"item"`
}

type rdfItem struct {
	About       string `xml:"about,attr"`
	Link        string `xml:"link"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Content     string `xml:"encoded"`
	Date        string `xml:"date"`
	Creator     string `xml:"creator"`
}

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
	Links   []atomLink  `xml:"link"`
}

type atomEntry struct {
	ID        string       `xml:"id"`
	Title     string       `xml:"title"`
	Summary   string       `xml:"summary"`
	Content   string       `xml:"content"`
	Published string       `xml:"published"`
	Updated   string       `xml:"updated"`
	Links     []atomLink   `xml:"link"`
	Authors   []atomAuthor `xml:"author"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func parseFeed(payload []byte, observedAt time.Time) (parsedFeed, error) {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	for {
		token, err := decoder.Token()
		if err != nil {
			return parsedFeed{}, fmt.Errorf("read feed XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "rss":
			var document rssDocument
			if err := decoder.DecodeElement(&document, &start); err != nil {
				return parsedFeed{}, fmt.Errorf("decode RSS feed: %w", err)
			}
			return parsedRSS(document, observedAt), nil
		case "RDF":
			var document rdfDocument
			if err := decoder.DecodeElement(&document, &start); err != nil {
				return parsedFeed{}, fmt.Errorf("decode RSS 1.0 feed: %w", err)
			}
			return parsedRDF(document, observedAt), nil
		case "feed":
			var feed atomFeed
			if err := decoder.DecodeElement(&feed, &start); err != nil {
				return parsedFeed{}, fmt.Errorf("decode Atom feed: %w", err)
			}
			return parsedAtom(feed, observedAt), nil
		default:
			return parsedFeed{}, fmt.Errorf("unsupported feed root %q", start.Name.Local)
		}
	}
}

func parsedRDF(document rdfDocument, observedAt time.Time) parsedFeed {
	feed := parsedFeed{Items: make([]domain.SourceItem, 0, len(document.Items))}
	seen := make(map[string]struct{}, len(document.Items))
	for _, entry := range document.Items {
		description := entry.Content
		if strings.TrimSpace(description) == "" {
			description = entry.Description
		}
		item, diagnostic := mapRSSItem(rssItem{
			GUID:        entry.About,
			Link:        entry.Link,
			Title:       entry.Title,
			Description: description,
			PubDate:     entry.Date,
			Author:      entry.Creator,
		}, observedAt)
		feed.appendItem(item, diagnostic, seen)
	}
	return feed
}

func parsedRSS(document rssDocument, observedAt time.Time) parsedFeed {
	feed := parsedFeed{Items: make([]domain.SourceItem, 0, len(document.Channel.Items))}
	seen := make(map[string]struct{}, len(document.Channel.Items))
	for _, entry := range document.Channel.Items {
		item, diagnostic := mapRSSItem(entry, observedAt)
		feed.appendItem(item, diagnostic, seen)
	}
	return feed
}

func parsedAtom(document atomFeed, observedAt time.Time) parsedFeed {
	feed := parsedFeed{Items: make([]domain.SourceItem, 0, len(document.Entries)), NextURL: nextAtomURL(document.Links)}
	seen := make(map[string]struct{}, len(document.Entries))
	for _, entry := range document.Entries {
		item, diagnostic := mapAtomItem(entry, observedAt)
		feed.appendItem(item, diagnostic, seen)
	}
	return feed
}

func (feed *parsedFeed) appendItem(item domain.SourceItem, diagnostic fetchDiagnostic, seen map[string]struct{}) {
	if diagnostic.Code != "" {
		feed.Diagnostics = append(feed.Diagnostics, diagnostic)
		return
	}
	if _, duplicate := seen[item.ExternalID]; duplicate {
		feed.Diagnostics = append(feed.Diagnostics, fetchDiagnostic{Code: "duplicate_external_id", SourceExternalID: item.ExternalID})
		return
	}
	seen[item.ExternalID] = struct{}{}
	feed.Items = append(feed.Items, item)
}

func mapRSSItem(entry rssItem, observedAt time.Time) (domain.SourceItem, fetchDiagnostic) {
	externalID, code := stableExternalID(entry.GUID, entry.Link)
	if code != "" {
		return domain.SourceItem{}, fetchDiagnostic{Code: code}
	}
	publishedAt, code := parsePublishedAt(entry.PubDate, rssTimeLayouts...)
	if code != "" {
		return domain.SourceItem{}, fetchDiagnostic{Code: code, SourceExternalID: externalID}
	}
	body := entry.Content
	if strings.TrimSpace(body) == "" {
		body = entry.Description
	}
	item, err := domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: externalID, ContentType: "article", Title: entry.Title,
		Body: body, URL: strings.TrimSpace(entry.Link), Author: entry.Author,
		PublishedAt: publishedAt, ObservedAt: observedAt.UTC(),
	})
	if err != nil {
		return domain.SourceItem{}, fetchDiagnostic{Code: "invalid_source_item", SourceExternalID: externalID}
	}
	return item, fetchDiagnostic{}
}

func mapAtomItem(entry atomEntry, observedAt time.Time) (domain.SourceItem, fetchDiagnostic) {
	link := preferredAtomURL(entry.Links)
	externalID, code := stableExternalID(entry.ID, link)
	if code != "" {
		return domain.SourceItem{}, fetchDiagnostic{Code: code}
	}
	published := entry.Published
	if strings.TrimSpace(published) == "" {
		published = entry.Updated
	}
	publishedAt, code := parsePublishedAt(published, time.RFC3339, time.RFC3339Nano)
	if code != "" {
		return domain.SourceItem{}, fetchDiagnostic{Code: code, SourceExternalID: externalID}
	}
	body := entry.Content
	if strings.TrimSpace(body) == "" {
		body = entry.Summary
	}
	author := ""
	if len(entry.Authors) > 0 {
		author = entry.Authors[0].Name
	}
	item, err := domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: externalID, ContentType: "article", Title: entry.Title,
		Body: body, URL: link, Author: author, PublishedAt: publishedAt, ObservedAt: observedAt.UTC(),
	})
	if err != nil {
		return domain.SourceItem{}, fetchDiagnostic{Code: "invalid_source_item", SourceExternalID: externalID}
	}
	return item, fetchDiagnostic{}
}

var rssTimeLayouts = []string{time.RFC1123Z, time.RFC1123, time.RFC850, time.ANSIC, time.RFC3339, time.RFC3339Nano, time.DateOnly}

func parsePublishedAt(value string, layouts ...string) (*time.Time, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ""
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			utc := parsed.UTC()
			return &utc, ""
		}
	}
	return nil, "invalid_published_at"
}

func stableExternalID(preferred, link string) (string, string) {
	if preferred = strings.TrimSpace(preferred); preferred != "" {
		return preferred, ""
	}
	normalized, err := normalizedURL(link)
	if err != nil {
		return "", "missing_external_id"
	}
	return "url:" + normalized, ""
}

func normalizedURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	if (parsed.Scheme == "https" && parsed.Port() == "443") || (parsed.Scheme == "http" && parsed.Port() == "80") {
		parsed.Host = parsed.Hostname()
	}
	return parsed.String(), nil
}

func preferredAtomURL(links []atomLink) string {
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(link.Rel), "alternate") && strings.TrimSpace(link.Href) != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	for _, link := range links {
		if strings.TrimSpace(link.Href) != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func nextAtomURL(links []atomLink) string {
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(link.Rel), "next") && strings.TrimSpace(link.Href) != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}
