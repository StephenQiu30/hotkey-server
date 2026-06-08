package fetcher

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrRateLimited = errors.New("rate limited")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not found")
)

func IsRateLimited(err error) bool { return errors.Is(err, ErrRateLimited) }
func IsForbidden(err error) bool   { return errors.Is(err, ErrForbidden) }
func IsNotFound(err error) bool    { return errors.Is(err, ErrNotFound) }

type SourceType string

const (
	SourceTypeRSS         SourceType = "rss"
	SourceTypePublicPage  SourceType = "public_page"
	SourceTypeX           SourceType = "x"
	SourceTypeHackerNews  SourceType = "hackernews"
	SourceTypeWeChatMP    SourceType = "wechat_mp"
	SourceTypeZhihu       SourceType = "zhihu"
	SourceTypeReddit      SourceType = "reddit"
	SourceTypeXiaohongshu SourceType = "xiaohongshu"
	SourceTypeYouTube     SourceType = "youtube"
	SourceTypeBilibili    SourceType = "bilibili"
)

type Source struct {
	ID             string
	Type           SourceType
	URL            string
	ComplianceNote string
}

type Item struct {
	Title          string
	URL            string
	ExternalID     string
	Snippet        string
	Author         string
	CoverImageURL  string
	PublishedAt    *time.Time
	Score          int
	Descendants    int
	CommentSamples []CommentSample
}

type CommentSample struct {
	Text   string
	Author string
}

type Fetcher interface {
	Fetch(ctx context.Context, source Source) ([]Item, error)
}

// MultiFetcher dispatches Fetch calls to the registered Fetcher for each SourceType.
type MultiFetcher struct {
	fetchers map[SourceType]Fetcher
}

func NewMultiFetcher(fetchers map[SourceType]Fetcher) *MultiFetcher {
	return &MultiFetcher{fetchers: fetchers}
}

func (m *MultiFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	f, ok := m.fetchers[source.Type]
	if !ok {
		return nil, fmt.Errorf("no fetcher registered for source type %q", source.Type)
	}
	if f == nil {
		return nil, fmt.Errorf("nil fetcher registered for source type %q", source.Type)
	}
	return f.Fetch(ctx, source)
}

type RSSFetcher struct {
	client *http.Client
}

type PublicPageFetcher struct {
	client *http.Client
}

// XiaohongshuFetcher retrieves Xiaohongshu note pages and extracts titles.
type XiaohongshuFetcher struct {
	client *http.Client
}

func NewRSSFetcher(client *http.Client) *RSSFetcher {
	return &RSSFetcher{client: httpClient(client)}
}

func NewPublicPageFetcher(client *http.Client) *PublicPageFetcher {
	return &PublicPageFetcher{client: httpClient(client)}
}

// NewXiaohongshuFetcher creates a XiaohongshuFetcher with the given HTTP client.
func NewXiaohongshuFetcher(client *http.Client) *XiaohongshuFetcher {
	return &XiaohongshuFetcher{client: httpClient(client)}
}

func (f *RSSFetcher) Fetch(ctx context.Context, source Source) (items []Item, err error) {
	if source.Type != SourceTypeRSS {
		return nil, errors.New("rss fetcher requires rss source")
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

	// Try RSS 2.0 first
	var feed rssFeed
	if xmlErr := xml.Unmarshal(payload, &feed); xmlErr == nil && len(feed.Channel.Items) > 0 {
		return feed.Channel.toItems(), nil
	}

	// Try Atom
	var atomFeed atomFeed
	if xmlErr := xml.Unmarshal(payload, &atomFeed); xmlErr == nil && len(atomFeed.Entries) > 0 {
		return atomFeed.toItems(), nil
	}

	// Neither format matched — try RSS parse again to get the real error
	if err := xml.Unmarshal(payload, &feed); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return feed.Channel.toItems(), nil
}

func (f *PublicPageFetcher) Fetch(ctx context.Context, source Source) (items []Item, err error) {
	if source.Type != SourceTypePublicPage {
		return nil, errors.New("public page fetcher requires public_page source")
	}
	if strings.TrimSpace(source.ComplianceNote) == "" {
		return nil, errors.New("public page compliance note is required")
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
	payload, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		return nil, err
	}
	title := pageTitle(string(payload))
	if title == "" {
		title = source.URL
	}
	return []Item{{Title: title, URL: source.URL, ExternalID: source.URL}}, nil
}

// Fetch retrieves a Xiaohongshu note page and extracts its title.
// It requires a non-empty compliance note and validates the source type.
func (f *XiaohongshuFetcher) Fetch(ctx context.Context, source Source) (items []Item, err error) {
	if source.Type != SourceTypeXiaohongshu {
		return nil, errors.New("xiaohongshu fetcher requires xiaohongshu source")
	}
	if strings.TrimSpace(source.ComplianceNote) == "" {
		return nil, errors.New("xiaohongshu compliance note is required")
	}
	var body io.ReadCloser
	body, err = fetchBody(ctx, f.client, source.URL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()
	var payload []byte
	payload, err = io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		return nil, err
	}
	title := pageTitle(string(payload))
	if title == "" {
		title = source.URL
	}
	items = []Item{{Title: title, URL: source.URL, ExternalID: source.URL}}
	return items, nil
}

func fetchBody(ctx context.Context, client *http.Client, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "HotKeyBot/1.0 (+public-source-collection)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("fetch %s: status %d", rawURL, resp.StatusCode)
	}
	return resp.Body, nil
}

func httpClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	if client.Timeout > 0 {
		return client
	}
	clone := *client
	clone.Timeout = 15 * time.Second
	return &clone
}

func pageTitle(payload string) string {
	lower := strings.ToLower(payload)
	start := strings.Index(lower, "<title>")
	end := strings.Index(lower, "</title>")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return strings.TrimSpace(payload[start+len("<title>") : end])
}

// RSS 2.0 structures

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

func (ch rssChannel) toItems() []Item {
	items := make([]Item, 0, len(ch.Items))
	for _, ri := range ch.Items {
		item := ri.toItem()
		if item.Title == "" && item.URL == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

type rssItem struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	GUID        string        `xml:"guid"`
	PubDate     string        `xml:"pubDate"`
	Description string        `xml:"description"`
	ContentEnc  string        `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Author      string        `xml:"author"`
	Enclosure   rssEnclosure  `xml:"enclosure"`
	MediaThumb  rssMediaThumb `xml:"http://search.yahoo.com/mrss/ thumbnail"`
}

type rssEnclosure struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

type rssMediaThumb struct {
	URL string `xml:"url,attr"`
}

func (ri rssItem) toItem() Item {
	item := Item{
		Title:      strings.TrimSpace(ri.Title),
		URL:        strings.TrimSpace(ri.Link),
		ExternalID: strings.TrimSpace(ri.GUID),
		Author:     strings.TrimSpace(ri.Author),
	}
	if parsed, err := http.ParseTime(strings.TrimSpace(ri.PubDate)); err == nil {
		item.PublishedAt = &parsed
	}
	// Snippet: prefer content:encoded over description
	if s := strings.TrimSpace(ri.ContentEnc); s != "" {
		item.Snippet = s
	} else {
		item.Snippet = strings.TrimSpace(ri.Description)
	}
	// Cover: prefer media:thumbnail over enclosure
	if u := strings.TrimSpace(ri.MediaThumb.URL); u != "" {
		item.CoverImageURL = u
	} else if strings.HasPrefix(ri.Enclosure.Type, "image/") && ri.Enclosure.URL != "" {
		item.CoverImageURL = ri.Enclosure.URL
	}
	// If no GUID, use link as external ID
	if item.ExternalID == "" {
		item.ExternalID = item.URL
	}
	return item
}

// Atom structures

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

func (af atomFeed) toItems() []Item {
	items := make([]Item, 0, len(af.Entries))
	for _, e := range af.Entries {
		item := e.toItem()
		if item.Title == "" && item.URL == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

type atomEntry struct {
	Title   string     `xml:"title"`
	Link    atomLink   `xml:"link"`
	ID      string     `xml:"id"`
	Updated string     `xml:"updated"`
	Author  atomAuthor `xml:"author"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func (e atomEntry) toItem() Item {
	item := Item{
		Title:      strings.TrimSpace(e.Title),
		URL:        strings.TrimSpace(e.Link.Href),
		ExternalID: strings.TrimSpace(e.ID),
		Author:     strings.TrimSpace(e.Author.Name),
	}
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(e.Updated)); err == nil {
		item.PublishedAt = &parsed
	}
	// Snippet: prefer content over summary
	if s := strings.TrimSpace(e.Content); s != "" {
		item.Snippet = s
	} else {
		item.Snippet = strings.TrimSpace(e.Summary)
	}
	if item.ExternalID == "" {
		item.ExternalID = item.URL
	}
	return item
}
