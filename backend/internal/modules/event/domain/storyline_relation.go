package domain

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

type StorylineCandidate struct {
	StorylineID       int64
	StorylineVersion  int64
	SubjectSimilarity float64
	ActionOverlap     float64
	TimeRecency       float64
	LatestEventAt     time.Time
}

type StorylineRelationInput struct {
	MicroEventID   int64
	MicroEventTime time.Time
	ProfileVersion string
	Candidates     []StorylineCandidate
}

type StorylineRelationDecision struct {
	CreateNew        bool
	StorylineID      int64
	StorylineVersion int64
	RelationType     string
	RelationScore    float64
	ReasonCodes      []string
}

func DecideStorylineRelation(input StorylineRelationInput) (StorylineRelationDecision, error) {
	if input.MicroEventID <= 0 || input.MicroEventTime.IsZero() || strings.TrimSpace(input.ProfileVersion) == "" ||
		len(input.ProfileVersion) > 64 || len(input.Candidates) > 20 {
		return StorylineRelationDecision{}, errors.New("invalid storyline relation input")
	}
	type scored struct {
		candidate StorylineCandidate
		score     float64
	}
	values := make([]scored, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.StorylineID <= 0 || candidate.StorylineVersion <= 0 || candidate.LatestEventAt.IsZero() ||
			!storylineUnitScore(candidate.SubjectSimilarity) || !storylineUnitScore(candidate.ActionOverlap) ||
			!storylineUnitScore(candidate.TimeRecency) {
			return StorylineRelationDecision{}, errors.New("invalid storyline candidate")
		}
		values = append(values, scored{candidate: candidate,
			score: candidate.SubjectSimilarity*.65 + candidate.ActionOverlap*.15 + candidate.TimeRecency*.20})
	}
	if len(values) == 0 {
		return StorylineRelationDecision{CreateNew: true, RelationType: "related", ReasonCodes: []string{"no_storyline_candidate"}}, nil
	}
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].score != values[right].score {
			return values[left].score > values[right].score
		}
		return values[left].candidate.StorylineID < values[right].candidate.StorylineID
	})
	best := values[0]
	if best.candidate.SubjectSimilarity < .60 || best.score < .55 {
		return StorylineRelationDecision{CreateNew: true, RelationType: "related", RelationScore: best.score,
			ReasonCodes: []string{"storyline_subject_below_threshold"}}, nil
	}
	relationType := "related"
	reason := "related_subject"
	if best.candidate.ActionOverlap >= .80 && !input.MicroEventTime.Before(best.candidate.LatestEventAt) {
		relationType = "updates"
		reason = "same_subject_action_update"
	} else if !input.MicroEventTime.Before(best.candidate.LatestEventAt) {
		relationType = "continues"
		reason = "same_subject_continuation"
	}
	return StorylineRelationDecision{StorylineID: best.candidate.StorylineID,
		StorylineVersion: best.candidate.StorylineVersion, RelationType: relationType,
		RelationScore: best.score, ReasonCodes: []string{reason}}, nil
}

func storylineUnitScore(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}
