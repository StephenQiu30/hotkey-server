package application

import (
	"context"
	"fmt"
	"math"
	"sort"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
)

const (
	plan009SourceCandidateLimit  = 8
	plan009LexicalCandidateLimit = 12
	plan009VectorCandidateLimit  = 12
	plan009CandidateUnionLimit   = 20
)

// RelevanceContent is the safe, normalized fact provided to the bounded
// scorer. Body text, object keys, prompts and provider payloads are
// intentionally absent: none may influence the candidate hash or explanation.
type RelevanceContent struct {
	ID, SourceConnectionID                      int64
	DedupeKey, Language, Region, Title, Excerpt string
	CanonicalURL, AuthorExternalID, AuthorName  string
}

// ModelProfileReference is enough provenance for the deterministic input
// hash. It is valid only when all three fields identify one exact model space.
type ModelProfileReference struct {
	ID, Version  int64
	ModelVersion string
}

func (reference ModelProfileReference) valid() bool {
	return reference.ID > 0 && reference.Version > 0 && reference.ModelVersion != ""
}

// RelevanceScoreRequest selects an optional vector space and an optional
// relevance-review profile. Task 3 never executes review; retaining the
// selected review identity here makes its future input hash deterministic.
type RelevanceScoreRequest struct {
	Content                RelevanceContent
	EmbeddingProfile       *ModelProfileReference
	RelevanceReviewProfile *ModelProfileReference
}

// RelevanceFactors are five independently explainable scores in the fixed
// PLAN-009 formula. Semantic is nil when the candidate has no exact-space
// vector and the degraded formula was used.
type RelevanceFactors struct {
	Semantic                           *float64
	Lexical, Entity, Title, Preference float64
}

// ScoredRelevanceCandidate is an in-memory deterministic result. Task 4 owns
// persistence and AI review; this Task has no side effects.
type ScoredRelevanceCandidate struct {
	MonitorID, MonitorConfigVersionID int64
	InputHash                         string
	ScoringVersion                    string
	RecallPaths, ReasonCodes          []string
	MatchedTerms, MatchedEntities     []string
	ExcludedTerms                     []string
	Factors                           RelevanceFactors
	RuleScore                         float64
	Decision                          ingestiondomain.MatchDecision
	Degraded, HardVeto                bool
	EmbeddingProfile                  *ModelProfileReference
}

type embeddingQueryPort interface {
	ActiveContent(context.Context, int64, intelligenceapplication.EmbeddingSpace) (intelligenceapplication.ActiveEmbedding, bool, error)
	NearestMonitors(context.Context, intelligenceapplication.ActiveEmbedding, int) ([]intelligenceapplication.EmbeddingNeighbor, error)
}

// CandidateRecallService composes exactly three bounded candidate paths. The
// domain reader owns source/lexical SQL and one batch load; vector recall is
// available only through intelligence's public read facade.
type CandidateRecallService struct {
	candidates ingestiondomain.RelevanceCandidateReader
	embeddings embeddingQueryPort
}

func NewCandidateRecallService(candidates ingestiondomain.RelevanceCandidateReader, embeddings embeddingQueryPort) (*CandidateRecallService, error) {
	if candidates == nil {
		return nil, fmt.Errorf("relevance candidate reader is required")
	}
	return &CandidateRecallService{candidates: candidates, embeddings: embeddings}, nil
}

