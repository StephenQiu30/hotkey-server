package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
)

const MaxCandidates = 50

type ScoreBreakdown struct {
	EntityAction  float64 `json:"entity_action"`
	Semantic      float64 `json:"semantic"`
	Temporal      float64 `json:"temporal"`
	Location      float64 `json:"location"`
	SourceContext float64 `json:"source_context"`
}

func (scores ScoreBreakdown) Validate() error {
	values := []float64{scores.EntityAction, scores.Semantic, scores.Temporal, scores.Location, scores.SourceContext}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return fmt.Errorf("score must be finite and between 0 and 100")
		}
	}
	return nil
}

func (scores ScoreBreakdown) MembershipScore() float64 {
	return scores.EntityAction*0.30 + scores.Semantic*0.30 + scores.Temporal*0.20 + scores.Location*0.10 + scores.SourceContext*0.10
}

type MembershipDecision string

const (
	DecisionAccept   MembershipDecision = "accepted"
	DecisionReview   MembershipDecision = "review"
	DecisionReject   MembershipDecision = "rejected"
	DecisionNewEvent MembershipDecision = "new_event"
)

func (decision MembershipDecision) Valid() bool {
	return decision == DecisionAccept || decision == DecisionReview || decision == DecisionReject || decision == DecisionNewEvent
}

type DecisionOrigin string

const (
	DecisionOriginRule  DecisionOrigin = "rule"
	DecisionOriginModel DecisionOrigin = "model"
	DecisionOriginUser  DecisionOrigin = "user"
)

func (origin DecisionOrigin) Valid() bool {
	return origin == DecisionOriginRule || origin == DecisionOriginModel || origin == DecisionOriginUser
}

type Candidate struct {
	EventID      int64
	EventKey     string
	Channel      CandidateChannel
	Score        float64
	HardConflict bool
}

func (candidate Candidate) Validate() error {
	if candidate.EventID <= 0 || strings.TrimSpace(candidate.EventKey) == "" || !candidate.Channel.Valid() || candidate.Score < 0 || candidate.Score > 100 {
		return fmt.Errorf("invalid event candidate")
	}
	return nil
}

type Decision struct {
	ContentID          int64
	CandidateEventID   *int64
	CandidateEventKey  string
	ClusteringVersion  string
	FeatureInputHash   string
	Channel            CandidateChannel
	CandidateRank      int
	Scores             ScoreBreakdown
	MembershipScore    float64
	Decision           MembershipDecision
	DecisionOrigin     DecisionOrigin
	ReasonCodes        []string
	EvidenceContentIDs []int64
	ActorUserID        *int64
}

func (decision Decision) Validate() error {
	if decision.ContentID <= 0 || strings.TrimSpace(decision.ClusteringVersion) == "" || len(decision.ClusteringVersion) > 64 || !validSHA256(decision.FeatureInputHash) || !decision.Channel.Valid() || decision.CandidateRank < 0 || !decision.Decision.Valid() || !decision.DecisionOrigin.Valid() || decision.MembershipScore < 0 || decision.MembershipScore > 100 {
		return fmt.Errorf("invalid clustering decision")
	}
	if decision.Decision == DecisionNewEvent {
		if decision.CandidateEventID != nil || decision.CandidateEventKey != "__new_event__" {
			return fmt.Errorf("new event decision must use __new_event__")
		}
	} else if decision.CandidateEventID == nil || *decision.CandidateEventID <= 0 || strings.TrimSpace(decision.CandidateEventKey) == "" {
		return fmt.Errorf("candidate event is required")
	}
	if err := decision.Scores.Validate(); err != nil {
		return err
	}
	if math.Abs(decision.MembershipScore-decision.Scores.MembershipScore()) > 0.01 && decision.Decision != DecisionNewEvent {
		return fmt.Errorf("membership score does not match score breakdown")
	}
	return nil
}

func (decision Decision) IdempotencyKey() string {
	candidate := decision.CandidateEventKey
	return strings.Join([]string{fmt.Sprint(decision.ContentID), decision.ClusteringVersion, decision.FeatureInputHash, candidate}, ":")
}

func CompareCandidates(candidates []Candidate) []Candidate {
	result := append([]Candidate(nil), candidates...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].EventKey < result[j].EventKey
	})
	if len(result) > MaxCandidates {
		result = result[:MaxCandidates]
	}
	return result
}

func Decide(scores ScoreBreakdown, hardConflict bool) (MembershipDecision, float64, error) {
	if err := scores.Validate(); err != nil {
		return "", 0, err
	}
	value := scores.MembershipScore()
	if hardConflict {
		return DecisionReject, value, nil
	}
	switch {
	case value >= 80:
		return DecisionAccept, value, nil
	case value >= 65:
		return DecisionReview, value, nil
	default:
		return DecisionNewEvent, value, nil
	}
}

func FeatureInputHash(parts ...string) string {
	joined := strings.Join(parts, "\x00")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
