package content

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

type ItemStatus string

const (
	ItemStatusPrimary   ItemStatus = "primary"
	ItemStatusDuplicate ItemStatus = "duplicate"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

type SourceItem struct {
	ID                string
	SourceID          string
	Title             string
	Snippet           string
	RawURL            string
	CanonicalURL      string
	PublishedAt       *time.Time
	ContentHash       string
	Language          string
	Status            ItemStatus
	DuplicateOfItemID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type HashInput struct {
	Title        string
	Snippet      string
	CanonicalURL string
}

type Repository interface {
	FindByCanonicalURL(context.Context, string) (SourceItem, error)
	FindByContentHash(context.Context, string) (SourceItem, error)
	Create(context.Context, SourceItem) (SourceItem, error)
}

func NewID() string {
	var body [16]byte
	if _, err := rand.Read(body[:]); err != nil {
		return "item-" + hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return "item-" + hex.EncodeToString(body[:])
}

func CanonicalURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", ErrInvalidInput
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidInput
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidInput
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", ErrInvalidInput
	}
	if port := parsed.Port(); port != "" && !isDefaultPort(parsed.Scheme, port) {
		host = net.JoinHostPort(host, port)
	}
	parsed.Host = host
	parsed.Fragment = ""
	parsed.User = nil

	cleanPath := path.Clean("/" + parsed.EscapedPath())
	if cleanPath == "/" {
		parsed.Path = ""
	} else {
		parsed.Path = cleanPath
	}

	query := parsed.Query()
	for key := range query {
		if isTrackingParam(key) {
			delete(query, key)
		}
	}
	parsed.RawQuery = sortedQuery(query)
	return parsed.String(), nil
}

func ContentHash(input HashInput) string {
	parts := []string{
		normalizeText(input.Title),
		normalizeText(input.Snippet),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func isDefaultPort(scheme string, port string) bool {
	return (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
}

func isTrackingParam(key string) bool {
	key = strings.ToLower(key)
	if strings.HasPrefix(key, "utm_") {
		return true
	}
	switch key {
	case "fbclid", "gclid", "dclid", "yclid", "mc_cid", "mc_eid", "igshid", "ref", "ref_src":
		return true
	default:
		return false
	}
}

func sortedQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(url.Values, len(values))
	for _, key := range keys {
		copied := append([]string(nil), values[key]...)
		sort.Strings(copied)
		ordered[key] = copied
	}
	return ordered.Encode()
}

type MemoryRepository struct {
	mu          sync.RWMutex
	items       map[string]SourceItem
	order       []string
	byCanonical map[string]string
	byHash      map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		items:       make(map[string]SourceItem),
		byCanonical: make(map[string]string),
		byHash:      make(map[string]string),
	}
}

func (r *MemoryRepository) FindByCanonicalURL(_ context.Context, canonicalURL string) (SourceItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, exists := r.byCanonical[canonicalURL]
	if !exists {
		return SourceItem{}, ErrNotFound
	}
	return cloneItem(r.items[id]), nil
}

func (r *MemoryRepository) FindByContentHash(_ context.Context, contentHash string) (SourceItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, exists := r.byHash[contentHash]
	if !exists {
		return SourceItem{}, ErrNotFound
	}
	return cloneItem(r.items[id]), nil
}

func (r *MemoryRepository) Create(_ context.Context, item SourceItem) (SourceItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[item.ID]; exists {
		return SourceItem{}, ErrAlreadyExists
	}
	if _, exists := r.byCanonical[item.CanonicalURL]; exists {
		return SourceItem{}, ErrAlreadyExists
	}
	r.items[item.ID] = cloneItem(item)
	r.order = append(r.order, item.ID)
	r.byCanonical[item.CanonicalURL] = item.ID
	if _, exists := r.byHash[item.ContentHash]; !exists {
		r.byHash[item.ContentHash] = item.ID
	}
	return cloneItem(item), nil
}

func (r *MemoryRepository) List(_ context.Context) ([]SourceItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]SourceItem, 0, len(r.order))
	for _, id := range r.order {
		items = append(items, cloneItem(r.items[id]))
	}
	return items, nil
}

func cloneItem(item SourceItem) SourceItem {
	if item.PublishedAt != nil {
		publishedAt := *item.PublishedAt
		item.PublishedAt = &publishedAt
	}
	return item
}
