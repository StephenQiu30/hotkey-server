package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidExpansionCandidate = errors.New("expansion candidate is invalid")
	ErrExpansionAlreadyReviewed  = errors.New("expansion candidate already has a decision")
)

type ExpansionSource string

const (
	ExpansionSourceUserInput                  ExpansionSource = "user_input"
	ExpansionSourceEntityAlias                ExpansionSource = "entity_alias"
	ExpansionSourceApprovedHistoricalFeedback ExpansionSource = "approved_historical_feedback"
	ExpansionSourceCorpusFeedback             ExpansionSource = "corpus_feedback"
	ExpansionSourceLLM                        ExpansionSource = "llm"
)

func (source ExpansionSource) Valid() bool {
	switch source {
	case ExpansionSourceUserInput, ExpansionSourceEntityAlias,
		ExpansionSourceApprovedHistoricalFeedback, ExpansionSourceCorpusFeedback,
		ExpansionSourceLLM:
		return true
	default:
		return false
	}
}

// Priority is intentionally fixed by the accepted product contract. Lower
// numbers have precedence when approved candidates normalize to the same term.
func (source ExpansionSource) Priority() int {
	switch source {
	case ExpansionSourceUserInput:
		return 0
	case ExpansionSourceEntityAlias:
		return 1
	case ExpansionSourceApprovedHistoricalFeedback:
		return 2
	case ExpansionSourceCorpusFeedback:
		return 3
	case ExpansionSourceLLM:
		return 4
	default:
		return 100
	}
}

type ExpansionRisk string

const (
	ExpansionRiskLow    ExpansionRisk = "low"
	ExpansionRiskMedium ExpansionRisk = "medium"
	ExpansionRiskHigh   ExpansionRisk = "high"
)

func (risk ExpansionRisk) Valid() bool {
	return risk == ExpansionRiskLow || risk == ExpansionRiskMedium || risk == ExpansionRiskHigh
}

type ExpansionApprovalStatus string

const (
	ExpansionApprovalPending  ExpansionApprovalStatus = "pending"
	ExpansionApprovalApproved ExpansionApprovalStatus = "approved"
	ExpansionApprovalRejected ExpansionApprovalStatus = "rejected"
)

func (status ExpansionApprovalStatus) Valid() bool {
	return status == ExpansionApprovalPending || status == ExpansionApprovalApproved || status == ExpansionApprovalRejected
}

type ExpansionDecision string

const (
	ExpansionDecisionApprove ExpansionDecision = "approved"
	ExpansionDecisionReject  ExpansionDecision = "rejected"
)

func (decision ExpansionDecision) Valid() bool {
	return decision == ExpansionDecisionApprove || decision == ExpansionDecisionReject
}

// ExpansionProvenance preserves why and how a candidate was produced. LLM
// provenance is fail-closed unless both model and prompt versions are known.
type ExpansionProvenance struct {
	source        ExpansionSource
	reason        string
	modelVersion  string
	promptVersion string
	inputHash     string
}

func NewExpansionProvenance(source ExpansionSource, reason, modelVersion, promptVersion, inputHash string) (ExpansionProvenance, error) {
	if !source.Valid() {
		return ExpansionProvenance{}, fmt.Errorf("%w: source is invalid", ErrInvalidExpansionCandidate)
	}
	normalizedReason, err := normalizeIntentValue(reason, 1000, "expansion reason")
	if err != nil {
		return ExpansionProvenance{}, fmt.Errorf("%w: %w", ErrInvalidExpansionCandidate, err)
	}
	modelVersion = normalizeText(modelVersion)
	promptVersion = normalizeText(promptVersion)
	if strings.ContainsAny(modelVersion+promptVersion, "\x00\r\n") || len([]byte(modelVersion)) > 128 || len([]byte(promptVersion)) > 128 {
		return ExpansionProvenance{}, fmt.Errorf("%w: model or prompt version is invalid", ErrInvalidExpansionCandidate)
	}
	if source == ExpansionSourceLLM && (modelVersion == "" || promptVersion == "") {
		return ExpansionProvenance{}, fmt.Errorf("%w: LLM provenance requires model and prompt versions", ErrInvalidExpansionCandidate)
	}
	if (modelVersion == "") != (promptVersion == "") {
		return ExpansionProvenance{}, fmt.Errorf("%w: model and prompt versions must be supplied together", ErrInvalidExpansionCandidate)
	}
	inputHash = strings.TrimSpace(inputHash)
	if !validIntentSHA256(inputHash) {
		return ExpansionProvenance{}, fmt.Errorf("%w: input hash is invalid", ErrInvalidExpansionCandidate)
	}
	return ExpansionProvenance{
		source: source, reason: normalizedReason, modelVersion: modelVersion,
		promptVersion: promptVersion, inputHash: inputHash,
	}, nil
}

