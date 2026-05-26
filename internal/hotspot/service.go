package hotspot

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

const (
	SortByHeat      = "heat"
	SortByTrust     = "trust"
	SortByRelevance = "relevance"
)

var ErrHotspotNotFound = errors.New("hotspot not found")

type RelatedContent struct {
	SourceItemID string `json:"sourceItemId"`
	Title        string `json:"title"`
	URL          string `json:"url"`
}

type EvidenceDetail struct {
	FactEvidenceIDs   []string `json:"factEvidenceIds"`
	SignalEvidenceIDs []string `json:"signalEvidenceIds"`
	RiskLabels        []string `json:"riskLabels"`
}

type HotspotSummary struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Keywords        []string `json:"keywords"`
	Region          string   `json:"region"`
	Language        string   `json:"language"`
	HeatScore       int      `json:"heatScore"`
	TrustScore      int      `json:"trustScore"`
	SimilarityScore float64  `json:"similarityScore"`
	RelevanceScore  float64  `json:"relevanceScore"`
}

type HotspotDetail struct {
	HotspotSummary
	RelatedContent []RelatedContent `json:"relatedContent"`
	Evidence       EvidenceDetail   `json:"evidence"`
}

type HotspotInput struct {
	ID              string
	Title           string
	Keywords        []string
	Region          string
	Language        string
	HeatScore       int
	TrustScore      int
	SimilarityScore float64
	RelatedContent  []RelatedContent
	Evidence        EvidenceDetail
}

type ListOptions struct {
	Keyword  string
	Region   string
	Language string
	MinTrust int
	SortBy   string
}

type Service struct {
	mu       sync.Mutex
	hotspots map[string]HotspotDetail
}

func NewService() *Service {
	service := NewEmptyService()
	service.UpsertHotspot(HotspotInput{
		ID:              "cluster_openai_reasoning",
		Title:           "OpenAI releases new reasoning model",
		Keywords:        []string{"OpenAI", "model", "reasoning"},
		Region:          "global",
		Language:        "en",
		HeatScore:       88,
		TrustScore:      95,
		SimilarityScore: 0.93,
		RelatedContent: []RelatedContent{
			{SourceItemID: "item_1", Title: "Research source", URL: "https://arxiv.org/abs/2605.00001"},
			{SourceItemID: "item_2", Title: "Developer discussion", URL: "https://github.com/trending"},
		},
		Evidence: EvidenceDetail{
			FactEvidenceIDs:   []string{"item_1"},
			SignalEvidenceIDs: []string{"item_2"},
			RiskLabels:        []string{"needs_followup"},
		},
	})
	service.UpsertHotspot(HotspotInput{
		ID:              "cluster_ai_safety_report",
		Title:           "Anthropic publishes AI safety report",
		Keywords:        []string{"Anthropic", "safety", "AI"},
		Region:          "global",
		Language:        "en",
		HeatScore:       64,
		TrustScore:      90,
		SimilarityScore: 0.86,
		RelatedContent: []RelatedContent{
			{SourceItemID: "item_3", Title: "Safety report", URL: "https://example.com/safety-report"},
		},
		Evidence: EvidenceDetail{
			FactEvidenceIDs: []string{"item_3"},
			RiskLabels:      []string{"low_risk"},
		},
	})
	return service
}

func NewEmptyService() *Service {
	return &Service{
		hotspots: make(map[string]HotspotDetail),
	}
}

func (s *Service) UpsertHotspot(input HotspotInput) {
	detail := normalizeHotspot(input)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.hotspots[detail.ID] = detail
}

func (s *Service) ListHotspots(options ListOptions) []HotspotSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]HotspotSummary, 0, len(s.hotspots))
	for _, detail := range s.hotspots {
		if !matches(detail.HotspotSummary, options) {
			continue
		}
		summary := detail.HotspotSummary
		summary.Keywords = append([]string(nil), summary.Keywords...)
		result = append(result, summary)
	}
	sortHotspots(result, options.SortBy)
	return result
}

