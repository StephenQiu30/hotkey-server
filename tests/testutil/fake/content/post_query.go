package fakecontent

import (
	"github.com/StephenQiu30/hotkey-server/internal/content"
)

// PostQueryService is a fake implementing content.PostQueryService.
type PostQueryService struct {
	Posts []content.PostSummary
	Err   error
}

func (s *PostQueryService) ListPostsByMonitor(monitorID int64, limit, offset int) ([]content.PostSummary, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	var out []content.PostSummary
	for _, p := range s.Posts {
		// PostSummary doesn't have MonitorID; caller filters externally.
		// For simplicity we return the stored slice (tests pre-filter).
		out = append(out, p)
	}
	if offset >= len(out) {
		return nil, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}
