package repo

import "github.com/StephenQiu30/hotkey-server/internal/content"

// ContentRepo defines the storage interface for source content items.
type ContentRepo interface {
	CreateItem(item content.SourceItem) error
	ListItems() ([]content.SourceItem, error)
	GetItem(id string) (content.SourceItem, error)
	GetByCanonicalURL(url string) (content.SourceItem, bool, error)
	GetByContentHash(hash string) (content.SourceItem, bool, error)
	ListByTitleWindow(normalizedTitle string) ([]content.SourceItem, error)
}
