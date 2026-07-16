package http

import (
	"encoding/json"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
)

// RelevanceExplanationResponse is a strict allowlist for a persisted score
// explanation. It deliberately excludes raw Content, input hashes, object
// keys, provider inputs, credentials, and internal AI-run identities.
type RelevanceExplanationResponse struct {
	MatchedTerms    []string           `json:"matched_terms"`
	MatchedEntities []string           `json:"matched_entities"`
	ExcludedTerms   []string           `json:"excluded_terms"`
	RecallPaths     []string           `json:"recall_paths"`
	Scores          map[string]float64 `json:"scores"`
	ReasonCodes     []string           `json:"reason_codes"`
}

type RelevanceMatchResponse struct {
	ID                     int64                        `json:"id"`
	Version                int64                        `json:"version"`
	ContentID              int64                        `json:"content_id"`
	MonitorConfigVersionID int64                        `json:"monitor_config_version_id"`
	ScoringVersion         string                       `json:"scoring_version"`
	RecallPaths            []string                     `json:"recall_paths"`
	ReasonCodes            []string                     `json:"reason_codes"`
	RuleScore              float64                      `json:"rule_score"`
	SemanticScore          *float64                     `json:"semantic_score" extensions:"x-nullable"`
	LLMScore               *float64                     `json:"llm_score" extensions:"x-nullable"`
	FinalScore             float64                      `json:"final_score"`
	Decision               string                       `json:"decision" enums:"accepted,rejected,review"`
	DecisionOrigin         string                       `json:"decision_origin" enums:"rule,ai"`
	Degraded               bool                         `json:"degraded"`
	ManualLocked           bool                         `json:"manual_locked"`
	Explanation            RelevanceExplanationResponse `json:"explanation"`
	CreatedAt              time.Time                    `json:"created_at"`
}

type RelevanceMatchPageResponse struct {
	Items      []RelevanceMatchResponse `json:"items"`
	NextCursor string                   `json:"next_cursor"`
}

type RelevanceContentResponse struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	CanonicalURL string    `json:"canonical_url"`
	Language     string    `json:"language"`
	PublishedAt  time.Time `json:"published_at"`
}

type RelevanceMatchDetailResponse struct {
	Match   RelevanceMatchResponse   `json:"match"`
	Content RelevanceContentResponse `json:"content"`
}

type RelevanceFactorsResponse struct {
	Semantic   *float64 `json:"semantic" extensions:"x-nullable"`
	Lexical    float64  `json:"lexical"`
	Entity     float64  `json:"entity"`
	Title      float64  `json:"title"`
	Preference float64  `json:"preference"`
}

type RelevancePreviewCandidateResponse struct {
	MonitorConfigVersionID int64                    `json:"monitor_config_version_id"`
	ScoringVersion         string                   `json:"scoring_version"`
	RecallPaths            []string                 `json:"recall_paths"`
	ReasonCodes            []string                 `json:"reason_codes"`
	MatchedTerms           []string                 `json:"matched_terms"`
	MatchedEntities        []string                 `json:"matched_entities"`
	ExcludedTerms          []string                 `json:"excluded_terms"`
	Factors                RelevanceFactorsResponse `json:"factors"`
	RuleScore              float64                  `json:"rule_score"`
	Decision               string                   `json:"decision"`
	Degraded               bool                     `json:"degraded"`
	HardVeto               bool                     `json:"hard_veto"`
}

type RelevancePreviewItemResponse struct {
	ContentID  int64                               `json:"content_id"`
	Candidates []RelevancePreviewCandidateResponse `json:"candidates"`
}

