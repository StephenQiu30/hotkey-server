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

type SourceType string

const (
	SourceTypeRSS        SourceType = "rss"
	SourceTypePublicPage SourceType = "public_page"
)

type Source struct {
	ID             string
	Type           SourceType
	URL            string
	ComplianceNote string
}

type Item struct {
	Title       string
	URL         string
	ExternalID  string
	PublishedAt *time.Time
}

type Fetcher interface {
	Fetch(ctx context.Context, source Source) ([]Item, error)
}

type RSSFetcher struct {
	client *http.Client
}

type PublicPageFetcher struct {
	client *http.Client
}

func NewRSSFetcher(client *http.Client) *RSSFetcher {
	return &RSSFetcher{client: httpClient(client)}
}

func NewPublicPageFetcher(client *http.Client) *PublicPageFetcher {
	return &PublicPageFetcher{client: httpClient(client)}
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

	var feed rssFeed
	if err := xml.NewDecoder(body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
	}
	items = make([]Item, 0, len(feed.Channel.Items))
	for _, rssItem := range feed.Channel.Items {
		item := Item{
			Title:      strings.TrimSpace(rssItem.Title),
			URL:        strings.TrimSpace(rssItem.Link),
			ExternalID: strings.TrimSpace(rssItem.GUID),
		}
		if parsed, err := http.ParseTime(strings.TrimSpace(rssItem.PubDate)); err == nil {
			item.PublishedAt = &parsed
		}
		if item.Title == "" && item.URL == "" {
			continue
		}
		items = append(items, item)
	}
	return items, nil
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

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
}
