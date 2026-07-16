package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// RelevanceReviewRequest combines the already-determined rule snapshot with
// only the safe, bounded evidence allowed by the relevance_review schema.
// Calling Review always saves Snapshot first, even for a deterministic
// accepted/rejected result or a later AI degradation.
type RelevanceReviewRequest struct {
	Snapshot                        ingestiondomain.RelevanceSnapshotInput
	ContentExcerpt, ContentLanguage string
	MonitorIntent                   string
	Factors                         RelevanceFactors
	EvidenceTerms                   []string
	HighThreshold                   float64
}

type RelevanceReviewResult struct {
	Status, ReasonCode string
	Snapshot           ingestiondomain.RelevanceSnapshot
	Reused             bool
}

type relevanceReviewExecutor interface {
	Review(context.Context, intelligenceapplication.RelevanceReviewRequest) (intelligenceapplication.RelevanceReviewResult, error)
}

type relevanceReviewSnapshotRepository interface {
	UpsertSnapshot(context.Context, ingestiondomain.RelevanceSnapshotInput) (ingestiondomain.RelevanceSnapshot, bool, error)
	ApplySuccessfulReview(context.Context, ingestiondomain.SuccessfulReviewInput) (ingestiondomain.RelevanceSnapshot, error)
	MarkReviewUnavailable(context.Context, int64, int64, string) (ingestiondomain.RelevanceSnapshot, error)
}

type RelevanceReviewServiceDependencies struct {
	Snapshots relevanceReviewSnapshotRepository
	Reviews   relevanceReviewExecutor
}

// RelevanceReviewService is the ingestion-side orchestration boundary. It
// owns monitor_match persistence but delegates all AI work to intelligence's
// narrow application facade; it has no provider, credential or ai_* table
// dependency.
type RelevanceReviewService struct {
	snapshots relevanceReviewSnapshotRepository
	reviews   relevanceReviewExecutor
}

func NewRelevanceReviewService(dependencies RelevanceReviewServiceDependencies) (*RelevanceReviewService, error) {
	if dependencies.Snapshots == nil || dependencies.Reviews == nil {
		return nil, fmt.Errorf("relevance snapshots and review executor are required")
	}
	return &RelevanceReviewService{snapshots: dependencies.Snapshots, reviews: dependencies.Reviews}, nil
}

func (service *RelevanceReviewService) Review(ctx context.Context, request RelevanceReviewRequest) (RelevanceReviewResult, error) {
	if service == nil || service.snapshots == nil || service.reviews == nil || !validRelevanceReviewRequest(request) {
		return RelevanceReviewResult{}, fmt.Errorf("valid relevance review request is required")
	}
	snapshot, _, err := service.snapshots.UpsertSnapshot(ctx, request.Snapshot)
	if err != nil {
		return RelevanceReviewResult{}, err
	}
	result := RelevanceReviewResult{Status: "rule", Snapshot: snapshot}
	if !eligibleForAIReview(snapshot) {
		return result, nil
	}

	executed, err := service.reviews.Review(ctx, buildRelevanceReviewInput(snapshot, request))
	if err != nil || executed.Status != "succeeded" || executed.RunID <= 0 {
		reason := "ai_unavailable"
		if executed.ReasonCode == "ai_in_progress" {
			reason = "ai_in_progress"
		}
		return service.degrade(ctx, request.Snapshot, snapshot, reason)
	}
	decision := ingestiondomain.MatchDecision(executed.Decision)
	if !decision.Valid() || decision != decisionForReviewScore(executed.Score, request.HighThreshold) {
		return service.degrade(ctx, request.Snapshot, snapshot, "ai_unavailable")
	}
	reviewed, err := service.snapshots.ApplySuccessfulReview(ctx, ingestiondomain.SuccessfulReviewInput{
		SnapshotID: snapshot.ID, ExpectedVersion: snapshot.Version, ReviewAIRunID: executed.RunID,
		LLMScore: executed.Score, FinalScore: executed.Score, Decision: decision, ReasonCodes: append([]string(nil), executed.ReasonCodes...),
	})
	if err == nil {
		return RelevanceReviewResult{Status: "succeeded", Snapshot: reviewed, Reused: executed.Reused}, nil
	}
	if !errors.Is(err, sharedrepository.ErrConflict) {
		return RelevanceReviewResult{}, err
	}
	// A concurrent caller may have recorded ai_in_progress while this caller
	// owned the run. Refreshing keeps the AI result attachable without
	// overwriting a manual lock or a completed review.
	refreshed, _, refreshErr := service.snapshots.UpsertSnapshot(ctx, request.Snapshot)
	if refreshErr != nil {
		return RelevanceReviewResult{}, refreshErr
	}
	if eligibleForAIReview(refreshed) {
		reviewed, err = service.snapshots.ApplySuccessfulReview(ctx, ingestiondomain.SuccessfulReviewInput{
			SnapshotID: refreshed.ID, ExpectedVersion: refreshed.Version, ReviewAIRunID: executed.RunID,
			LLMScore: executed.Score, FinalScore: executed.Score, Decision: decision, ReasonCodes: append([]string(nil), executed.ReasonCodes...),
		})
		if err == nil {
			return RelevanceReviewResult{Status: "succeeded", Snapshot: reviewed, Reused: executed.Reused}, nil
		}
		if !errors.Is(err, sharedrepository.ErrConflict) {
			return RelevanceReviewResult{}, err
		}
		refreshed, _, refreshErr = service.snapshots.UpsertSnapshot(ctx, request.Snapshot)
		if refreshErr != nil {
			return RelevanceReviewResult{}, refreshErr
		}
	}
	return reviewResultForExistingSnapshot(refreshed), nil
}

