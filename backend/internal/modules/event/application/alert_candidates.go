package application

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// AlertUpdateRef is the bounded identity envelope consumed by Event-owned
// alert candidate reads. No Event body or queue payload crosses this port.
type AlertUpdateRef struct {
	ID              int64
	Version         int64
	EvidenceSetHash string
}

func (ref AlertUpdateRef) Validate() error {
	if ref.ID <= 0 || ref.Version <= 0 || len(ref.EvidenceSetHash) != 64 || ref.EvidenceSetHash != strings.ToLower(ref.EvidenceSetHash) {
		return fmt.Errorf("%w: invalid event update reference", sharedrepository.ErrInvalidInput)
	}
	if _, err := hex.DecodeString(ref.EvidenceSetHash); err != nil {
		return fmt.Errorf("%w: invalid event update hash", sharedrepository.ErrInvalidInput)
	}
	return nil
}

// AlertCandidate contains only the Event-owned update and visible Watch match
// facts required by Alert eligibility.
type AlertCandidate struct {
	MonitorID      int64
	EventID        int64
	UpdateKind     string
	FinalScore     float64
	TitleSnapshot  string
	ReasonSnapshot string
	ReasonCodes    []string
	TriggeredAt    time.Time
}

type AlertCandidateReader interface {
	ListAlertCandidates(context.Context, AlertUpdateRef) ([]AlertCandidate, error)
}