func (service *CandidateRecallService) Score(ctx context.Context, request RelevanceScoreRequest) ([]ScoredRelevanceCandidate, error) {
	if service == nil || service.candidates == nil || !validRelevanceContent(request.Content) {
		return nil, fmt.Errorf("valid relevance content is required")
	}
	if request.EmbeddingProfile != nil && !request.EmbeddingProfile.valid() || request.RelevanceReviewProfile != nil && !request.RelevanceReviewProfile.valid() {
		return nil, fmt.Errorf("complete relevance model profile reference is required")
	}
	source, err := service.candidates.SourceCandidates(ctx, request.Content.SourceConnectionID, plan009SourceCandidateLimit)
	if err != nil {
		return nil, err
	}
	terms := lexicalLookupTerms(request.Content.Title, request.Content.Excerpt)
	lexical := []ingestiondomain.RelevanceCandidateHit{}
	if len(terms) > 0 {
		lexical, err = service.candidates.LexicalCandidates(ctx, terms, plan009LexicalCandidateLimit)
		if err != nil {
			return nil, err
		}
	}

	vector, vectorProfile := service.vectorCandidates(ctx, request)
	selected := mergeCandidateHits(source, lexical, vector)
	if len(selected) == 0 {
		return []ScoredRelevanceCandidate{}, nil
	}
	ids := make([]int64, 0, len(selected))
	for _, candidate := range selected {
		ids = append(ids, candidate.monitorID)
	}
	loaded, err := service.candidates.LoadRelevanceCandidates(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]ingestiondomain.RelevanceCandidate, len(loaded))
	for _, candidate := range loaded {
		byID[candidate.MonitorID] = candidate
	}
	results := make([]ScoredRelevanceCandidate, 0, len(selected))
	for _, hit := range selected {
		candidate, exists := byID[hit.monitorID]
		if !exists {
			continue // state changed after the bounded candidate read
		}
		result, err := scoreRelevanceCandidate(request, candidate, hit, vectorProfile)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (service *CandidateRecallService) vectorCandidates(ctx context.Context, request RelevanceScoreRequest) ([]intelligenceapplication.EmbeddingNeighbor, *ModelProfileReference) {
	if service.embeddings == nil || request.EmbeddingProfile == nil || !request.EmbeddingProfile.valid() {
		return nil, nil
	}
	profile := *request.EmbeddingProfile
	active, found, err := service.embeddings.ActiveContent(ctx, request.Content.ID, intelligenceapplication.EmbeddingSpace{
		ModelProfileID: profile.ID, ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
	})
	if err != nil || !found {
		return nil, nil // vector reads are explicitly safe-degradable
	}
	neighbors, err := service.embeddings.NearestMonitors(ctx, active, plan009VectorCandidateLimit)
	if err != nil {
		return nil, nil
	}
	return neighbors, &profile
}

type mergedCandidateHit struct {
	monitorID int64
	source    bool
	lexical   float64
	vector    float64
	vectorSet bool
}

func mergeCandidateHits(source, lexical []ingestiondomain.RelevanceCandidateHit, vector []intelligenceapplication.EmbeddingNeighbor) []mergedCandidateHit {
	byID := map[int64]mergedCandidateHit{}
	for _, hit := range source[:min(len(source), plan009SourceCandidateLimit)] {
		if hit.MonitorID <= 0 {
			continue
		}
		value := byID[hit.MonitorID]
		value.monitorID, value.source = hit.MonitorID, true
		byID[hit.MonitorID] = value
	}
	for _, hit := range lexical[:min(len(lexical), plan009LexicalCandidateLimit)] {
		if hit.MonitorID <= 0 {
			continue
		}
		value := byID[hit.MonitorID]
		value.monitorID = hit.MonitorID
		if hit.LexicalScore > value.lexical {
			value.lexical = hit.LexicalScore
		}
		byID[hit.MonitorID] = value
	}
	for _, hit := range vector[:min(len(vector), plan009VectorCandidateLimit)] {
		if hit.TargetID <= 0 || math.IsNaN(hit.Distance) || math.IsInf(hit.Distance, 0) || hit.Distance < 0 {
			continue
		}
		value := byID[hit.TargetID]
		value.monitorID = hit.TargetID
		if !value.vectorSet || hit.Distance < value.vector {
			value.vector, value.vectorSet = hit.Distance, true
		}
		byID[hit.TargetID] = value
	}
	merged := make([]mergedCandidateHit, 0, len(byID))
	for _, hit := range byID {
		merged = append(merged, hit)
	}
	sort.Slice(merged, func(left, right int) bool {
		first, second := merged[left], merged[right]
		if first.source != second.source {
			return first.source
		}
		if first.lexical != second.lexical {
			return first.lexical > second.lexical
		}
		firstDistance, secondDistance := candidateDistance(first), candidateDistance(second)
		if firstDistance != secondDistance {
			return firstDistance < secondDistance
		}
		return first.monitorID < second.monitorID
	})
	if len(merged) > plan009CandidateUnionLimit {
		merged = merged[:plan009CandidateUnionLimit]
	}
	return merged
}

func candidateDistance(hit mergedCandidateHit) float64 {
	if !hit.vectorSet {
		return math.Inf(1)
	}
	return hit.vector
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
