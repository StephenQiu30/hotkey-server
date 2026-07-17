package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"time"
)

type FeedItem struct {
	ID, Title, URL, Summary string
	PublishedAt             time.Time
}
type Feed struct {
	Title, Link string
	UpdatedAt   time.Time
	Items       []FeedItem
}
type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Title   string    `xml:"title"`
	Link    string    `xml:"link"`
	Updated string    `xml:"lastBuildDate"`
	Items   []rssItem `xml:"item"`
}
type rssItem struct {
	ID          string `xml:"guid"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Published   string `xml:"pubDate"`
}

type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Entries []atomEntry `xml:"entry"`
}
type atomEntry struct {
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Link    atomLink `xml:"link"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary"`
}
type atomLink struct {
	Href string `xml:"href,attr"`
}

func RenderRSS(feed Feed) ([]byte, string, error) {
	if feed.Title == "" || feed.Link == "" || feed.UpdatedAt.IsZero() {
		return nil, "", fmt.Errorf("invalid feed")
	}
	items := make([]rssItem, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item.ID == "" || item.Title == "" || item.URL == "" {
			return nil, "", fmt.Errorf("invalid feed item")
		}
		items = append(items, rssItem{ID: item.ID, Title: item.Title, Link: item.URL, Description: item.Summary, Published: item.PublishedAt.UTC().Format(time.RFC1123Z)})
	}
	document := rssDocument{Version: "2.0", Channel: rssChannel{Title: feed.Title, Link: feed.Link, Updated: feed.UpdatedAt.UTC().Format(time.RFC1123Z), Items: items}}
	body, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, "", err
	}
	body = append([]byte(xml.Header), body...)
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}

func RenderAtom(feed Feed) ([]byte, string, error) {
	if feed.Title == "" || feed.Link == "" || feed.UpdatedAt.IsZero() {
		return nil, "", fmt.Errorf("invalid feed")
	}
	entries := make([]atomEntry, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item.ID == "" || item.Title == "" || item.URL == "" {
			return nil, "", fmt.Errorf("invalid feed item")
		}
		entries = append(entries, atomEntry{ID: item.ID, Title: item.Title, Link: atomLink{Href: item.URL}, Updated: item.PublishedAt.UTC().Format(time.RFC3339), Summary: item.Summary})
	}
	document := atomDocument{XMLNS: "http://www.w3.org/2005/Atom", Title: feed.Title, ID: feed.Link, Updated: feed.UpdatedAt.UTC().Format(time.RFC3339), Entries: entries}
	body, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, "", err
	}
	body = append([]byte(xml.Header), body...)
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}