type RelevanceFeedbackResponse struct {
	ID           int64     `json:"id"`
	Version      int64     `json:"version"`
	ContentID    int64     `json:"content_id"`
	MatchID      *int64    `json:"match_id" extensions:"x-nullable"`
	FeedbackType string    `json:"feedback_type"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RelevanceEvaluationResponse struct {
	ScoringVersion             string  `json:"scoring_version"`
	PrecisionAt20              float64 `json:"precision_at_20"`
	ExclusionFalsePositiveRate float64 `json:"exclusion_false_positive_rate"`
	EvaluatedCount             int64   `json:"evaluated_count"`
}

type RelevanceSuggestionResponse struct {
	ID         int64     `json:"id"`
	Version    int64     `json:"version"`
	Suggestion string    `json:"suggestion_type"`
	Value      string    `json:"value"`
	Support    int       `json:"support_count"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type RelevanceSuggestionPageResponse struct {
	Items      []RelevanceSuggestionResponse `json:"items"`
	NextCursor string                        `json:"next_cursor"`
}

type RelevanceRefreshResponse struct {
	SuggestionCount int `json:"suggestion_count"`
}

func relevanceMatchResponse(snapshot ingestiondomain.RelevanceSnapshot) RelevanceMatchResponse {
	return RelevanceMatchResponse{
		ID: snapshot.ID, Version: snapshot.Version, ContentID: snapshot.ContentID, MonitorConfigVersionID: snapshot.MonitorConfigVersionID,
		ScoringVersion: snapshot.ScoringVersion, RecallPaths: snapshot.RecallPaths, ReasonCodes: snapshot.ReasonCodes,
		RuleScore: snapshot.RuleScore, SemanticScore: snapshot.SemanticScore, LLMScore: snapshot.LLMScore, FinalScore: snapshot.FinalScore,
		Decision: string(snapshot.Decision), DecisionOrigin: string(snapshot.DecisionOrigin), Degraded: snapshot.Degraded,
		ManualLocked: snapshot.ManualLocked, Explanation: relevanceExplanationResponse(snapshot.Explanation), CreatedAt: snapshot.CreatedAt,
	}
}

func relevanceExplanationResponse(raw json.RawMessage) RelevanceExplanationResponse {
	var value RelevanceExplanationResponse
	_ = json.Unmarshal(raw, &value)
	return value
}

func relevanceContentResponse(content ingestiondomain.Content) RelevanceContentResponse {
	return RelevanceContentResponse{ID: content.ID, Title: content.Title, CanonicalURL: content.CanonicalURL, Language: content.Language, PublishedAt: content.PublishedAt}
}

func relevancePreviewItemResponse(item ingestionapplication.RelevancePreviewItem) RelevancePreviewItemResponse {
	response := RelevancePreviewItemResponse{ContentID: item.ContentID, Candidates: make([]RelevancePreviewCandidateResponse, 0, len(item.Candidates))}
	for _, candidate := range item.Candidates {
		response.Candidates = append(response.Candidates, RelevancePreviewCandidateResponse{
			MonitorConfigVersionID: candidate.MonitorConfigVersionID, ScoringVersion: candidate.ScoringVersion,
			RecallPaths: candidate.RecallPaths, ReasonCodes: candidate.ReasonCodes, MatchedTerms: candidate.MatchedTerms,
			MatchedEntities: candidate.MatchedEntities, ExcludedTerms: candidate.ExcludedTerms,
			Factors:   RelevanceFactorsResponse{Semantic: candidate.Factors.Semantic, Lexical: candidate.Factors.Lexical, Entity: candidate.Factors.Entity, Title: candidate.Factors.Title, Preference: candidate.Factors.Preference},
			RuleScore: candidate.RuleScore, Decision: string(candidate.Decision), Degraded: candidate.Degraded, HardVeto: candidate.HardVeto,
		})
	}
	return response
}

func relevanceFeedbackResponse(feedback ingestiondomain.RelevanceFeedback) RelevanceFeedbackResponse {
	return RelevanceFeedbackResponse{ID: feedback.ID, Version: feedback.Version, ContentID: feedback.ContentID, MatchID: feedback.MonitorMatchID, FeedbackType: string(feedback.FeedbackType), UpdatedAt: feedback.UpdatedAt}
}

func relevanceEvaluationResponse(value ingestiondomain.RelevanceEvaluation) RelevanceEvaluationResponse {
	return RelevanceEvaluationResponse{ScoringVersion: value.ScoringVersion, PrecisionAt20: value.PrecisionAt20, ExclusionFalsePositiveRate: value.ExclusionFalsePositiveRate, EvaluatedCount: value.EvaluatedCount}
}

func relevanceSuggestionResponse(value ingestiondomain.RelevanceSuggestion) RelevanceSuggestionResponse {
	return RelevanceSuggestionResponse{ID: value.ID, Version: value.Version, Suggestion: string(value.SuggestionType), Value: value.Value, Support: value.SupportCount, Status: string(value.Status), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}
