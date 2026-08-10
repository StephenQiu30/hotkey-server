package rss

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

const (
	sourceCode               = "rss"
	CollectorProfileVersion  = "rss-http-feed-go-xml-v1"
	RSSItemSelectorVersion   = "rss2-go-xml-v1"
	RDFItemSelectorVersion   = "rss-rdf-go-xml-v1"
	AtomEntrySelectorVersion = "atom-go-xml-v1"
)

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
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Link  string    `xml:"link"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	GUID        string         `xml:"guid"`
	Link        string         `xml:"link"`
	Title       string         `xml:"title"`
	Description string         `xml:"description"`
	Content     string         `xml:"encoded"`
	PubDate     string         `xml:"pubDate"`
	Author      string         `xml:"author"`
	Publisher   string         `xml:"publisher"`
	Enclosures  []rssEnclosure `xml:"enclosure"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length string `xml:"length,attr"`
}

type rdfDocument struct {
	XMLName xml.Name   `xml:"RDF"`
	Channel rdfChannel `xml:"channel"`
	Items   []rdfItem  `xml:"item"`
}

type rdfChannel struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type rdfItem struct {
	About       string `xml:"about,attr"`
	Link        string `xml:"link"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Content     string `xml:"encoded"`
	Date        string `xml:"date"`
	Creator     string `xml:"creator"`
	Publisher   string `xml:"publisher"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	ID      string      `xml:"id"`
	Title   string      `xml:"title"`
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
	Source    atomSource   `xml:"source"`
}

type atomSource struct {
	ID    string     `xml:"id"`
	Title string     `xml:"title"`
	Links []atomLink `xml:"link"`
}

type atomLink struct {
	Rel    string `xml:"rel,attr"`
	Href   string `xml:"href,attr"`
	Type   string `xml:"type,attr"`
	Length string `xml:"length,attr"`
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
	distributor := explicitFeedParty(domain.SourcePartyRoleDistributor, "rss:feed", document.Channel.Link,
		document.Channel.Title, document.Channel.Link)
	seen := make(map[string]struct{}, len(document.Items))
	for index, entry := range document.Items {
		item, diagnostic := mapRSSItem(rssItem{
			GUID:        entry.About,
			Link:        entry.Link,
			Title:       entry.Title,
			Description: entry.Description,
			Content:     entry.Content,
			PubDate:     entry.Date,
			Author:      entry.Creator,
			Publisher:   entry.Publisher,
		}, observedAt)
		item.Parties = append(item.Parties, distributor...)
		item.Parties = append(item.Parties, explicitFeedParty(domain.SourcePartyRolePublisher, "rss:publisher", entry.Publisher, entry.Publisher, "")...)
		item, diagnostic = normalizeMappedFeedItem(item, diagnostic)
		bindXMLItemEvidence(&item, fmt.Sprintf("/rdf:RDF/item[%d]", index+1), RDFItemSelectorVersion, entry)
		feed.appendItem(item, diagnostic, seen)
	}
	return feed
}

func parsedRSS(document rssDocument, observedAt time.Time) parsedFeed {
	feed := parsedFeed{Items: make([]domain.SourceItem, 0, len(document.Channel.Items))}
	distributor := explicitFeedParty(domain.SourcePartyRoleDistributor, "rss:feed", document.Channel.Link,
		document.Channel.Title, document.Channel.Link)
	seen := make(map[string]struct{}, len(document.Channel.Items))
	for index, entry := range document.Channel.Items {
		item, diagnostic := mapRSSItem(entry, observedAt)
		item.Parties = append(item.Parties, distributor...)
		item.Parties = append(item.Parties, explicitFeedParty(domain.SourcePartyRolePublisher, "rss:publisher", entry.Publisher, entry.Publisher, "")...)
		item, diagnostic = normalizeMappedFeedItem(item, diagnostic)
		bindXMLItemEvidence(&item, fmt.Sprintf("/rss/channel/item[%d]", index+1), RSSItemSelectorVersion, entry)
		feed.appendItem(item, diagnostic, seen)
	}
	return feed
}

func parsedAtom(document atomFeed, observedAt time.Time) parsedFeed {
	feed := parsedFeed{Items: make([]domain.SourceItem, 0, len(document.Entries)), NextURL: nextAtomURL(document.Links)}
	distributor := explicitFeedParty(domain.SourcePartyRoleDistributor, "atom:feed", document.ID,
		document.Title, preferredAtomURL(document.Links))
	seen := make(map[string]struct{}, len(document.Entries))
	for index, entry := range document.Entries {
		item, diagnostic := mapAtomItem(entry, observedAt)
		item.Parties = append(item.Parties, distributor...)
		item.Parties = append(item.Parties, explicitFeedParty(domain.SourcePartyRoleContentOrigin, "atom:source", entry.Source.ID,
			entry.Source.Title, preferredAtomURL(entry.Source.Links))...)
		item, diagnostic = normalizeMappedFeedItem(item, diagnostic)
		bindXMLItemEvidence(&item, fmt.Sprintf("/feed/entry[%d]", index+1), AtomEntrySelectorVersion, entry)
		feed.appendItem(item, diagnostic, seen)
	}
	return feed
}

