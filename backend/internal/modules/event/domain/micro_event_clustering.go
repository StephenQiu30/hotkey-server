package domain

import (
	"errors"
	"math"
	"sort"
	"strings"
)

type MicroEventAction string

const (
	MicroEventActionCreate MicroEventAction = "create"
	MicroEventActionJoin   MicroEventAction = "join"
	MicroEventActionReview MicroEventAction = "review"
)

type MicroEventFeatures struct {
	SparseSimilarity      float64
	DenseSimilarity       float64
	EntityOverlap         float64
	ActionOverlap         float64
	LocationConsistency   float64
	IdentifierConsistency float64
	TimeSimilarity        float64
	LineageRelation       float64
}

func (features MicroEventFeatures) Validate() error {
	for _, value := range []float64{features.SparseSimilarity, features.DenseSimilarity, features.EntityOverlap,
		features.ActionOverlap, features.LocationConsistency, features.IdentifierConsistency,
		features.TimeSimilarity, features.LineageRelation} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return errors.New("micro-event feature is outside [0,1]")
		}
	}
	return nil
}

func (features MicroEventFeatures) sameEventScore() float64 {
	return features.SparseSimilarity*.18 + features.DenseSimilarity*.20 + features.EntityOverlap*.15 +
		features.ActionOverlap*.15 + features.LocationConsistency*.10 + features.IdentifierConsistency*.10 +
		features.TimeSimilarity*.07 + features.LineageRelation*.05
}

type MicroEventCandidate struct {
	MicroEventID        int64
	EventVersion        int64
	Features            MicroEventFeatures
	DenseAvailable      bool
	HardConflict        bool
	HardConflictReasons []string
}

type MicroEventDecisionInput struct {
	ContentFamilyID int64
	Candidates      []MicroEventCandidate
	ProfileVersion  string
}

type MicroEventDecision struct {
	Action         MicroEventAction
	MicroEventID   int64
	EventVersion   int64
	SameEventScore float64
	LeadingMargin  float64
	Features       MicroEventFeatures
	ProfileVersion string
	ReasonCodes    []string
}

func DecideMicroEventMembership(input MicroEventDecisionInput) (MicroEventDecision, error) {
	if input.ContentFamilyID <= 0 || strings.TrimSpace(input.ProfileVersion) == "" || len(input.ProfileVersion) > 64 || len(input.Candidates) > 20 {
		return MicroEventDecision{}, errors.New("invalid micro-event decision input")
	}
	type scoredCandidate struct {
		candidate MicroEventCandidate
		score     float64
	}
	scored := make([]scoredCandidate, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.MicroEventID <= 0 || candidate.EventVersion <= 0 || candidate.Features.Validate() != nil {
			return MicroEventDecision{}, errors.New("invalid micro-event candidate")
		}
		if candidate.HardConflict {
			reasons := append([]string{"hard_conflict"}, candidate.HardConflictReasons...)
			return MicroEventDecision{Action: MicroEventActionCreate, ProfileVersion: input.ProfileVersion, ReasonCodes: reasons}, nil
		}
		scored = append(scored, scoredCandidate{candidate: candidate, score: candidate.Features.sameEventScore()})
	}
	if len(scored) == 0 {
		return MicroEventDecision{Action: MicroEventActionCreate, ProfileVersion: input.ProfileVersion, ReasonCodes: []string{"no_candidate"}}, nil
	}
	sort.SliceStable(scored, func(left, right int) bool {
		if scored[left].score != scored[right].score {
			return scored[left].score > scored[right].score
		}
		return scored[left].candidate.MicroEventID < scored[right].candidate.MicroEventID
	})
	best := scored[0]
	secondScore := 0.0
	if len(scored) > 1 {
		secondScore = scored[1].score
	}
	margin := best.score - secondScore
	result := MicroEventDecision{MicroEventID: best.candidate.MicroEventID, EventVersion: best.candidate.EventVersion,
		SameEventScore: best.score, LeadingMargin: margin, Features: best.candidate.Features, ProfileVersion: input.ProfileVersion}
	switch {
	// Feature facts are persisted as numeric(8,7) before clustering. Accept one
	// storage quantum at the documented boundary so an exact 0.90/0.15 decision
	// cannot flip to review after the database round-trip.
	case best.score >= .90-1e-7 && margin >= .15-1e-7:
		result.Action = MicroEventActionJoin
		result.ReasonCodes = []string{"same_event_high_margin"}
	case best.score >= .60:
		result.Action = MicroEventActionReview
		result.ReasonCodes = []string{"same_event_ambiguous"}
	default:
		result.Action = MicroEventActionCreate
		result.MicroEventID = 0
		result.EventVersion = 0
		result.ReasonCodes = []string{"below_same_event_threshold"}
	}
	if !best.candidate.DenseAvailable {
		result.ReasonCodes = append(result.ReasonCodes, "dense_unavailable")
	}
	return result, nil
}
