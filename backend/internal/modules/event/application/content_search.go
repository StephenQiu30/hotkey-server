package application

import "context"

// ContentSearchReference is the safe, read-only Event projection exposed to
// Content search. It deliberately omits member evidence and governance state.
type ContentSearchReference struct {
	ContentID  int64
	EventID    int64
	EventTitle string
}

// ContentSearchReader lets another application enrich a bounded Content page
// without reading Event-owned tables directly.
type ContentSearchReader interface {
	ListContentSearchReferences(context.Context, []int64) ([]ContentSearchReference, error)
}
