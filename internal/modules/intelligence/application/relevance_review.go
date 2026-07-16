package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

const (
	relevanceReviewPromptVersion      = "relevance-review-v1"
	relevanceReviewInputSchemaVersion = "v1"
	relevanceReviewSchemaVersion      = "v1"
	relevanceReviewParametersVersion  = "relevance-v1"
)

// RelevanceReviewRequest is the whole allowed cross-module input for one
// relevance_review run. It deliberately contains neither arbitrary prompt
// text nor provider configuration; RunService still performs the static JSON
// schema validation before any claim is made.
type RelevanceReviewRequest struct {
	TargetID                                int64
	InputHash                               string
	ContentExcerpt, ContentLanguage         string
	MonitorIntent                           string
	ScoringVersion                          string
	Scores                                  RelevanceReviewScores
	RecallPaths, ReasonCodes, EvidenceTerms []string
}

// RelevanceReviewScores keeps the five deterministic factors explicit. It is
// not the final decision and never accepts an arbitrary score field map.
type RelevanceReviewScores struct {
	Semantic, Lexical, Entity, Title, Preference float64
}

// RelevanceReviewResult exposes only schema-validated business fields and an
// AI run ID suitable for the ingestion provenance check. The Provider result,
// prompt and raw response are intentionally unavailable to callers.
type RelevanceReviewResult struct {
	Status, ReasonCode string
	RunID              int64
	Decision           string
	Score              float64
	ReasonCodes        []string
	Reused             bool
}

// RelevanceReviewService is intelligence's single structured facade for the
// PLAN-009 task. Downstream modules use this instead of RunService so they
// cannot select task types, schemas, providers or free-form instructions.
type RelevanceReviewService struct{ runs *RunService }

func NewRelevanceReviewService(runs *RunService) (*RelevanceReviewService, error) {
	if runs == nil {
		return nil, fmt.Errorf("AI run service is required for relevance review")
	}
	return &RelevanceReviewService{runs: runs}, nil
}