func (service *RelevanceReviewService) degrade(ctx context.Context, original ingestiondomain.RelevanceSnapshotInput, snapshot ingestiondomain.RelevanceSnapshot, reason string) (RelevanceReviewResult, error) {
	updated, err := service.snapshots.MarkReviewUnavailable(ctx, snapshot.ID, snapshot.Version, reason)
	if err == nil {
		return RelevanceReviewResult{Status: "degraded", ReasonCode: reason, Snapshot: updated}, nil
	}
	if !errors.Is(err, sharedrepository.ErrConflict) {
		return RelevanceReviewResult{}, err
	}
	refreshed, _, refreshErr := service.snapshots.UpsertSnapshot(ctx, original)
	if refreshErr != nil {
		return RelevanceReviewResult{}, refreshErr
	}
	result := reviewResultForExistingSnapshot(refreshed)
	if result.Status == "rule" {
		result.Status, result.ReasonCode = "degraded", reason
	}
	return result, nil
}

func validRelevanceReviewRequest(request RelevanceReviewRequest) bool {
	if request.Snapshot.Validate() != nil || request.Snapshot.DecisionOrigin != ingestiondomain.DecisionOriginRule || request.Snapshot.LLMScore != nil ||
		request.Snapshot.FinalScore != request.Snapshot.RuleScore || request.HighThreshold < 75 || request.HighThreshold > 100 ||
		strings.TrimSpace(request.ContentExcerpt) == "" || strings.TrimSpace(request.MonitorIntent) == "" ||
		(request.ContentLanguage != "zh" && request.ContentLanguage != "en" && request.ContentLanguage != "und") ||
		math.IsNaN(request.Factors.Lexical) || math.IsNaN(request.Factors.Entity) || math.IsNaN(request.Factors.Title) || math.IsNaN(request.Factors.Preference) {
		return false
	}
	return true
}

func eligibleForAIReview(snapshot ingestiondomain.RelevanceSnapshot) bool {
	return !snapshot.ManualLocked && snapshot.Decision == ingestiondomain.MatchDecisionReview && snapshot.DecisionOrigin == ingestiondomain.DecisionOriginRule &&
		snapshot.LLMScore == nil && snapshot.ReviewAIRunID == nil
}

func buildRelevanceReviewInput(snapshot ingestiondomain.RelevanceSnapshot, request RelevanceReviewRequest) intelligenceapplication.RelevanceReviewRequest {
	reasons := make([]string, 0, 5)
	for _, path := range snapshot.RecallPaths {
		switch path {
		case "source":
			reasons = append(reasons, "source_candidate")
		case "lexical":
			reasons = append(reasons, "lexical_candidate")
		case "vector":
			reasons = append(reasons, "vector_candidate")
		}
	}
	if snapshot.Degraded {
		reasons = append(reasons, "degraded_vector")
	}
	reasons = uniqueStrings(append(reasons, "low_confidence"))
	semantic := 0.0
	if request.Factors.Semantic != nil {
		semantic = *request.Factors.Semantic
	}
	return intelligenceapplication.RelevanceReviewRequest{
		TargetID: snapshot.ID, InputHash: snapshot.InputHash, ContentExcerpt: request.ContentExcerpt, ContentLanguage: request.ContentLanguage,
		MonitorIntent: request.MonitorIntent, ScoringVersion: snapshot.ScoringVersion,
		Scores: intelligenceapplication.RelevanceReviewScores{
			Semantic: semantic, Lexical: request.Factors.Lexical, Entity: request.Factors.Entity, Title: request.Factors.Title, Preference: request.Factors.Preference,
		},
		RecallPaths: append([]string(nil), snapshot.RecallPaths...), ReasonCodes: reasons, EvidenceTerms: append([]string(nil), request.EvidenceTerms...),
	}
}

func decisionForReviewScore(score, highThreshold float64) ingestiondomain.MatchDecision {
	switch {
	case score >= highThreshold:
		return ingestiondomain.MatchDecisionAccepted
	case score < 60:
		return ingestiondomain.MatchDecisionRejected
	default:
		return ingestiondomain.MatchDecisionReview
	}
}

func reviewResultForExistingSnapshot(snapshot ingestiondomain.RelevanceSnapshot) RelevanceReviewResult {
	if snapshot.DecisionOrigin == ingestiondomain.DecisionOriginAI && snapshot.ReviewAIRunID != nil {
		return RelevanceReviewResult{Status: "succeeded", Snapshot: snapshot, Reused: true}
	}
	return RelevanceReviewResult{Status: "rule", Snapshot: snapshot}
}
