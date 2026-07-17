package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

// EventIntelligenceSource is the Event-owned, bounded evidence snapshot used
// to produce a summary. Its Evidence values have already been limited to
// active, non-duplicate Event members by the owning Event adapter.
type EventIntelligenceSource struct {
	Event    domain.Event
	Evidence []domain.EvidenceRef
}

// EventIntelligenceReader keeps event evidence selection in the Event module.
// Intelligence receives only the selected, short evidence values below.
type EventIntelligenceReader interface {
	LoadEventIntelligenceSource(context.Context, int64) (EventIntelligenceSource, error)
}

// EventIntelligenceRunner is the narrow cross-module Application boundary.
// It does not expose provider, model-profile, or persistence dependencies.
type EventIntelligenceRunner interface {
	Execute(context.Context, intelligenceapplication.EventIntelligenceInput) (intelligenceapplication.EventIntelligenceResult, error)
}

type EventSummaryGenerationResult struct {
	Summary    domain.EventSummary
	RunID      int64
	Reused     bool
	ReasonCode string
}

// EventSummaryService validates model output against the exact evidence input
// before returning it. A provider failure produces only a representative,
// fact-only fallback and never exposes provider internals.
type EventSummaryService struct {
	reader EventIntelligenceReader
	runner EventIntelligenceRunner
}

func NewEventSummaryService(reader EventIntelligenceReader, runner EventIntelligenceRunner) *EventSummaryService {
	return &EventSummaryService{reader: reader, runner: runner}
}

func (service *EventSummaryService) Generate(ctx context.Context, eventID int64) (EventSummaryGenerationResult, error) {
	if service == nil || service.reader == nil || service.runner == nil || eventID <= 0 {
		return EventSummaryGenerationResult{}, fmt.Errorf("event summary dependencies are required")
	}
	source, err := service.reader.LoadEventIntelligenceSource(ctx, eventID)
	if err != nil {
		return EventSummaryGenerationResult{}, err
	}
	if err := source.validate(eventID); err != nil {
		return EventSummaryGenerationResult{}, err
	}

	executed, err := service.runner.Execute(ctx, intelligenceapplication.EventIntelligenceInput{
		TaskType: intelligencedomain.TaskTypeEventSummary,
		EventID:  source.Event.ID,
		EventKey: source.Event.EventKey,
		Evidence: eventIntelligenceEvidence(source.Evidence),
	})
	if err != nil {
		return EventSummaryGenerationResult{}, err
	}
	if executed.Status == "degraded" {
		summary, err := DegradedSummary(source.Event.TitleZH, source.Event.TitleEN, source.representativeEvidence())
		if err != nil {
			return EventSummaryGenerationResult{}, err
		}
		return EventSummaryGenerationResult{Summary: summary, ReasonCode: executed.ReasonCode}, nil
	}
	if executed.Status != "succeeded" || executed.Run.ID <= 0 {
		return EventSummaryGenerationResult{}, fmt.Errorf("event summary run did not produce a result")
	}
	summary, err := validatedEventSummary(executed.Result, source.Evidence)
	if err != nil {
		return EventSummaryGenerationResult{}, err
	}
	return EventSummaryGenerationResult{Summary: summary, RunID: executed.Run.ID, Reused: executed.Reused}, nil
}

func (source EventIntelligenceSource) validate(eventID int64) error {
	if source.Event.ID != eventID || strings.TrimSpace(source.Event.EventKey) == "" || strings.TrimSpace(source.Event.TitleZH) == "" || len(source.Evidence) == 0 || len(source.Evidence) > 64 {
		return fmt.Errorf("invalid event intelligence source")
	}
	seen := make(map[string]bool, len(source.Evidence))
	for _, evidence := range source.Evidence {
		if err := evidence.Validate(); err != nil || strings.TrimSpace(evidence.Excerpt) == "" {
			return fmt.Errorf("invalid event intelligence evidence")
		}
		key := evidenceKey(evidence.ContentID, evidence.Locator)
		if seen[key] {
			return fmt.Errorf("duplicate event intelligence evidence")
		}
		seen[key] = true
	}
	return nil
}

func (source EventIntelligenceSource) representativeEvidence() domain.EvidenceRef {
	if source.Event.RepresentativeContentID != nil {
		for _, evidence := range source.Evidence {
			if evidence.ContentID == *source.Event.RepresentativeContentID {
				return evidence
			}
		}
	}
	return source.Evidence[0]
}

func eventIntelligenceEvidence(evidence []domain.EvidenceRef) []intelligenceapplication.EventIntelligenceEvidence {
	result := make([]intelligenceapplication.EventIntelligenceEvidence, 0, len(evidence))
	for _, item := range evidence {
		result = append(result, intelligenceapplication.EventIntelligenceEvidence{ContentID: item.ContentID, Locator: item.Locator, Excerpt: item.Excerpt})
	}
	return result
}

func validatedEventSummary(raw json.RawMessage, evidence []domain.EvidenceRef) (domain.EventSummary, error) {
	var output eventSummaryOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return domain.EventSummary{}, fmt.Errorf("decode event summary output: %w", err)
	}
	available := make(map[string]domain.EvidenceRef, len(evidence))
	activeContentIDs := make(map[int64]bool, len(evidence))
	for _, item := range evidence {
		available[evidenceKey(item.ContentID, item.Locator)] = item
		activeContentIDs[item.ContentID] = true
	}
	summary := domain.EventSummary{Version: "event-summary-v1", TitleZH: output.TitleZH, TitleEN: output.TitleEN, Sentences: make([]domain.EvidenceSentence, 0, len(output.Sentences))}
	for _, sentence := range output.Sentences {
		verified := domain.EvidenceSentence{Text: sentence.Text, Evidence: make([]domain.EvidenceRef, 0, len(sentence.Evidence))}
		for _, citation := range sentence.Evidence {
			item, found := available[evidenceKey(citation.ContentID, citation.Locator)]
			if !found {
				return domain.EventSummary{}, fmt.Errorf("summary cites evidence outside the input snapshot")
			}
			// Take locator and excerpt from the trusted snapshot rather than the
			// model response, so a response cannot forge citation metadata.
			verified.Evidence = append(verified.Evidence, item)
		}
		summary.Sentences = append(summary.Sentences, verified)
	}
	if err := NewEvidenceValidator().ValidateSummary(summary, activeContentIDs); err != nil {
		return domain.EventSummary{}, err
	}
	return summary, nil
}

type eventSummaryOutput struct {
	TitleZH   string                       `json:"title_zh"`
	TitleEN   string                       `json:"title_en"`
	Sentences []eventSummaryOutputSentence `json:"sentences"`
}

type eventSummaryOutputSentence struct {
	Text     string                       `json:"text"`
	Evidence []eventSummaryOutputEvidence `json:"evidence"`
}

type eventSummaryOutputEvidence struct {
	ContentID int64  `json:"content_id"`
	Locator   string `json:"locator"`
}

func evidenceKey(contentID int64, locator string) string {
	return fmt.Sprintf("%d:%s", contentID, locator)
}