func (service *RelevanceReviewService) Review(ctx context.Context, request RelevanceReviewRequest) (RelevanceReviewResult, error) {
	if service == nil || service.runs == nil || !request.valid() {
		return RelevanceReviewResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	payload, err := json.Marshal(struct {
		ContentExcerpt  string                `json:"content_excerpt"`
		ContentLanguage string                `json:"content_language"`
		MonitorIntent   string                `json:"monitor_intent"`
		ScoringVersion  string                `json:"scoring_version"`
		Scores          RelevanceReviewScores `json:"scores"`
		RecallPaths     []string              `json:"recall_paths"`
		ReasonCodes     []string              `json:"reason_codes"`
		EvidenceTerms   []string              `json:"evidence_terms"`
	}{
		ContentExcerpt: request.ContentExcerpt, ContentLanguage: request.ContentLanguage, MonitorIntent: request.MonitorIntent,
		ScoringVersion: request.ScoringVersion, Scores: request.Scores, RecallPaths: append([]string(nil), request.RecallPaths...),
		ReasonCodes: append([]string(nil), request.ReasonCodes...), EvidenceTerms: append([]string(nil), request.EvidenceTerms...),
	})
	if err != nil {
		return RelevanceReviewResult{}, err
	}
	executed, err := service.runs.ExecuteStructured(ctx, StructuredExecutionInput{
		TaskType: domain.TaskTypeRelevanceReview, TargetType: "monitor_match", TargetID: request.TargetID,
		PromptVersion: relevanceReviewPromptVersion, InputSchemaVersion: relevanceReviewInputSchemaVersion,
		SchemaVersion: relevanceReviewSchemaVersion, ParametersVersion: relevanceReviewParametersVersion,
		InputHash: request.InputHash, EvidenceSetHash: request.InputHash, Input: payload,
	})
	if err != nil {
		if reason, safe := safeRelevanceReviewDegradation(err); safe {
			return RelevanceReviewResult{Status: "degraded", ReasonCode: reason}, nil
		}
		return RelevanceReviewResult{}, err
	}
	if executed.Status != "succeeded" {
		return RelevanceReviewResult{Status: "degraded", ReasonCode: "ai_unavailable"}, nil
	}
	var output struct {
		Decision    string   `json:"decision"`
		Score       float64  `json:"score"`
		ReasonCodes []string `json:"reason_codes"`
	}
	if err := json.Unmarshal(executed.Result, &output); err != nil || !validRelevanceReviewDecision(output.Decision) ||
		!validRelevanceReviewScore(output.Score) || !validRelevanceReviewOutputReasons(output.ReasonCodes) {
		return RelevanceReviewResult{Status: "degraded", ReasonCode: "ai_unavailable"}, nil
	}
	return RelevanceReviewResult{
		Status: "succeeded", RunID: executed.Run.ID, Decision: output.Decision, Score: output.Score,
		ReasonCodes: append([]string(nil), output.ReasonCodes...), Reused: executed.Reused,
	}, nil
}

func (request RelevanceReviewRequest) valid() bool {
	if request.TargetID <= 0 || !validSHA256(request.InputHash) || strings.TrimSpace(request.ContentExcerpt) == "" ||
		utf8.RuneCountInString(request.ContentExcerpt) > 1200 || strings.TrimSpace(request.MonitorIntent) == "" ||
		utf8.RuneCountInString(request.MonitorIntent) > 500 || (request.ContentLanguage != "zh" && request.ContentLanguage != "en" && request.ContentLanguage != "und") ||
		(request.ScoringVersion != "relevance-v1" && request.ScoringVersion != "relevance-v1-degraded") ||
		!validRelevanceReviewScore(request.Scores.Semantic) || !validRelevanceReviewScore(request.Scores.Lexical) ||
		!validRelevanceReviewScore(request.Scores.Entity) || !validRelevanceReviewScore(request.Scores.Title) || !validRelevanceReviewScore(request.Scores.Preference) ||
		!validRelevanceReviewPaths(request.RecallPaths) || !validRelevanceReviewInputReasons(request.ReasonCodes) || !validRelevanceReviewEvidenceTerms(request.EvidenceTerms) {
		return false
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validRelevanceReviewScore(value float64) bool { return value >= 0 && value <= 100 }

func validRelevanceReviewDecision(value string) bool {
	return value == "accepted" || value == "review" || value == "rejected"
}

func validRelevanceReviewPaths(values []string) bool {
	if len(values) == 0 || len(values) > 3 {
		return false
	}
	valid := map[string]bool{"source": true, "lexical": true, "vector": true}
	return uniqueKnownStrings(values, valid, 0)
}

func validRelevanceReviewInputReasons(values []string) bool {
	valid := map[string]bool{"source_candidate": true, "lexical_candidate": true, "vector_candidate": true, "low_confidence": true, "degraded_vector": true}
	return uniqueKnownStrings(values, valid, 12)
}

func validRelevanceReviewOutputReasons(values []string) bool {
	if len(values) == 0 {
		return false
	}
	valid := map[string]bool{"relevant_evidence": true, "insufficient_evidence": true, "ambiguous_context": true, "conflicting_signals": true}
	return uniqueKnownStrings(values, valid, 8)
}

func uniqueKnownStrings(values []string, allowed map[string]bool, maximum int) bool {
	if maximum > 0 && len(values) > maximum {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !allowed[value] {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validRelevanceReviewEvidenceTerms(values []string) bool {
	if len(values) > 16 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || utf8.RuneCountInString(value) > 120 {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func safeRelevanceReviewDegradation(err error) (string, bool) {
	code, known := domain.CodeOf(err)
	if !known {
		return "", false
	}
	switch code {
	case domain.CodeAIRunInProgress, domain.CodeAIRunLeaseExpired:
		return "ai_in_progress", true
	case domain.CodeAIModelUnavailable, domain.CodeAIBudgetExhausted, domain.CodeAIProviderRateLimited,
		domain.CodeAIProviderTransient, domain.CodeAIProviderTimeout, domain.CodeAIOutputInvalid:
		return "ai_unavailable", true
	default:
		return "", false
	}
}
