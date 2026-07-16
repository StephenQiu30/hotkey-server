package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type MatchDecision string

const (
	MatchDecisionAccepted MatchDecision = "accepted"
	MatchDecisionReview   MatchDecision = "review"
	MatchDecisionRejected MatchDecision = "rejected"
)

func (decision MatchDecision) Valid() bool {
	return decision == MatchDecisionAccepted || decision == MatchDecisionReview || decision == MatchDecisionRejected
}

type DecisionOrigin string

const (
	DecisionOriginRule DecisionOrigin = "rule"
	DecisionOriginAI   DecisionOrigin = "ai"
)

func (origin DecisionOrigin) Valid() bool {
	return origin == DecisionOriginRule || origin == DecisionOriginAI
}

// RelevanceSnapshotInput is the immutable score/result fact for exactly one
// monitored Content input. Its model fields preserve vector provenance
// separately from the structured-review run so a later review cannot be
// mistaken for the embedding space that produced a candidate.
type RelevanceSnapshotInput struct {
	MonitorID, MonitorConfigVersionID, ContentID int64
	InputHash, ScoringVersion                    string
	RecallPaths, ReasonCodes                     []string
	RuleScore                                    float64
	SemanticScore, LLMScore                      *float64
	FinalScore                                   float64
	Decision                                     MatchDecision
	DecisionOrigin                               DecisionOrigin
	Explanation                                  json.RawMessage
	Degraded                                     bool
	EmbeddingModelProfileID                      *int64
	EmbeddingModelProfileVersion                 *int64
	EmbeddingModelVersion                        *string
}

func (input RelevanceSnapshotInput) Validate() error {
	if input.MonitorID <= 0 || input.MonitorConfigVersionID <= 0 || input.ContentID <= 0 ||
		!validSHA256(input.InputHash) || strings.TrimSpace(input.ScoringVersion) == "" || len(input.ScoringVersion) > 64 ||
		!input.Decision.Valid() || !input.DecisionOrigin.Valid() || !validRelevanceScore(input.RuleScore) || !validRelevanceScore(input.FinalScore) {
		return fmt.Errorf("invalid relevance snapshot")
	}
	if input.SemanticScore != nil && !validRelevanceScore(*input.SemanticScore) || input.LLMScore != nil && !validRelevanceScore(*input.LLMScore) {
		return fmt.Errorf("invalid relevance score")
	}
	if !validRecallPaths(input.RecallPaths) || !validReasonCodes(input.ReasonCodes, 12) || validateExplanation(input.Explanation) != nil {
		return fmt.Errorf("invalid relevance explanation")
	}
	if (input.EmbeddingModelProfileID == nil) != (input.EmbeddingModelProfileVersion == nil) ||
		(input.EmbeddingModelProfileID == nil) != (input.EmbeddingModelVersion == nil) {
		return fmt.Errorf("incomplete embedding provenance")
	}
	if input.EmbeddingModelProfileID != nil && (*input.EmbeddingModelProfileID <= 0 || *input.EmbeddingModelProfileVersion <= 0 || strings.TrimSpace(*input.EmbeddingModelVersion) == "" || len(*input.EmbeddingModelVersion) > 64) {
		return fmt.Errorf("invalid embedding provenance")
	}
	if input.DecisionOrigin == DecisionOriginRule && input.LLMScore != nil {
		return fmt.Errorf("rule decision cannot carry AI provenance")
	}
	if input.DecisionOrigin == DecisionOriginAI {
		return fmt.Errorf("invalid AI decision provenance")
	}
	return nil
}

type RelevanceSnapshot struct {
	RelevanceSnapshotInput
	ID, Version          int64
	ReviewAIRunID        *int64
	CreatedAt, UpdatedAt time.Time
}

type RelevanceSnapshotCursor struct {
	FinalScore float64
	ID         int64
}

type RelevanceSnapshotListQuery struct {
	Limit    int
	Decision *MatchDecision
	Cursor   *RelevanceSnapshotCursor
}

type RelevanceSnapshotPage struct {
	Items []RelevanceSnapshot
	Next  *RelevanceSnapshotCursor
}

// SuccessfulReviewInput is separate from snapshot creation because the AI
// run needs the persisted monitor_match ID as its target before its successful
// provenance can be attached. It never changes the deterministic rule score.
type SuccessfulReviewInput struct {
	SnapshotID, ExpectedVersion, ReviewAIRunID int64
	LLMScore, FinalScore                       float64
	Decision                                   MatchDecision
	ReasonCodes                                []string
}

func (input SuccessfulReviewInput) Validate() error {
	if input.SnapshotID <= 0 || input.ExpectedVersion <= 0 || input.ReviewAIRunID <= 0 ||
		!input.Decision.Valid() || !validRelevanceScore(input.LLMScore) || !validRelevanceScore(input.FinalScore) || input.LLMScore != input.FinalScore ||
		!validReasonCodes(input.ReasonCodes, 12) {
		return fmt.Errorf("invalid successful relevance review")
	}
	return nil
}

func (query RelevanceSnapshotListQuery) Validate() error {
	if query.Limit < 1 || query.Limit > 100 || query.Decision != nil && !query.Decision.Valid() ||
		query.Cursor != nil && (!validRelevanceScore(query.Cursor.FinalScore) || query.Cursor.ID <= 0) {
		return fmt.Errorf("invalid relevance snapshot list query")
	}
	return nil
}

type FeedbackType string

const (
	FeedbackTypeRelevant      FeedbackType = "relevant"
	FeedbackTypeIrrelevant    FeedbackType = "irrelevant"
	FeedbackTypeFalsePositive FeedbackType = "false_positive"
	FeedbackTypeFalseNegative FeedbackType = "false_negative"
)

