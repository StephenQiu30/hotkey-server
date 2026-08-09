package application

import (
	"context"
	"fmt"
	"time"
)

// One collection fetch is capped at 100 items, while multi-request connectors
// can additionally retain an index/list response. Keep bounded headroom for
// those response graphs without accepting an unbounded policy query.
const maximumRawEvidenceRightsSubjects = 256

// RawEvidenceRightsSubjectDTO is the exact raw-response identity evaluated by
// the rights resolver. It contains no payload bytes or persistence details.
type RawEvidenceRightsSubjectDTO struct {
	EvidenceKey   string
	PayloadSHA256 string
}

// CurrentRawEvidenceRightsQuery requests a single consistent read of current
// store_raw and retain decisions for a bounded fetch batch.
type CurrentRawEvidenceRightsQuery struct {
	SourceConnectionID int64
	DecisionAt         time.Time
	Subjects           []RawEvidenceRightsSubjectDTO
}

func (query CurrentRawEvidenceRightsQuery) Validate() error {
	if query.SourceConnectionID <= 0 || query.DecisionAt.IsZero() {
		return fmt.Errorf("raw evidence rights source and decision time are required")
	}
	if len(query.Subjects) < 1 || len(query.Subjects) > maximumRawEvidenceRightsSubjects {
		return fmt.Errorf("raw evidence rights subject count must be from 1 to %d", maximumRawEvidenceRightsSubjects)
	}
	seen := make(map[string]struct{}, len(query.Subjects))
	for _, subject := range query.Subjects {
		if !validSHA256Hex(subject.EvidenceKey) || !validSHA256Hex(subject.PayloadSHA256) {
			return fmt.Errorf("raw evidence rights subject identity is invalid")
		}
		identity := subject.EvidenceKey + ":" + subject.PayloadSHA256
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("raw evidence rights subjects must be unique")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

// CurrentRawEvidenceRightsResult contains only explicit current allows.
// Missing entries represent deny, unknown, conflict, expiry, or no policy and
// must be treated identically by callers.
type CurrentRawEvidenceRightsResult struct {
	StoreRawDecisions map[string]RawEvidenceRightsDecisionDTO
	RetainDecisions   map[string]RawEvidenceRightsDecisionDTO
}

// CurrentRawEvidenceRightsReader resolves effective decisions without
// creating policy facts. Policy authoring remains a separate use case.
type CurrentRawEvidenceRightsReader interface {
	ResolveCurrent(context.Context, CurrentRawEvidenceRightsQuery) (CurrentRawEvidenceRightsResult, error)
}
