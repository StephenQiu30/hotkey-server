package trust

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

const (
	EvidenceLayerFact   = "fact"
	EvidenceLayerSignal = "signal"

	TrustLevelHigh   = "high"
	TrustLevelMedium = "medium"
	TrustLevelLow    = "low"

	TrustLabelVerified   = "verified"
	TrustLabelPartial    = "partial"
	TrustLabelUnverified = "unverified"
)

var (
	ErrInvalidEvidence = errors.New("invalid evidence")
	ErrMissingCitation = errors.New("missing citation")
)

type EvidenceInput struct {
	EventID      string `json:"eventId"`
	SourceID     string `json:"sourceId"`
	SourceItemID string `json:"sourceItemId"`
	Layer        string `json:"layer"`
	TrustLevel   string `json:"trustLevel"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	HeatWeight   int    `json:"heatWeight"`
	RiskNote     string `json:"riskNote"`
}

type Evidence struct {
	EventID      string `json:"eventId"`
	SourceID     string `json:"sourceId"`
	SourceItemID string `json:"sourceItemId"`
	Layer        string `json:"layer"`
	TrustLevel   string `json:"trustLevel"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	HeatWeight   int    `json:"heatWeight"`
	RiskNote     string `json:"riskNote"`
}

type AISummaryInput struct {
	Summary     string   `json:"summary"`
	CitationIDs []string `json:"citationIds"`
}

type AISummary struct {
	Summary     string   `json:"summary"`
	CitationIDs []string `json:"citationIds"`
}

type EventEvidenceDetail struct {
	EventID        string     `json:"eventId"`
	FactEvidence   []Evidence `json:"factEvidence"`
	SignalEvidence []Evidence `json:"signalEvidence"`
	FactScore      int        `json:"factScore"`
	HeatScore      int        `json:"heatScore"`
	TrustLabel     string     `json:"trustLabel"`
	RiskNotes      []string   `json:"riskNotes"`
	AISummary      *AISummary `json:"aiSummary,omitempty"`
}

type Service struct {
	mu        sync.Mutex
	evidence  map[string][]Evidence
	summaries map[string]AISummary
}

func NewService() *Service {
	return &Service{
		evidence:  make(map[string][]Evidence),
		summaries: make(map[string]AISummary),
	}
}

func (s *Service) AddEvidence(input EvidenceInput) error {
	evidence, err := normalizeEvidence(input)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[evidence.EventID] = append(s.evidence[evidence.EventID], evidence)
	return nil
}

func (s *Service) SetAISummary(eventID string, input AISummaryInput) error {
	eventID = strings.TrimSpace(eventID)
	summary := strings.TrimSpace(input.Summary)
	citations := normalizeCitations(input.CitationIDs)
	if eventID == "" || summary == "" {
		return ErrInvalidEvidence
	}
	if len(citations) == 0 {
		return ErrMissingCitation
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.summaries[eventID] = AISummary{
		Summary:     summary,
		CitationIDs: citations,
	}
	return nil
}

func (s *Service) GetEventEvidence(eventID string) EventEvidenceDetail {
	s.mu.Lock()
	defer s.mu.Unlock()

	detail := EventEvidenceDetail{
		EventID:    eventID,
		TrustLabel: TrustLabelUnverified,
	}
	for _, evidence := range s.evidence[eventID] {
		if evidence.Layer == EvidenceLayerFact {
			detail.FactEvidence = append(detail.FactEvidence, evidence)
			detail.FactScore += trustScore(evidence.TrustLevel)
		}
		if evidence.Layer == EvidenceLayerSignal {
			detail.SignalEvidence = append(detail.SignalEvidence, evidence)
			detail.HeatScore += evidence.HeatWeight
		}
		if evidence.RiskNote != "" {
			detail.RiskNotes = append(detail.RiskNotes, evidence.RiskNote)
		}
	}
	sortEvidence(detail.FactEvidence)
	sortEvidence(detail.SignalEvidence)
	sort.Strings(detail.RiskNotes)
	detail.TrustLabel = labelForFactScore(detail.FactScore)
	if summary, ok := s.summaries[eventID]; ok {
		copied := summary
		copied.CitationIDs = append([]string(nil), summary.CitationIDs...)
		detail.AISummary = &copied
	}
	return detail
}

func normalizeEvidence(input EvidenceInput) (Evidence, error) {
	evidence := Evidence{
		EventID:      strings.TrimSpace(input.EventID),
		SourceID:     strings.TrimSpace(input.SourceID),
		SourceItemID: strings.TrimSpace(input.SourceItemID),
		Layer:        strings.TrimSpace(input.Layer),
		TrustLevel:   strings.TrimSpace(input.TrustLevel),
		Title:        strings.Join(strings.Fields(input.Title), " "),
		URL:          strings.TrimSpace(input.URL),
		HeatWeight:   input.HeatWeight,
		RiskNote:     strings.Join(strings.Fields(input.RiskNote), " "),
	}
	if evidence.EventID == "" || evidence.SourceID == "" || evidence.SourceItemID == "" || evidence.Title == "" || evidence.URL == "" {
		return Evidence{}, ErrInvalidEvidence
	}
	if evidence.Layer != EvidenceLayerFact && evidence.Layer != EvidenceLayerSignal {
		return Evidence{}, ErrInvalidEvidence
	}
	if evidence.TrustLevel != TrustLevelHigh && evidence.TrustLevel != TrustLevelMedium && evidence.TrustLevel != TrustLevelLow {
		return Evidence{}, ErrInvalidEvidence
	}
	if evidence.Layer == EvidenceLayerFact && evidence.TrustLevel == TrustLevelLow {
		return Evidence{}, ErrInvalidEvidence
	}
	if evidence.Layer == EvidenceLayerSignal && evidence.HeatWeight < 0 {
		return Evidence{}, ErrInvalidEvidence
	}
	return evidence, nil
}

func trustScore(level string) int {
	switch level {
	case TrustLevelHigh:
		return 100
	case TrustLevelMedium:
		return 60
	default:
		return 0
	}
}

func labelForFactScore(score int) string {
	if score >= 100 {
		return TrustLabelVerified
	}
	if score > 0 {
		return TrustLabelPartial
	}
	return TrustLabelUnverified
}

func normalizeCitations(citations []string) []string {
	seen := make(map[string]struct{})
	normalized := make([]string, 0, len(citations))
	for _, citation := range citations {
		value := strings.TrimSpace(citation)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func sortEvidence(items []Evidence) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SourceItemID == items[j].SourceItemID {
			return items[i].SourceID < items[j].SourceID
		}
		return items[i].SourceItemID < items[j].SourceItemID
	})
}
