package memory

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/content"
)

const titleDuplicateWindow = 24 * time.Hour

// ContentRepo is an in-memory implementation of repo.ContentRepo.
type ContentRepo struct {
	mu    sync.Mutex
	items map[string]content.SourceItem
}

func NewContentRepo() *ContentRepo {
	return &ContentRepo{
		items: make(map[string]content.SourceItem),
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

func cloneItem(item content.SourceItem) content.SourceItem {
	item.RawMetadata = cloneMetadata(item.RawMetadata)
	return item
}

func (r *ContentRepo) CreateItem(item content.SourceItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
	return nil
}

func (r *ContentRepo) ListItems() ([]content.SourceItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]content.SourceItem, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, cloneItem(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (r *ContentRepo) GetItem(id string) (content.SourceItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return content.SourceItem{}, content.ErrInvalidSourceItem
	}
	return cloneItem(item), nil
}

func (r *ContentRepo) GetByCanonicalURL(url string) (content.SourceItem, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.items {
		if item.CanonicalURL == url {
			return cloneItem(item), true, nil
		}
	}
	return content.SourceItem{}, false, nil
}

func (r *ContentRepo) GetByContentHash(hash string) (content.SourceItem, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.items {
		if item.ContentHash == hash {
			return cloneItem(item), true, nil
		}
	}
	return content.SourceItem{}, false, nil
}

func (r *ContentRepo) ListByTitleWindow(normalizedTitle string) ([]content.SourceItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []content.SourceItem
	for _, item := range r.items {
		if normalizeText(item.Title) == normalizedTitle {
			items = append(items, cloneItem(item))
		}
	}
	return items, nil
}

func normalizeText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}
