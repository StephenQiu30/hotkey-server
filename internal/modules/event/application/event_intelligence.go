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

// EventSummaryStore makes the generated result a durable Event fact. Without
// this port the queue job only validates a transient model response and the
// report layer continues to read the old, usually empty events.summary value.
type EventSummaryStore interface {
	PersistSummary(context.Context, int64, domain.EventSummary) error
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
	reader       EventIntelligenceReader
	runner       EventIntelligenceRunner
	summaryStore EventSummaryStore
}

func NewEventSummaryService(reader EventIntelligenceReader, runner EventIntelligenceRunner, stores ...EventSummaryStore) *EventSummaryService {
	service := &EventSummaryService{reader: reader, runner: runner}
	if len(stores) > 0 {
		service.summaryStore = stores[0]
	}
	return service
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
		result := EventSummaryGenerationResult{Summary: summary, ReasonCode: executed.ReasonCode}
		return service.persist(ctx, eventID, result)
	}
	if executed.Status != "succeeded" || executed.Run.ID <= 0 {
		return EventSummaryGenerationResult{}, fmt.Errorf("event summary run did not produce a result")
	}
	summary, err := validatedEventSummary(executed.Result, source.Evidence)
	if err != nil {
		return EventSummaryGenerationResult{}, err
	}
	return service.persist(ctx, eventID, EventSummaryGenerationResult{Summary: summary, RunID: executed.Run.ID, Reused: executed.Reused})
}

func (service *EventSummaryService) persist(ctx context.Context, eventID int64, result EventSummaryGenerationResult) (EventSummaryGenerationResult, error) {
	if service.summaryStore == nil {
		return result, nil
	}
	if err := service.summaryStore.PersistSummary(ctx, eventID, result.Summary); err != nil {
		return EventSummaryGenerationResult{}, fmt.Errorf("persist event summary: %w", err)
	}
	return result, nil
}

// RenderEventSummary is the stable plain-text projection used by the Event
// read model and report snapshots. Evidence metadata remains available through
// the intelligence API; the report receives the verified human-readable facts.
func RenderEventSummary(summary domain.EventSummary) string {
	parts := make([]string, 0, len(summary.Sentences)+2)
	if title := strings.TrimSpace(summary.TitleZH); title != "" {
		parts = append(parts, title)
	}
	if title := strings.TrimSpace(summary.TitleEN); title != "" && title != strings.TrimSpace(summary.TitleZH) {
		parts = append(parts, title)
	}
	for _, sentence := range summary.Sentences {
		if text := strings.TrimSpace(sentence.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
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
