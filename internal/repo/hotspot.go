package repo

import "github.com/StephenQiu30/hotkey-server/internal/hotspot"

// HotspotRepo defines the storage interface for hotspots.
type HotspotRepo interface {
	UpsertHotspot(detail hotspot.HotspotDetail) error
	ListHotspots() ([]hotspot.HotspotDetail, error)
	GetHotspot(id string) (hotspot.HotspotDetail, error)
}
