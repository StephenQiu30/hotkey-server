package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

const (
	eventSummaryPromptVersion     = "event-summary-prompt-v1"
	eventSummaryParametersVersion = "event-summary-parameters-v1"
	entityClaimPromptVersion      = "entity-claim-prompt-v1"
	entityClaimParametersVersion  = "entity-claim-parameters-v1"
)

// StructuredExecutor is the narrow Intelligence Application boundary used by
// event intelligence. It intentionally excludes model-profile, provider, and
// persistence implementation details from the calling module.
type StructuredExecutor interface {
	ExecuteStructured(context.Context, StructuredExecutionInput) (StructuredExecutionResult, error)
}

// EventIntelligenceEvidence is the bounded, cited input passed to a
// structured model task. It is not raw Content and cannot carry credentials,
// object keys, or unbounded source payloads.
type EventIntelligenceEvidence struct {
	ContentID int64  `json:"content_id"`
	Locator   string `json:"locator"`
	Excerpt   string `json:"excerpt"`
}

type EventIntelligenceInput struct {
	TaskType domain.TaskType
	EventID  int64
	EventKey string
	Evidence []EventIntelligenceEvidence
}

type EventIntelligenceResult struct {
	Status, ReasonCode string
	Run                domain.Run
	Result             json.RawMessage
	Reused             bool
}

// EventIntelligenceService creates the versioned, evidence-bound run input
// for event_summary and entity_claim_extraction. The RunService remains the
// only component that selects profiles, invokes a provider, repairs schema
// output, and persists an AI run.
type EventIntelligenceService struct{ runs StructuredExecutor }

func NewEventIntelligenceService(runs StructuredExecutor) *EventIntelligenceService {
	return &EventIntelligenceService{runs: runs}
}

func (service *EventIntelligenceService) Execute(ctx context.Context, input EventIntelligenceInput) (EventIntelligenceResult, error) {
	if service == nil || service.runs == nil {
		return EventIntelligenceResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	prepared, err := service.prepare(input)
	if err != nil {
		return EventIntelligenceResult{}, err
	}
	executed, err := service.runs.ExecuteStructured(ctx, prepared)
	if err != nil {
		return EventIntelligenceResult{}, err
	}
	if executed.Status == "degraded" {
		return EventIntelligenceResult{Status: "degraded", ReasonCode: executed.ReasonCode}, nil
	}
	if executed.Status != "succeeded" || executed.Run.ID <= 0 || !json.Valid(executed.Result) {
		return EventIntelligenceResult{}, domain.NewError(domain.CodeAIOutputInvalid)
	}
	return EventIntelligenceResult{Status: "succeeded", Run: executed.Run, Result: cloneRawJSON(executed.Result), Reused: executed.Reused}, nil
}

func (service *EventIntelligenceService) prepare(input EventIntelligenceInput) (StructuredExecutionInput, error) {
	if !eventIntelligenceTask(input.TaskType) || input.EventID <= 0 || strings.TrimSpace(input.EventKey) == "" || len(input.EventKey) > 128 ||
		len(input.Evidence) == 0 || len(input.Evidence) > 64 {
		return StructuredExecutionInput{}, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	evidence := append([]EventIntelligenceEvidence(nil), input.Evidence...)
	for _, item := range evidence {
		if item.ContentID <= 0 || strings.TrimSpace(item.Locator) == "" || utf8.RuneCountInString(item.Locator) > 512 ||
			strings.TrimSpace(item.Excerpt) == "" || utf8.RuneCountInString(item.Excerpt) > 500 {
			return StructuredExecutionInput{}, domain.NewError(domain.CodeAIModelProfileInvalid)
		}
	}
	sort.Slice(evidence, func(left, right int) bool {
		if evidence[left].ContentID != evidence[right].ContentID {
			return evidence[left].ContentID < evidence[right].ContentID
		}
		if evidence[left].Locator != evidence[right].Locator {
			return evidence[left].Locator < evidence[right].Locator
		}
		return evidence[left].Excerpt < evidence[right].Excerpt
	})
	for index := 1; index < len(evidence); index++ {
		if evidence[index].ContentID == evidence[index-1].ContentID && evidence[index].Locator == evidence[index-1].Locator {
			return StructuredExecutionInput{}, domain.NewError(domain.CodeAIModelProfileInvalid)
		}
	}
	payload := struct {
		EventID  int64                       `json:"event_id"`
		EventKey string                      `json:"event_key"`
		Evidence []EventIntelligenceEvidence `json:"evidence"`
	}{EventID: input.EventID, EventKey: strings.TrimSpace(input.EventKey), Evidence: evidence}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return StructuredExecutionInput{}, fmt.Errorf("encode event intelligence input: %w", err)
	}
	evidenceEncoded, err := json.Marshal(evidence)
	if err != nil {
		return StructuredExecutionInput{}, fmt.Errorf("encode event intelligence evidence: %w", err)
	}
	promptVersion, parametersVersion := eventSummaryPromptVersion, eventSummaryParametersVersion
	if input.TaskType == domain.TaskTypeEntityClaimExtraction {
		promptVersion, parametersVersion = entityClaimPromptVersion, entityClaimParametersVersion
	}
	return StructuredExecutionInput{
		TaskType: input.TaskType, TargetType: "event", TargetID: input.EventID,
		PromptVersion: promptVersion, InputSchemaVersion: "v1", SchemaVersion: "v1", ParametersVersion: parametersVersion,
		InputHash: sha256Hex(encoded), EvidenceSetHash: sha256Hex(evidenceEncoded), Input: encoded,
	}, nil
}

func eventIntelligenceTask(taskType domain.TaskType) bool {
	return taskType == domain.TaskTypeEventSummary || taskType == domain.TaskTypeEntityClaimExtraction
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