func (feedback FeedbackType) Valid() bool {
	switch feedback {
	case FeedbackTypeRelevant, FeedbackTypeIrrelevant, FeedbackTypeFalsePositive, FeedbackTypeFalseNegative:
		return true
	default:
		return false
	}
}

type RelevanceFeedbackInput struct {
	MonitorID, MonitorConfigVersionID, ContentID, ActorUserID int64
	MonitorMatchID                                            *int64
	ExpectedVersion                                           *int64
	FeedbackType                                              FeedbackType
}

func (input RelevanceFeedbackInput) Validate() error {
	if input.MonitorID <= 0 || input.MonitorConfigVersionID <= 0 || input.ContentID <= 0 || input.ActorUserID <= 0 || !input.FeedbackType.Valid() ||
		input.MonitorMatchID != nil && *input.MonitorMatchID <= 0 || input.ExpectedVersion != nil && *input.ExpectedVersion <= 0 {
		return fmt.Errorf("invalid relevance feedback")
	}
	return nil
}

type RelevanceFeedback struct {
	RelevanceFeedbackInput
	ID, Version          int64
	CreatedAt, UpdatedAt time.Time
}

type SuggestionType string

const (
	SuggestionTypeAddTerm    SuggestionType = "add_term"
	SuggestionTypeAddExclude SuggestionType = "add_exclude"
	SuggestionTypeAddEntity  SuggestionType = "add_entity"
)

func (suggestion SuggestionType) Valid() bool {
	return suggestion == SuggestionTypeAddTerm || suggestion == SuggestionTypeAddExclude || suggestion == SuggestionTypeAddEntity
}

type SuggestionStatus string

const (
	SuggestionStatusPending  SuggestionStatus = "pending"
	SuggestionStatusApproved SuggestionStatus = "approved"
	SuggestionStatusRejected SuggestionStatus = "rejected"
)

func (status SuggestionStatus) Valid() bool {
	return status == SuggestionStatusPending || status == SuggestionStatusApproved || status == SuggestionStatusRejected
}

type RelevanceSuggestionInput struct {
	MonitorID, MonitorConfigVersionID int64
	SuggestionType                    SuggestionType
	Value                             string
	SupportCount                      int
}

func (input RelevanceSuggestionInput) Validate() error {
	if input.MonitorID <= 0 || input.MonitorConfigVersionID <= 0 || !input.SuggestionType.Valid() ||
		strings.TrimSpace(input.Value) == "" || len(input.Value) > 500 || input.SupportCount < 2 {
		return fmt.Errorf("invalid relevance suggestion")
	}
	return nil
}

type RelevanceSuggestion struct {
	ID, Version, MonitorID, MonitorConfigVersionID int64
	SuggestionType                                 SuggestionType
	Value                                          string
	SupportCount                                   int
	Status                                         SuggestionStatus
	ReviewedByUserID                               *int64
	CreatedAt, UpdatedAt                           time.Time
}

func validRelevanceScore(score float64) bool {
	return !math.IsNaN(score) && !math.IsInf(score, 0) && score >= 0 && score <= 100
}

func validRecallPaths(paths []string) bool {
	if len(paths) > 3 {
		return false
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path != "source" && path != "lexical" && path != "vector" {
			return false
		}
		if _, exists := seen[path]; exists {
			return false
		}
		seen[path] = struct{}{}
	}
	return true
}

func validReasonCodes(codes []string, maximum int) bool {
	if len(codes) > maximum {
		return false
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if strings.TrimSpace(code) == "" || len(code) > 64 {
			return false
		}
		if _, exists := seen[code]; exists {
			return false
		}
		seen[code] = struct{}{}
	}
	return true
}

func validateExplanation(payload json.RawMessage) error {
	if !json.Valid(payload) {
		return fmt.Errorf("explanation is not JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{
		"matched_terms": true, "matched_entities": true, "excluded_terms": true, "recall_paths": true,
		"scores": true, "reason_codes": true, "provenance": true,
	}
	for name, value := range fields {
		if !allowed[name] {
			return fmt.Errorf("forbidden explanation field %q", name)
		}
		switch name {
		case "matched_terms", "matched_entities", "excluded_terms", "reason_codes":
			var values []string
			if err := json.Unmarshal(value, &values); err != nil || !validReasonCodes(values, 64) {
				return fmt.Errorf("invalid explanation %s", name)
			}
		case "recall_paths":
			var paths []string
			if err := json.Unmarshal(value, &paths); err != nil || !validRecallPaths(paths) {
				return fmt.Errorf("invalid explanation recall paths")
			}
		case "scores":
			var scores map[string]float64
			if err := json.Unmarshal(value, &scores); err != nil || len(scores) != 5 {
				return fmt.Errorf("invalid explanation scores")
			}
			for _, name := range []string{"semantic", "lexical", "entity", "title", "preference"} {
				if score, exists := scores[name]; !exists || !validRelevanceScore(score) {
					return fmt.Errorf("invalid explanation score")
				}
			}
		case "provenance":
			var provenance map[string]json.RawMessage
			if err := json.Unmarshal(value, &provenance); err != nil {
				return fmt.Errorf("invalid explanation provenance")
			}
			for field := range provenance {
				switch field {
				case "scoring_version", "embedding_model_profile_id", "embedding_model_profile_version", "embedding_model_version", "review_ai_run_id", "legacy_backfill":
				default:
					return fmt.Errorf("forbidden explanation provenance field %q", field)
				}
			}
		}
	}
	return nil
}
