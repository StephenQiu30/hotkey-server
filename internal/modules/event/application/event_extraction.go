package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

type ExtractedEventEntity struct {
	Entity                                 domain.Entity
	Alias, NormalizedAlias, Language, Role string
	Confidence                             float64
}

type ExtractedEventFacts struct {
	EventID  int64
	Entities []ExtractedEventEntity
	Claims   []domain.Claim
}

func (facts ExtractedEventFacts) Validate() error {
	if facts.EventID <= 0 || len(facts.Entities) > 64 || len(facts.Claims) > 64 {
		return fmt.Errorf("invalid extracted event facts")
	}
	entityKeys := make(map[string]bool, len(facts.Entities))
	for _, item := range facts.Entities {
		if err := item.Entity.Validate(); err != nil || strings.TrimSpace(item.Alias) == "" || len(item.Alias) > 255 || strings.TrimSpace(item.NormalizedAlias) == "" || len(item.NormalizedAlias) > 255 || strings.TrimSpace(item.Language) == "" || len(item.Language) > 16 {
			return fmt.Errorf("invalid extracted entity")
		}
		if err := (domain.EventEntity{EventID: facts.EventID, EntityID: 1, Role: item.Role, Confidence: item.Confidence, Origin: domain.FactOriginModel}).Validate(); err != nil {
			return err
		}
		if entityKeys[item.Entity.Key] {
			return fmt.Errorf("duplicate extracted entity")
		}
		entityKeys[item.Entity.Key] = true
	}
	claimHashes := make(map[string]bool, len(facts.Claims))
	for _, claim := range facts.Claims {
		if claim.EventID != facts.EventID || claimHashes[claim.ClaimHash] {
			return fmt.Errorf("duplicate or invalid extracted claim")
		}
		if err := claim.Validate(); err != nil {
			return err
		}
		claimHashes[claim.ClaimHash] = true
	}
	return nil
}

type PersistedEventFacts struct {
	Entities []domain.Entity
	Claims   []domain.Claim
}

type EventIntelligenceFactStore interface {
	PersistExtractedFacts(context.Context, ExtractedEventFacts) (PersistedEventFacts, error)
}

type EventClaimExtractionResult struct {
	Status, ReasonCode string
	RunID              int64
	Reused             bool
	Facts              PersistedEventFacts
}

// EventClaimExtractionService converts a safe structured extraction result
// into Event-owned candidate facts. It never accepts model-provided claim
// status, evidence metadata, or direct database access.
type EventClaimExtractionService struct {
	reader EventIntelligenceReader
	runner EventIntelligenceRunner
	store  EventIntelligenceFactStore
}

func NewEventClaimExtractionService(reader EventIntelligenceReader, runner EventIntelligenceRunner, store EventIntelligenceFactStore) *EventClaimExtractionService {
	return &EventClaimExtractionService{reader: reader, runner: runner, store: store}
}

func (service *EventClaimExtractionService) Extract(ctx context.Context, eventID int64) (EventClaimExtractionResult, error) {
	if service == nil || service.reader == nil || service.runner == nil || service.store == nil || eventID <= 0 {
		return EventClaimExtractionResult{}, fmt.Errorf("event claim extraction dependencies are required")
	}
	source, err := service.reader.LoadEventIntelligenceSource(ctx, eventID)
	if err != nil {
		return EventClaimExtractionResult{}, err
	}
	if err := source.validate(eventID); err != nil {
		return EventClaimExtractionResult{}, err
	}
	executed, err := service.runner.Execute(ctx, intelligenceapplication.EventIntelligenceInput{
		TaskType: intelligencedomain.TaskTypeEntityClaimExtraction,
		EventID:  source.Event.ID,
		EventKey: source.Event.EventKey,
		Evidence: eventIntelligenceEvidence(source.Evidence),
	})
	if err != nil {
		return EventClaimExtractionResult{}, err
	}
	if executed.Status == "degraded" {
		return EventClaimExtractionResult{Status: "degraded", ReasonCode: executed.ReasonCode}, nil
	}
	if executed.Status != "succeeded" || executed.Run.ID <= 0 {
		return EventClaimExtractionResult{}, fmt.Errorf("entity claim extraction run did not produce a result")
	}
	facts, err := extractedEventFacts(source.Event.ID, executed.Result, source.Evidence)
	if err != nil {
		return EventClaimExtractionResult{}, err
	}
	persisted, err := service.store.PersistExtractedFacts(ctx, facts)
	if err != nil {
		return EventClaimExtractionResult{}, err
	}
	return EventClaimExtractionResult{Status: "succeeded", RunID: executed.Run.ID, Reused: executed.Reused, Facts: persisted}, nil
}

