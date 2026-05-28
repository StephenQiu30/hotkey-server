package memory

import (
	"sort"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/hotspot"
)

// HotspotRepo is an in-memory implementation of repo.HotspotRepo.
type HotspotRepo struct {
	mu       sync.Mutex
	hotspots map[string]hotspot.HotspotDetail
}

func NewHotspotRepo() *HotspotRepo {
	return &HotspotRepo{
		hotspots: make(map[string]hotspot.HotspotDetail),
	}
}

func cloneKeywords(kw []string) []string {
	return append([]string(nil), kw...)
}

func cloneRelated(items []hotspot.RelatedContent) []hotspot.RelatedContent {
	return append([]hotspot.RelatedContent(nil), items...)
}

func cloneEvidence(ev hotspot.EvidenceDetail) hotspot.EvidenceDetail {
	return hotspot.EvidenceDetail{
		FactEvidenceIDs:   append([]string(nil), ev.FactEvidenceIDs...),
		SignalEvidenceIDs: append([]string(nil), ev.SignalEvidenceIDs...),
		RiskLabels:        append([]string(nil), ev.RiskLabels...),
	}
}

func cloneDetail(d hotspot.HotspotDetail) hotspot.HotspotDetail {
	d.Keywords = cloneKeywords(d.Keywords)
	d.RelatedContent = cloneRelated(d.RelatedContent)
	d.Evidence = cloneEvidence(d.Evidence)
	return d
}

func (r *HotspotRepo) UpsertHotspot(detail hotspot.HotspotDetail) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hotspots[detail.ID] = detail
	return nil
}

func (r *HotspotRepo) ListHotspots() ([]hotspot.HotspotDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	hotspots := make([]hotspot.HotspotDetail, 0, len(r.hotspots))
	for _, h := range r.hotspots {
		hotspots = append(hotspots, cloneDetail(h))
	}
	sort.Slice(hotspots, func(i, j int) bool {
		return hotspots[i].ID < hotspots[j].ID
	})
	return hotspots, nil
}

func (r *HotspotRepo) GetHotspot(id string) (hotspot.HotspotDetail, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	detail, ok := r.hotspots[id]
	if !ok {
		return hotspot.HotspotDetail{}, hotspot.ErrHotspotNotFound
	}
	return cloneDetail(detail), nil
}
