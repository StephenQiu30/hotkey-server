package content

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	ResultCreated   = "created"
	ResultDuplicate = "duplicate"

	titleDuplicateWindow = 24 * time.Hour
)

var ErrInvalidSourceItem = errors.New("invalid source item")

type SourceItem struct {
	ID           string            `json:"id"`
	SourceID     string            `json:"sourceId"`
	OriginalURL  string            `json:"originalUrl"`
	CanonicalURL string            `json:"canonicalUrl"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	PublishedAt  time.Time         `json:"publishedAt"`
	FetchedAt    time.Time         `json:"fetchedAt"`
	ContentHash  string            `json:"contentHash"`
	RawMetadata  map[string]string `json:"rawMetadata"`
}

type IngestSourceItemInput struct {
	SourceID    string
	OriginalURL string
	Title       string
	Summary     string
	PublishedAt time.Time
	FetchedAt   time.Time
	RawMetadata map[string]string
}

type Service struct {
	mu         sync.Mutex
	nextNumber int
	items      map[string]SourceItem
	byURL      map[string]string
	byHash     map[string]string
}

func NewService() *Service {
	return &Service{
		nextNumber: 1,
		items:      make(map[string]SourceItem),
		byURL:      make(map[string]string),
		byHash:     make(map[string]string),
	}
}

func (s *Service) IngestSourceItem(input IngestSourceItemInput) (SourceItem, string, error) {
	item, normalizedTitle, err := normalizeItem(input)
	if err != nil {
		return SourceItem{}, "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.byURL[item.CanonicalURL]; ok {
		return cloneItem(s.items[id]), ResultDuplicate, nil
	}
	if id, ok := s.byHash[item.ContentHash]; ok {
		return cloneItem(s.items[id]), ResultDuplicate, nil
	}
	if duplicate, ok := s.findTitleWindowDuplicate(normalizedTitle, item.PublishedAt); ok {
		return cloneItem(duplicate), ResultDuplicate, nil
	}

	item.ID = fmt.Sprintf("item_%d", s.nextNumber)
	s.nextNumber++
	s.items[item.ID] = item
	s.byURL[item.CanonicalURL] = item.ID
	s.byHash[item.ContentHash] = item.ID
	return cloneItem(item), ResultCreated, nil
}

func (s *Service) ListSourceItems() []SourceItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]SourceItem, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, cloneItem(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items
}

func (s *Service) findTitleWindowDuplicate(normalizedTitle string, publishedAt time.Time) (SourceItem, bool) {
	for _, item := range s.items {
		if normalizeText(item.Title) != normalizedTitle {
			continue
		}
		delta := item.PublishedAt.Sub(publishedAt)
		if delta < 0 {
			delta = -delta
		}
		if delta <= titleDuplicateWindow {
			return item, true
		}
	}
	return SourceItem{}, false
}

func normalizeItem(input IngestSourceItemInput) (SourceItem, string, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	originalURL := strings.TrimSpace(input.OriginalURL)
	title := collapseSpaces(strings.TrimSpace(input.Title))
	summary := collapseSpaces(strings.TrimSpace(input.Summary))
	if sourceID == "" || originalURL == "" || title == "" || input.PublishedAt.IsZero() || input.FetchedAt.IsZero() {
		return SourceItem{}, "", ErrInvalidSourceItem
	}

	canonicalURL, err := canonicalizeURL(originalURL)
	if err != nil {
		return SourceItem{}, "", ErrInvalidSourceItem
	}
	normalizedTitle := normalizeText(title)
	item := SourceItem{
		SourceID:     sourceID,
		OriginalURL:  originalURL,
		CanonicalURL: canonicalURL,
		Title:        title,
		Summary:      summary,
		PublishedAt:  input.PublishedAt,
		FetchedAt:    input.FetchedAt,
		ContentHash:  hashContent(normalizedTitle, normalizeText(summary)),
		RawMetadata:  cloneMetadata(input.RawMetadata),
	}
	return item, normalizedTitle, nil
}

func canonicalizeURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", err
	}
	query := parsed.Query()
	for key := range query {
		if strings.HasPrefix(strings.ToLower(key), "utm_") {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func hashContent(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func normalizeText(value string) string {
	return strings.ToLower(collapseSpaces(strings.TrimSpace(value)))
}

func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func cloneItem(item SourceItem) SourceItem {
	item.RawMetadata = cloneMetadata(item.RawMetadata)
	return item
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