func extractedEventFacts(eventID int64, raw json.RawMessage, evidence []domain.EvidenceRef) (ExtractedEventFacts, error) {
	var output entityClaimExtractionOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return ExtractedEventFacts{}, fmt.Errorf("decode entity claim extraction output: %w", err)
	}
	available := make(map[string]domain.EvidenceRef, len(evidence))
	for _, item := range evidence {
		available[evidenceKey(item.ContentID, item.Locator)] = item
	}
	facts := ExtractedEventFacts{EventID: eventID, Entities: make([]ExtractedEventEntity, 0, len(output.Entities)), Claims: make([]domain.Claim, 0, len(output.Claims))}
	for _, item := range output.Entities {
		name := strings.TrimSpace(item.CanonicalName)
		facts.Entities = append(facts.Entities, ExtractedEventEntity{
			Entity:          domain.Entity{ID: 1, Version: 1, Key: strings.TrimSpace(item.Key), Name: name, Type: domain.EntityType(item.Type)},
			Alias:           name,
			NormalizedAlias: normalizedFactText(name),
			Language:        "und",
			Role:            "mentioned",
			Confidence:      50,
		})
	}
	for _, item := range output.Claims {
		claimText := strings.TrimSpace(item.Claim)
		claim := domain.Claim{ID: 1, Version: 1, EventID: eventID, NormalizedClaim: normalizedFactText(claimText), ClaimHash: factHash(normalizedFactText(claimText)), Confidence: 0, Evidence: make([]domain.ClaimEvidence, 0, len(item.Evidence))}
		for _, citation := range item.Evidence {
			trusted, found := available[evidenceKey(citation.ContentID, citation.Locator)]
			if !found {
				return ExtractedEventFacts{}, fmt.Errorf("claim cites evidence outside the input snapshot")
			}
			stance := citation.Stance
			if stance == "" {
				stance = "mentions"
			}
			claim.Evidence = append(claim.Evidence, domain.ClaimEvidence{EvidenceRef: trusted, Stance: stance, Confidence: citation.Confidence})
		}
		claim.Status, claim.Confidence = inferredClaimStatus(claim.Evidence)
		facts.Claims = append(facts.Claims, claim)
	}
	if err := facts.Validate(); err != nil {
		return ExtractedEventFacts{}, err
	}
	return facts, nil
}

func normalizedFactText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func factHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func inferredClaimStatus(evidence []domain.ClaimEvidence) (domain.ClaimStatus, float64) {
	contentIDs := make(map[int64]bool, len(evidence))
	var supports, contradicts, total float64
	for _, item := range evidence {
		contentIDs[item.ContentID] = true
		total += item.Confidence
		switch item.Stance {
		case "supports":
			supports++
		case "contradicts":
			contradicts++
		}
	}
	confidence := 0.0
	if len(evidence) > 0 {
		confidence = total / float64(len(evidence))
	}
	switch {
	case supports > 0 && contradicts > 0:
		return domain.ClaimDisputed, confidence
	case len(contentIDs) >= 2 && supports >= 2:
		return domain.ClaimCorroborated, confidence
	case len(contentIDs) == 1 && supports == 1:
		return domain.ClaimSingleSource, confidence
	default:
		return domain.ClaimUnverified, confidence
	}
}

type entityClaimExtractionOutput struct {
	Entities []entityClaimOutputEntity `json:"entities"`
	Claims   []entityClaimOutputClaim  `json:"claims"`
}

type entityClaimOutputEntity struct {
	Key           string `json:"entity_key"`
	Type          string `json:"entity_type"`
	CanonicalName string `json:"canonical_name"`
}

type entityClaimOutputClaim struct {
	Claim    string                      `json:"claim"`
	Evidence []entityClaimOutputEvidence `json:"evidence"`
}

type entityClaimOutputEvidence struct {
	ContentID  int64   `json:"content_id"`
	Locator    string  `json:"locator"`
	Stance     string  `json:"stance"`
	Confidence float64 `json:"confidence"`
}