func bindXMLItemEvidence(item *domain.SourceItem, locator, selectorVersion string, selected any) {
	if item == nil {
		return
	}
	item.ItemLocator = locator
	payload, err := xml.Marshal(selected)
	if err != nil {
		return
	}
	digest := sha256.Sum256(payload)
	item.EvidenceReferences = []domain.EvidenceReference{{
		LocatorType: domain.EvidenceLocatorXMLPath, LocatorValue: locator,
		SelectedPayloadSHA256: fmt.Sprintf("%x", digest), SelectorVersion: selectorVersion,
	}}
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
	completeness := domain.EvidenceCompletenessFullBody
	if strings.TrimSpace(body) == "" {
		body = entry.Description
		completeness = domain.EvidenceCompletenessSummaryOnly
	}
	if strings.TrimSpace(body) == "" {
		completeness = domain.EvidenceCompletenessMetadataOnly
	}
	item, err := domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: externalID, ContentType: "article", Title: entry.Title,
		Body: body, URL: strings.TrimSpace(entry.Link), Author: entry.Author,
		PublishedAt: publishedAt, ObservedAt: observedAt.UTC(), EvidenceCompleteness: completeness,
		Attachments: rssAttachments(entry.Enclosures), Parties: feedAuthorParty(entry.Author),
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
	completeness := domain.EvidenceCompletenessFullBody
	if strings.TrimSpace(body) == "" {
		body = entry.Summary
		completeness = domain.EvidenceCompletenessSummaryOnly
	}
	if strings.TrimSpace(body) == "" {
		completeness = domain.EvidenceCompletenessMetadataOnly
	}
	author := ""
	if len(entry.Authors) > 0 {
		author = entry.Authors[0].Name
	}
	item, err := domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: externalID, ContentType: "article", Title: entry.Title,
		Body: body, URL: link, Author: author, PublishedAt: publishedAt, ObservedAt: observedAt.UTC(),
		EvidenceCompleteness: completeness, Attachments: atomAttachments(entry.Links), Parties: feedAuthorParty(author),
	})
	if err != nil {
		return domain.SourceItem{}, fetchDiagnostic{Code: "invalid_source_item", SourceExternalID: externalID}
	}
	return item, fetchDiagnostic{}
}

func feedAuthorParty(author string) []domain.SourcePartyAssertion {
	author = strings.TrimSpace(author)
	if author == "" {
		return []domain.SourcePartyAssertion{}
	}
	digest := sha256.Sum256([]byte(author))
	return []domain.SourcePartyAssertion{{
		Role: domain.SourcePartyRoleAuthor, Kind: domain.SourcePartyKindPerson,
		IdentityNamespace: "rss:author", ExternalID: fmt.Sprintf("%x", digest), DisplayName: author,
	}}
}

func explicitFeedParty(role domain.SourcePartyRole, namespace, externalID, displayName, homepageURL string) []domain.SourcePartyAssertion {
	externalID = strings.TrimSpace(externalID)
	displayName = strings.TrimSpace(displayName)
	homepageURL = safeFeedPartyHomepage(homepageURL)
	if displayName == "" {
		return []domain.SourcePartyAssertion{}
	}
	if externalID == "" || len(externalID) > 512 {
		digest := sha256.Sum256([]byte(namespace + "\x00" + displayName + "\x00" + homepageURL))
		externalID = fmt.Sprintf("%x", digest)
	}
	return []domain.SourcePartyAssertion{{
		Role: role, Kind: domain.SourcePartyKindOrganization, IdentityNamespace: namespace,
		ExternalID: externalID, DisplayName: displayName, HomepageURL: homepageURL,
	}}
}

func safeFeedPartyHomepage(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || len(value) > 2048 {
		return ""
	}
	return value
}

func normalizeMappedFeedItem(item domain.SourceItem, diagnostic fetchDiagnostic) (domain.SourceItem, fetchDiagnostic) {
	if diagnostic.Code != "" {
		return item, diagnostic
	}
	normalized, err := domain.NormalizeSourceItem(item)
	if err != nil {
		return domain.SourceItem{}, fetchDiagnostic{Code: "invalid_source_party", SourceExternalID: item.ExternalID}
	}
	return normalized, diagnostic
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
		if !strings.EqualFold(strings.TrimSpace(link.Rel), "enclosure") && strings.TrimSpace(link.Href) != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func rssAttachments(enclosures []rssEnclosure) []domain.SourceAttachment {
	attachments := make([]domain.SourceAttachment, 0, min(len(enclosures), domain.MaxSourceAttachments))
	for _, enclosure := range enclosures {
		if len(attachments) == domain.MaxSourceAttachments {
			break
		}
		if attachment, ok := sourceAttachment(enclosure.URL, enclosure.Type, enclosure.Length); ok {
			attachments = append(attachments, attachment)
		}
	}
	return attachments
}

func atomAttachments(links []atomLink) []domain.SourceAttachment {
	attachments := make([]domain.SourceAttachment, 0)
	for _, link := range links {
		if len(attachments) == domain.MaxSourceAttachments {
			break
		}
		if strings.EqualFold(strings.TrimSpace(link.Rel), "enclosure") {
			if attachment, ok := sourceAttachment(link.Href, link.Type, link.Length); ok {
				attachments = append(attachments, attachment)
			}
		}
	}
	return attachments
}

func sourceAttachment(rawURL, mimeType, rawSize string) (domain.SourceAttachment, bool) {
	rawURL = strings.TrimSpace(rawURL)
	mimeType = strings.TrimSpace(mimeType)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || len(rawURL) > 2048 || len(mimeType) > 255 {
		return domain.SourceAttachment{}, false
	}
	attachment := domain.SourceAttachment{URL: rawURL, MIMEType: mimeType}
	if size, err := strconv.ParseInt(strings.TrimSpace(rawSize), 10, 64); err == nil && size >= 0 {
		attachment.SizeBytes = &size
	}
	return attachment, true
}

func nextAtomURL(links []atomLink) string {
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(link.Rel), "next") && strings.TrimSpace(link.Href) != "" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}