func (provenance ExpansionProvenance) Source() ExpansionSource { return provenance.source }
func (provenance ExpansionProvenance) Reason() string          { return provenance.reason }
func (provenance ExpansionProvenance) ModelVersion() string    { return provenance.modelVersion }
func (provenance ExpansionProvenance) PromptVersion() string   { return provenance.promptVersion }
func (provenance ExpansionProvenance) InputHash() string       { return provenance.inputHash }

type ExpansionAssessment struct {
	similarity float64
	risk       ExpansionRisk
}

func NewExpansionAssessment(similarity float64, risk ExpansionRisk) (ExpansionAssessment, error) {
	if math.IsNaN(similarity) || math.IsInf(similarity, 0) || similarity < 0 || similarity > 1 || !risk.Valid() {
		return ExpansionAssessment{}, fmt.Errorf("%w: similarity or risk is invalid", ErrInvalidExpansionCandidate)
	}
	return ExpansionAssessment{similarity: similarity, risk: risk}, nil
}

func (assessment ExpansionAssessment) Similarity() float64 { return assessment.similarity }
func (assessment ExpansionAssessment) Risk() ExpansionRisk { return assessment.risk }

type ExpansionReview struct {
	decision       ExpansionDecision
	reviewerUserID int64
	reviewedAt     time.Time
	note           string
}

func NewExpansionReview(decision ExpansionDecision, reviewerUserID int64, reviewedAt time.Time, note string) (ExpansionReview, error) {
	if !decision.Valid() || reviewerUserID <= 0 || reviewedAt.IsZero() {
		return ExpansionReview{}, fmt.Errorf("%w: review identity is invalid", ErrInvalidExpansionCandidate)
	}
	note = normalizeText(note)
	if strings.ContainsRune(note, '\x00') || len([]byte(note)) > 2000 {
		return ExpansionReview{}, fmt.Errorf("%w: review note is invalid", ErrInvalidExpansionCandidate)
	}
	return ExpansionReview{decision: decision, reviewerUserID: reviewerUserID, reviewedAt: reviewedAt.UTC(), note: note}, nil
}

func (review ExpansionReview) Decision() ExpansionDecision { return review.decision }
func (review ExpansionReview) ReviewerUserID() int64       { return review.reviewerUserID }
func (review ExpansionReview) ReviewedAt() time.Time       { return review.reviewedAt }
func (review ExpansionReview) Note() string                { return review.note }

// ExpansionCandidate is immutable. Decisions create a new value so prior
// draft/version snapshots retain the exact pending fact that they observed.
type ExpansionCandidate struct {
	id         string
	value      string
	provenance ExpansionProvenance
	assessment ExpansionAssessment
	status     ExpansionApprovalStatus
	review     *ExpansionReview
}

func NewExpansionCandidate(id, value string, provenance ExpansionProvenance, assessment ExpansionAssessment) (ExpansionCandidate, error) {
	return RestoreExpansionCandidate(id, value, provenance, assessment, ExpansionApprovalPending, nil)
}

