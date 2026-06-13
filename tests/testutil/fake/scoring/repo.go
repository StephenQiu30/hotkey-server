package fakescoring

import (
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
)

// HitRepo is a fake implementing scoring.HitRepository.
type HitRepo struct {
	Saved []scoring.SavedScore
}

func (r *HitRepo) UpdateScores(_ int64, s scoring.SavedScore) error {
	r.Saved = append(r.Saved, s)
	return nil
}
