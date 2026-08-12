package application

import "context"

// ContentSearchReference is the safe, read-only MicroEvent projection exposed
// to Content search. It deliberately omits member evidence and governance
// state, and never falls back to the quarantined legacy Event identity.
type ContentSearchReference struct {
	ContentID       int64
	MicroEventID    int64
	MicroEventTitle string
}

// ContentSearchReader lets another application enrich a bounded Content page
// without reading Event-owned tables directly.
type ContentSearchReader interface {
	ListContentSearchReferences(context.Context, []int64) ([]ContentSearchReference, error)
}