func (s *Service) GetHotspotDetail(id string) (HotspotDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	detail, ok := s.hotspots[id]
	if !ok {
		return HotspotDetail{}, ErrHotspotNotFound
	}
	return cloneDetail(detail), nil
}

func normalizeHotspot(input HotspotInput) HotspotDetail {
	keywords := normalizeStrings(input.Keywords)
	relevance := (float64(input.HeatScore) * 0.4) + (float64(input.TrustScore) * 0.4) + (input.SimilarityScore * 100 * 0.2)
	return HotspotDetail{
		HotspotSummary: HotspotSummary{
			ID:              strings.TrimSpace(input.ID),
			Title:           strings.Join(strings.Fields(input.Title), " "),
			Keywords:        keywords,
			Region:          strings.TrimSpace(input.Region),
			Language:        strings.TrimSpace(input.Language),
			HeatScore:       input.HeatScore,
			TrustScore:      input.TrustScore,
			SimilarityScore: input.SimilarityScore,
			RelevanceScore:  relevance,
		},
		RelatedContent: cloneRelated(input.RelatedContent),
		Evidence: EvidenceDetail{
			FactEvidenceIDs:   normalizeStrings(input.Evidence.FactEvidenceIDs),
			SignalEvidenceIDs: normalizeStrings(input.Evidence.SignalEvidenceIDs),
			RiskLabels:        normalizeStrings(input.Evidence.RiskLabels),
		},
	}
}

func matches(summary HotspotSummary, options ListOptions) bool {
	if options.Region != "" && summary.Region != options.Region {
		return false
	}
	if options.Language != "" && summary.Language != options.Language {
		return false
	}
	if options.MinTrust > 0 && summary.TrustScore < options.MinTrust {
		return false
	}
	if options.Keyword != "" && !containsKeyword(summary, options.Keyword) {
		return false
	}
	return true
}

func containsKeyword(summary HotspotSummary, keyword string) bool {
	target := strings.ToLower(strings.TrimSpace(keyword))
	if target == "" {
		return true
	}
	if strings.Contains(strings.ToLower(summary.Title), target) {
		return true
	}
	for _, value := range summary.Keywords {
		if strings.ToLower(value) == target {
			return true
		}
	}
	return false
}

func sortHotspots(hotspots []HotspotSummary, sortBy string) {
	sort.Slice(hotspots, func(i, j int) bool {
		switch sortBy {
		case SortByTrust:
			if hotspots[i].TrustScore == hotspots[j].TrustScore {
				return hotspots[i].ID < hotspots[j].ID
			}
			return hotspots[i].TrustScore > hotspots[j].TrustScore
		case SortByRelevance:
			if hotspots[i].RelevanceScore == hotspots[j].RelevanceScore {
				return hotspots[i].ID < hotspots[j].ID
			}
			return hotspots[i].RelevanceScore > hotspots[j].RelevanceScore
		default:
			if hotspots[i].HeatScore == hotspots[j].HeatScore {
				return hotspots[i].ID < hotspots[j].ID
			}
			return hotspots[i].HeatScore > hotspots[j].HeatScore
		}
	})
}

func cloneDetail(detail HotspotDetail) HotspotDetail {
	detail.Keywords = append([]string(nil), detail.Keywords...)
	detail.RelatedContent = cloneRelated(detail.RelatedContent)
	detail.Evidence = EvidenceDetail{
		FactEvidenceIDs:   append([]string(nil), detail.Evidence.FactEvidenceIDs...),
		SignalEvidenceIDs: append([]string(nil), detail.Evidence.SignalEvidenceIDs...),
		RiskLabels:        append([]string(nil), detail.Evidence.RiskLabels...),
	}
	return detail
}

func cloneRelated(items []RelatedContent) []RelatedContent {
	return append([]RelatedContent(nil), items...)
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{})
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		cleaned := strings.Join(strings.Fields(value), " ")
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	sort.Strings(normalized)
	return normalized
}