func RestoreExpansionCandidate(id, value string, provenance ExpansionProvenance, assessment ExpansionAssessment, status ExpansionApprovalStatus, review *ExpansionReview) (ExpansionCandidate, error) {
	normalizedID, err := normalizeIntentIdentifier(id, 128, "expansion candidate id")
	if err != nil {
		return ExpansionCandidate{}, fmt.Errorf("%w: %w", ErrInvalidExpansionCandidate, err)
	}
	normalizedValue, err := normalizeIntentValue(value, 160, "expansion candidate value")
	if err != nil {
		return ExpansionCandidate{}, fmt.Errorf("%w: %w", ErrInvalidExpansionCandidate, err)
	}
	if !provenance.source.Valid() || provenance.reason == "" || !validIntentSHA256(provenance.inputHash) ||
		!assessment.risk.Valid() || math.IsNaN(assessment.similarity) || math.IsInf(assessment.similarity, 0) || assessment.similarity < 0 || assessment.similarity > 1 || !status.Valid() {
		return ExpansionCandidate{}, fmt.Errorf("%w: candidate facts are invalid", ErrInvalidExpansionCandidate)
	}
	if status == ExpansionApprovalPending && review != nil {
		return ExpansionCandidate{}, fmt.Errorf("%w: pending candidate cannot have a review", ErrInvalidExpansionCandidate)
	}
	if status != ExpansionApprovalPending {
		if review == nil || (status == ExpansionApprovalApproved && review.decision != ExpansionDecisionApprove) ||
			(status == ExpansionApprovalRejected && review.decision != ExpansionDecisionReject) {
			return ExpansionCandidate{}, fmt.Errorf("%w: terminal candidate requires the matching review", ErrInvalidExpansionCandidate)
		}
	}
	var copiedReview *ExpansionReview
	if review != nil {
		copy := *review
		copiedReview = &copy
	}
	return ExpansionCandidate{
		id: normalizedID, value: normalizedValue, provenance: provenance,
		assessment: assessment, status: status, review: copiedReview,
	}, nil
}

func (candidate ExpansionCandidate) ID() string                              { return candidate.id }
func (candidate ExpansionCandidate) Value() string                           { return candidate.value }
func (candidate ExpansionCandidate) Provenance() ExpansionProvenance         { return candidate.provenance }
func (candidate ExpansionCandidate) Assessment() ExpansionAssessment         { return candidate.assessment }
func (candidate ExpansionCandidate) ApprovalStatus() ExpansionApprovalStatus { return candidate.status }
func (candidate ExpansionCandidate) Review() *ExpansionReview {
	if candidate.review == nil {
		return nil
	}
	copy := *candidate.review
	return &copy
}

func (candidate ExpansionCandidate) Decide(decision ExpansionDecision, reviewerUserID int64, reviewedAt time.Time, note string) (ExpansionCandidate, error) {
	if candidate.status != ExpansionApprovalPending {
		return ExpansionCandidate{}, ErrExpansionAlreadyReviewed
	}
	review, err := NewExpansionReview(decision, reviewerUserID, reviewedAt, note)
	if err != nil {
		return ExpansionCandidate{}, err
	}
	status := ExpansionApprovalRejected
	if decision == ExpansionDecisionApprove {
		status = ExpansionApprovalApproved
	}
	return RestoreExpansionCandidate(candidate.id, candidate.value, candidate.provenance, candidate.assessment, status, &review)
}

// ApprovedExpansionCandidates is the only projection intended for query
// compilation. Pending and rejected candidates are filtered before de-duping,
// so an unapproved high-priority value can never suppress an approved one.
func ApprovedExpansionCandidates(candidates []ExpansionCandidate) []ExpansionCandidate {
	approved := make([]ExpansionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.status == ExpansionApprovalApproved {
			approved = append(approved, candidate)
		}
	}
	sort.Slice(approved, func(i, j int) bool {
		leftPriority := approved[i].provenance.source.Priority()
		rightPriority := approved[j].provenance.source.Priority()
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftValue, rightValue := canonicalIntentKey(approved[i].value), canonicalIntentKey(approved[j].value)
		if leftValue != rightValue {
			return leftValue < rightValue
		}
		return approved[i].id < approved[j].id
	})
	seen := make(map[string]struct{}, len(approved))
	result := make([]ExpansionCandidate, 0, len(approved))
	for _, candidate := range approved {
		key := canonicalIntentKey(candidate.value)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func validIntentSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
