package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// RelevanceProfileStatus controls whether an automatic document match is
// merely recorded, shadowed, allowed to decide, or retained only as history.
// It never represents factual truth or source credibility.
type RelevanceProfileStatus string

const (
	RelevanceProfileUncalibrated RelevanceProfileStatus = "uncalibrated"
	RelevanceProfileShadow       RelevanceProfileStatus = "shadow"
	RelevanceProfileActive       RelevanceProfileStatus = "active"
	RelevanceProfileRolledBack   RelevanceProfileStatus = "rolled_back"
)

func (status RelevanceProfileStatus) Valid() bool {
	return status == RelevanceProfileUncalibrated || status == RelevanceProfileShadow ||
		status == RelevanceProfileActive || status == RelevanceProfileRolledBack
}

// RelevanceDecisionProfile is an immutable calibration/threshold entity. A
// probability produced in this profile means only "relevant to the monitor".
type RelevanceDecisionProfile struct {
	ID, Version, EvaluationRunID              int64
	MatchingAlgorithmVersion, RerankerVersion string
	CalibrationVersion                        string
	Status                                    RelevanceProfileStatus
	RejectThreshold, AcceptThreshold          float64
	CalibrationSlope, CalibrationIntercept    float64
}

func (profile RelevanceDecisionProfile) Validate() error {
	if profile.ID <= 0 || profile.Version != 1 || !profile.Status.Valid() ||
		!validSemanticVersion(profile.MatchingAlgorithmVersion) || !validSemanticVersion(profile.RerankerVersion) ||
		!validSemanticVersion(profile.CalibrationVersion) || !validProbability(profile.RejectThreshold) ||
		!validProbability(profile.AcceptThreshold) || profile.RejectThreshold >= profile.AcceptThreshold ||
		math.IsNaN(profile.CalibrationSlope) || math.IsInf(profile.CalibrationSlope, 0) ||
		math.IsNaN(profile.CalibrationIntercept) || math.IsInf(profile.CalibrationIntercept, 0) {
		return fmt.Errorf("invalid relevance decision profile")
	}
	if profile.Status == RelevanceProfileUncalibrated && !strings.HasPrefix(profile.CalibrationVersion, "uncalibrated") {
		return fmt.Errorf("uncalibrated profile requires an explicit uncalibrated version")
	}
	if profile.Status == RelevanceProfileActive && strings.HasPrefix(profile.CalibrationVersion, "uncalibrated") {
		return fmt.Errorf("active relevance profile must be calibrated")
	}
	if (profile.Status == RelevanceProfileActive || profile.Status == RelevanceProfileRolledBack) && profile.EvaluationRunID <= 0 {
		return fmt.Errorf("active relevance profile requires an evaluation run")
	}
	if (profile.Status == RelevanceProfileActive || profile.Status == RelevanceProfileRolledBack || profile.Status == RelevanceProfileShadow) &&
		(profile.CalibrationSlope <= 0 || profile.CalibrationSlope > 100 || math.Abs(profile.CalibrationIntercept) > 100) {
		return fmt.Errorf("calibrated relevance profile requires finite Platt coefficients")
	}
	return nil
}

// DecideDocumentMatch is deliberately conservative: uncalibrated, shadow,
// rolled-back, degraded, or scoreless candidates remain review-only. A hard
// MUST_NOT conflict is the sole scoreless automatic rejection.
func DecideDocumentMatch(profile RelevanceDecisionProfile, probability *float64, degraded, hardVeto bool) (MatchDecision, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}
	if probability != nil && !validProbability(*probability) {
		return "", fmt.Errorf("invalid relevance probability")
	}
	if hardVeto {
		return MatchDecisionRejected, nil
	}
	if profile.Status != RelevanceProfileActive || degraded || probability == nil {
		return MatchDecisionReview, nil
	}
	switch {
	case *probability >= profile.AcceptThreshold:
		return MatchDecisionAccepted, nil
	case *probability < profile.RejectThreshold:
		return MatchDecisionRejected, nil
	default:
		return MatchDecisionReview, nil
	}
}

// DocumentMatchDecision is one immutable automatic decision for an exact
// MonitorVersion and DocumentVersion. Manual review is stored separately.
type DocumentMatchDecision struct {
	ID, MonitorID, MonitorVersionID, CompiledProfileID int64
	DocumentVersionID, RelevanceProfileID              int64
	MatchingAlgorithmVersion, RerankerVersion          string
	CalibrationVersion, InputHash                      string
	RRFScore                                           float64
	RelevanceProbability                               *float64
	Decision                                           MatchDecision
	Degraded                                           bool
	ReasonCodes                                        []string
	CreatedAt                                          time.Time
}

func (decision DocumentMatchDecision) Validate() error {
	if decision.ID <= 0 || decision.MonitorID <= 0 || decision.MonitorVersionID <= 0 || decision.CompiledProfileID <= 0 ||
		decision.DocumentVersionID <= 0 || decision.RelevanceProfileID <= 0 || !decision.Decision.Valid() ||
		!validSemanticVersion(decision.MatchingAlgorithmVersion) || !validSemanticVersion(decision.RerankerVersion) ||
		!validSemanticVersion(decision.CalibrationVersion) || !validSHA256(decision.InputHash) ||
		math.IsNaN(decision.RRFScore) || math.IsInf(decision.RRFScore, 0) || decision.RRFScore < 0 ||
		!validReasonCodes(decision.ReasonCodes, 32) {
		return fmt.Errorf("invalid document match decision")
	}
	if decision.RelevanceProbability != nil && !validProbability(*decision.RelevanceProbability) {
		return fmt.Errorf("invalid document match probability")
	}
	if decision.Decision == MatchDecisionAccepted && (decision.RelevanceProbability == nil || decision.Degraded) {
		return fmt.Errorf("accepted document match requires a non-degraded probability")
	}
	return nil
}

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func validSemanticVersion(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len([]byte(value)) > 64 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '.' || character == '_' || character == ':' || character == '-') {
			continue
		}
		return false
	}
	return true
}
