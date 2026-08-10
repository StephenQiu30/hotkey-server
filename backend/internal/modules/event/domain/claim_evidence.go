package domain

import (
	"errors"
	"sort"
	"strings"
)

type ClaimEvidenceRelation string

const (
	EvidenceAsserts      ClaimEvidenceRelation = "asserts"
	EvidenceAttributes   ClaimEvidenceRelation = "attributes_to"
	EvidenceMentions     ClaimEvidenceRelation = "mentions"
	EvidenceContradicts  ClaimEvidenceRelation = "contradicts"
	EvidenceCorrects     ClaimEvidenceRelation = "corrects"
	EvidenceWithdraws    ClaimEvidenceRelation = "withdraws"
	EvidenceRelationNone ClaimEvidenceRelation = "unknown"
)

func (relation ClaimEvidenceRelation) Valid() bool {
	switch relation {
	case EvidenceAsserts, EvidenceAttributes, EvidenceMentions, EvidenceContradicts,
		EvidenceCorrects, EvidenceWithdraws, EvidenceRelationNone:
		return true
	default:
		return false
	}
}

type EvidenceState string

const (
	EvidenceNoCitableBody      EvidenceState = "no_citable_body"
	EvidenceSingleOrigin       EvidenceState = "single_origin"
	EvidenceMultipleOrigins    EvidenceState = "multiple_origins"
	EvidenceConflictingReports EvidenceState = "conflicting_reports"
	EvidencePublisherCorrected EvidenceState = "publisher_corrected"
	EvidencePublisherWithdrawn EvidenceState = "publisher_withdrawn"
)

func (state EvidenceState) Valid() bool {
	switch state {
	case EvidenceNoCitableBody, EvidenceSingleOrigin, EvidenceMultipleOrigins,
		EvidenceConflictingReports, EvidencePublisherCorrected, EvidencePublisherWithdrawn:
		return true
	default:
		return false
	}
}

type EvidenceStateItem struct {
	ClaimEvidenceVersionID int64
	LineageRootID          int64
	Relation               ClaimEvidenceRelation
	Citable                bool
}

type EvidenceStateInput struct {
	AlgorithmVersion string
	Items            []EvidenceStateItem
}

type EvidenceStateResult struct {
	State                  EvidenceState
	IndependentOriginCount int
	ReasonCodes            []string
}

func CalculateEvidenceState(input EvidenceStateInput) (EvidenceStateResult, error) {
	if strings.TrimSpace(input.AlgorithmVersion) == "" || len(input.AlgorithmVersion) > 64 {
		return EvidenceStateResult{}, errors.New("invalid evidence state profile")
	}
	origins := map[int64]struct{}{}
	hasContradiction, hasCorrection, hasWithdrawal := false, false, false
	for _, item := range input.Items {
		if item.ClaimEvidenceVersionID <= 0 || item.LineageRootID <= 0 || !item.Relation.Valid() {
			return EvidenceStateResult{}, errors.New("invalid evidence state item")
		}
		if !item.Citable {
			continue
		}
		origins[item.LineageRootID] = struct{}{}
		switch item.Relation {
		case EvidenceContradicts:
			hasContradiction = true
		case EvidenceCorrects:
			hasCorrection = true
		case EvidenceWithdraws:
			hasWithdrawal = true
		}
	}
	result := EvidenceStateResult{IndependentOriginCount: len(origins)}
	switch {
	case len(origins) == 0:
		result.State, result.ReasonCodes = EvidenceNoCitableBody, []string{"no_current_citable_evidence"}
	case hasWithdrawal:
		result.State, result.ReasonCodes = EvidencePublisherWithdrawn, []string{"publisher_withdrawal_reported"}
	case hasCorrection:
		result.State, result.ReasonCodes = EvidencePublisherCorrected, []string{"publisher_correction_reported"}
	case hasContradiction:
		result.State, result.ReasonCodes = EvidenceConflictingReports, []string{"contradicting_lineage_report"}
	case len(origins) >= 2:
		result.State, result.ReasonCodes = EvidenceMultipleOrigins, []string{"multiple_independent_lineage_roots"}
	default:
		result.State, result.ReasonCodes = EvidenceSingleOrigin, []string{"single_independent_lineage_root"}
	}
	sort.Strings(result.ReasonCodes)
	return result, nil
}
