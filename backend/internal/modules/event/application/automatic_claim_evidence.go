package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	AtomicClaimEvidencePromptVersion     = "atomic-claim-evidence-prompt-v2"
	AtomicClaimEvidenceParametersVersion = "atomic-claim-evidence-parameters-v2"
	AutomaticEvidenceSummaryProfile      = "claim-evidence-summary-v1"
	maximumAtomicClaimEvidenceBodyBytes  = 100000
)

var ErrInvalidAutomaticClaimEvidenceContract = errors.New("automatic claim evidence contract is invalid")

type AutomaticClaimEvidenceCommand struct {
	MicroEventID, ExpectedEventVersion, DocumentVersionID int64
}

// ScheduleAutomaticClaimEvidenceCommand contains only durable identities. The
// consumer resolves the current event version when it starts so a later
// membership update cannot make an otherwise valid evidence job stale.
type ScheduleAutomaticClaimEvidenceCommand struct {
	MicroEventID, DocumentVersionID int64
}

type ScheduleAutomaticClaimEvidenceResult struct {
	MicroEventID, DocumentVersionID int64
	JobID                           int64
	Created                         bool
}

type AutomaticClaimEvidenceScheduler interface {
	ScheduleAutomaticClaimEvidence(context.Context, ScheduleAutomaticClaimEvidenceCommand) (ScheduleAutomaticClaimEvidenceResult, error)
}

type AutomaticClaimEvidenceTargetQuery struct {
	MicroEventID, ExpectedEventVersion, DocumentVersionID int64
	DecisionAt                                            time.Time
}

type AutomaticClaimEvidenceArtifactDTO struct {
	DocumentID, DocumentVersionID int64
	ArtifactType                  string
	TransformerProfileSHA256      string
	MIMEType, PlaintextSHA256     string
	SizeBytes                     int64
	RetentionUntil                time.Time
}

type AutomaticClaimEvidenceTargetDTO struct {
	MicroEventID, EventVersion int64
	EventKey                   string
	Artifact                   AutomaticClaimEvidenceArtifactDTO
	ExternalModelAllowed       bool
	DecisionAt                 time.Time
}

type AutomaticClaimEvidenceProjectionQuery struct {
	Artifact AutomaticClaimEvidenceArtifactDTO
	MaxBytes int64
}

type AutomaticClaimEvidenceProjectionDTO struct {
	Plaintext, MIMEType, SHA256 string
	SizeBytes                   int64
}

type LocateAutomaticQuoteSelectorCommand struct {
	DocumentVersionID    int64
	ExactQuote           string
	PlaintextSHA256      string
	NormalizationVersion string
	DecisionAt           time.Time
}

type LocatedAutomaticQuoteSelectorDTO struct {
	ID, Version, DocumentVersionID int64
	ExactQuote, PlaintextSHA256    string
}

type AutomaticClaimEvidenceTargetReader interface {
	ReadAutomaticClaimEvidenceTarget(context.Context, AutomaticClaimEvidenceTargetQuery) (AutomaticClaimEvidenceTargetDTO, error)
}

type AutomaticClaimEvidenceProjectionReader interface {
	ReadAutomaticClaimEvidenceProjection(context.Context, AutomaticClaimEvidenceProjectionQuery) (AutomaticClaimEvidenceProjectionDTO, error)
}

type AutomaticQuoteSelectorLocator interface {
	LocateAutomaticQuoteSelector(context.Context, LocateAutomaticQuoteSelectorCommand) (LocatedAutomaticQuoteSelectorDTO, error)
}

type AutomaticEvidenceQualityProfileReader interface {
	IsDecisionQualityProfileActive(context.Context, string, string) (bool, error)
}

type AutomaticClaimEvidenceDependencies struct {
	Targets         AutomaticClaimEvidenceTargetReader
	Projections     AutomaticClaimEvidenceProjectionReader
	Models          intelligenceapplication.StructuredExecutor
	Selectors       AutomaticQuoteSelectorLocator
	Evidence        *ClaimEvidenceService
	Summaries       *EvidenceSummaryService
	Clock           sharedclock.Clock
	QualityProfiles AutomaticEvidenceQualityProfileReader
}

type AutomaticClaimEvidenceResult struct {
	Status, ReasonCode string
	ModelRunID         int64
	Evidence           []ClaimEvidenceVersionDTO
	EvidenceState      *EvidenceStateSnapshotDTO
	Summary            *EvidenceSummaryDTO
	ReusedModelRun     bool
}

type AutomaticClaimEvidenceService struct {
	targets         AutomaticClaimEvidenceTargetReader
	projections     AutomaticClaimEvidenceProjectionReader
	models          intelligenceapplication.StructuredExecutor
	selectors       AutomaticQuoteSelectorLocator
	evidence        *ClaimEvidenceService
	summaries       *EvidenceSummaryService
	clock           sharedclock.Clock
	qualityProfiles AutomaticEvidenceQualityProfileReader
}

func NewAutomaticClaimEvidenceService(dependencies AutomaticClaimEvidenceDependencies) (*AutomaticClaimEvidenceService, error) {
	if dependencies.Targets == nil || dependencies.Projections == nil || dependencies.Models == nil || dependencies.Selectors == nil ||
		dependencies.Evidence == nil || dependencies.Summaries == nil {
		return nil, fmt.Errorf("%w: dependencies are required", ErrInvalidAutomaticClaimEvidenceContract)
	}
	if dependencies.Clock == nil {
		dependencies.Clock = sharedclock.System{}
	}
	return &AutomaticClaimEvidenceService{targets: dependencies.Targets, projections: dependencies.Projections,
		models: dependencies.Models, selectors: dependencies.Selectors, evidence: dependencies.Evidence,
		summaries: dependencies.Summaries, clock: dependencies.Clock, qualityProfiles: dependencies.QualityProfiles}, nil
}

func (service *AutomaticClaimEvidenceService) Extract(ctx context.Context, command AutomaticClaimEvidenceCommand) (AutomaticClaimEvidenceResult, error) {
	if service == nil || service.targets == nil || service.projections == nil || service.models == nil || service.selectors == nil ||
		service.evidence == nil || service.summaries == nil || service.clock == nil || command.MicroEventID <= 0 ||
		command.ExpectedEventVersion < 0 || command.DocumentVersionID <= 0 {
		return AutomaticClaimEvidenceResult{}, ErrInvalidAutomaticClaimEvidenceContract
	}
	decisionAt := service.clock.Now().UTC()
	target, err := service.targets.ReadAutomaticClaimEvidenceTarget(ctx, AutomaticClaimEvidenceTargetQuery{
		MicroEventID: command.MicroEventID, ExpectedEventVersion: command.ExpectedEventVersion,
		DocumentVersionID: command.DocumentVersionID, DecisionAt: decisionAt,
	})
	if err != nil {
		return AutomaticClaimEvidenceResult{}, fmt.Errorf("read automatic claim evidence target: %w", err)
	}
	if command.ExpectedEventVersion == 0 {
		command.ExpectedEventVersion = target.EventVersion
	}
	if !automaticClaimEvidenceTargetMatches(target, command, decisionAt) {
		return AutomaticClaimEvidenceResult{}, ErrInvalidAutomaticClaimEvidenceContract
	}
	if service.qualityProfiles != nil {
		active, readErr := service.qualityProfiles.IsDecisionQualityProfileActive(ctx, "evidence_relation", "claim-evidence-relation-v1")
		if readErr != nil || !active {
			return AutomaticClaimEvidenceResult{Status: "degraded", ReasonCode: "quality_profile_not_active"}, nil
		}
	}
	if !target.ExternalModelAllowed {
		return AutomaticClaimEvidenceResult{Status: "degraded", ReasonCode: "external_model_not_authorized"}, nil
	}
	projection, err := service.projections.ReadAutomaticClaimEvidenceProjection(ctx, AutomaticClaimEvidenceProjectionQuery{
		Artifact: target.Artifact, MaxBytes: target.Artifact.SizeBytes,
	})
	if err != nil {
		return AutomaticClaimEvidenceResult{}, fmt.Errorf("read authorized automatic claim evidence plaintext: %w", err)
	}
	if !automaticClaimEvidenceProjectionMatches(projection, target.Artifact) {
		return AutomaticClaimEvidenceResult{}, fmt.Errorf("%w: plaintext projection receipt changed", sharedrepository.ErrConflict)
	}
	body, truncated := boundedAtomicClaimEvidenceBody(projection.Plaintext, maximumAtomicClaimEvidenceBodyBytes)
	input := automaticClaimEvidenceModelInput{EventID: target.MicroEventID, EventVersion: target.EventVersion,
		EventKey: target.EventKey, DocumentVersionID: target.Artifact.DocumentVersionID,
		PlaintextSHA256: target.Artifact.PlaintextSHA256, Body: body, BodyTruncated: truncated}
	encoded, err := json.Marshal(input)
	if err != nil {
		return AutomaticClaimEvidenceResult{}, fmt.Errorf("encode automatic claim evidence input: %w", err)
	}
	// Re-read the exact rights-bound target immediately before the external
	// model boundary. A revocation between body read and dispatch fails closed.
	rechecked, err := service.targets.ReadAutomaticClaimEvidenceTarget(ctx, AutomaticClaimEvidenceTargetQuery{
		MicroEventID: command.MicroEventID, ExpectedEventVersion: command.ExpectedEventVersion,
		DocumentVersionID: command.DocumentVersionID, DecisionAt: service.clock.Now().UTC(),
	})
	if err != nil || !automaticClaimEvidenceTargetEquivalent(target, rechecked) || !rechecked.ExternalModelAllowed {
		return AutomaticClaimEvidenceResult{}, fmt.Errorf("%w: external-model authorization changed", sharedrepository.ErrConflict)
	}
	inputDigest := sha256.Sum256(encoded)
	evidenceDigest := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%s", target.MicroEventID, target.Artifact.DocumentVersionID, target.Artifact.PlaintextSHA256)))
	executed, err := service.models.ExecuteStructured(ctx, intelligenceapplication.StructuredExecutionInput{
		TaskType: intelligencedomain.TaskTypeEntityClaimExtraction, TargetType: "event", TargetID: target.MicroEventID,
		PromptVersion: AtomicClaimEvidencePromptVersion, InputSchemaVersion: "v2", SchemaVersion: "v2",
		ParametersVersion: AtomicClaimEvidenceParametersVersion, InputHash: hex.EncodeToString(inputDigest[:]),
		EvidenceSetHash: hex.EncodeToString(evidenceDigest[:]), Input: encoded,
	})
	if err != nil {
		return AutomaticClaimEvidenceResult{}, fmt.Errorf("execute automatic claim evidence model: %w", err)
	}
	if executed.Status == "degraded" {
		return AutomaticClaimEvidenceResult{Status: "degraded", ReasonCode: executed.ReasonCode}, nil
	}
	if executed.Status != "succeeded" || executed.Run.ID <= 0 || !json.Valid(executed.Result) {
		return AutomaticClaimEvidenceResult{}, ErrInvalidAutomaticClaimEvidenceContract
	}
	output, err := decodeAutomaticClaimEvidenceOutput(executed.Result)
	if err != nil {
		return AutomaticClaimEvidenceResult{}, err
	}
	result := AutomaticClaimEvidenceResult{Status: "succeeded", ModelRunID: executed.Run.ID,
		ReusedModelRun: executed.Reused, Evidence: make([]ClaimEvidenceVersionDTO, 0, len(output.Claims))}
	for index, claim := range output.Claims {
		selector, locateErr := service.selectors.LocateAutomaticQuoteSelector(ctx, LocateAutomaticQuoteSelectorCommand{
			DocumentVersionID: target.Artifact.DocumentVersionID, ExactQuote: claim.ExactQuote,
			PlaintextSHA256:      target.Artifact.PlaintextSHA256,
			NormalizationVersion: "nfc-lf-collapse-space-v1", DecisionAt: decisionAt,
		})
		if locateErr != nil {
			return result, fmt.Errorf("locate automatic claim quote %d: %w", index, locateErr)
		}
		if selector.ID <= 0 || selector.DocumentVersionID != target.Artifact.DocumentVersionID ||
			selector.ExactQuote != claim.ExactQuote || selector.PlaintextSHA256 != target.Artifact.PlaintextSHA256 {
			return result, ErrInvalidAutomaticClaimEvidenceContract
		}
		modelRunID, relationScore := executed.Run.ID, claim.RelationScore
		recorded, recordErr := service.evidence.Record(ctx, RecordClaimEvidenceCommand{
			MicroEventID: target.MicroEventID, ExpectedEventVersion: target.EventVersion,
			DocumentVersionID: target.Artifact.DocumentVersionID, TextQuoteSelectorID: selector.ID,
			Subject: claim.Subject, Predicate: claim.Predicate, Object: claim.Object, Qualifiers: claim.Qualifiers,
			Relation: claim.Relation, ModelRunID: &modelRunID, ModelRelationScore: &relationScore,
			ExtractionSchemaVersion: CanonicalClaimExtractionSchemaVersion, Origin: "automatic",
			IdempotencyKey: fmt.Sprintf("automatic-claim-%d-%d", executed.Run.ID, index), DecisionAt: decisionAt,
		})
		if recordErr != nil {
			return result, fmt.Errorf("record automatic claim evidence %d: %w", index, recordErr)
		}
		result.Evidence = append(result.Evidence, recorded.Evidence)
	}
	state, err := service.evidence.CalculateState(ctx, CalculateEvidenceStateCommand{MicroEventID: target.MicroEventID,
		ExpectedEventVersion: target.EventVersion, AlgorithmVersion: CanonicalEvidenceStateAlgorithmVersion,
		CalculatedAt: decisionAt})
	if err != nil {
		return result, fmt.Errorf("calculate automatic evidence state: %w", err)
	}
	result.EvidenceState = &state.Snapshot
	sentences := make([]EvidenceSummarySentenceInputDTO, 0, len(result.Evidence))
	for index, evidence := range result.Evidence {
		claim := output.Claims[index]
		runID := executed.Run.ID
		sentences = append(sentences, EvidenceSummarySentenceInputDTO{Text: strings.TrimSpace(strings.Join([]string{claim.Subject, claim.Predicate, claim.Object}, " ")),
			ClaimEvidenceVersionIDs: []int64{evidence.ID}, DecisionOrigin: "automatic", ModelRunID: &runID})
	}
	published, err := service.summaries.Publish(ctx, PublishEvidenceSummaryCommand{MicroEventID: target.MicroEventID,
		ExpectedEventVersion: target.EventVersion, SummaryProfileVersion: AutomaticEvidenceSummaryProfile,
		Sentences: sentences, IdempotencyKey: fmt.Sprintf("automatic-summary-%d-v%d-run-%d", target.MicroEventID, target.EventVersion, executed.Run.ID),
		CreatedAt: decisionAt})
	if err != nil {
		return result, fmt.Errorf("publish automatic evidence summary: %w", err)
	}
	result.Summary = &published.Summary
	return result, nil
}

type automaticClaimEvidenceModelInput struct {
	EventID, EventVersion, DocumentVersionID int64
	EventKey, PlaintextSHA256, Body          string
	BodyTruncated                            bool
}

func (input automaticClaimEvidenceModelInput) MarshalJSON() ([]byte, error) {
	type wire struct {
		EventID           int64  `json:"event_id"`
		EventVersion      int64  `json:"event_version"`
		EventKey          string `json:"event_key"`
		DocumentVersionID int64  `json:"document_version_id"`
		PlaintextSHA256   string `json:"plaintext_sha256"`
		Body              string `json:"body"`
		BodyTruncated     bool   `json:"body_truncated"`
	}
	return json.Marshal(wire{EventID: input.EventID, EventVersion: input.EventVersion, EventKey: input.EventKey,
		DocumentVersionID: input.DocumentVersionID, PlaintextSHA256: input.PlaintextSHA256,
		Body: input.Body, BodyTruncated: input.BodyTruncated})
}

type automaticClaimEvidenceOutput struct {
	Claims []automaticClaimEvidenceOutputClaim `json:"claims"`
}

type automaticClaimEvidenceOutputClaim struct {
	Subject, Predicate, Object, Relation, ExactQuote string
	RelationScore                                    float64
	Qualifiers                                       []ClaimQualifierDTO
}

func (claim *automaticClaimEvidenceOutputClaim) UnmarshalJSON(encoded []byte) error {
	var wire struct {
		Subject       string              `json:"subject"`
		Predicate     string              `json:"predicate"`
		Object        string              `json:"object"`
		Relation      string              `json:"relation"`
		ExactQuote    string              `json:"exact_quote"`
		RelationScore float64             `json:"relation_score"`
		Qualifiers    []ClaimQualifierDTO `json:"qualifiers"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		return err
	}
	*claim = automaticClaimEvidenceOutputClaim{Subject: wire.Subject, Predicate: wire.Predicate, Object: wire.Object,
		Relation: wire.Relation, ExactQuote: wire.ExactQuote, RelationScore: wire.RelationScore, Qualifiers: wire.Qualifiers}
	return nil
}

func decodeAutomaticClaimEvidenceOutput(encoded []byte) (automaticClaimEvidenceOutput, error) {
	var output automaticClaimEvidenceOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		return automaticClaimEvidenceOutput{}, fmt.Errorf("decode automatic claim evidence output: %w", err)
	}
	if len(output.Claims) == 0 || len(output.Claims) > 32 {
		return automaticClaimEvidenceOutput{}, ErrInvalidAutomaticClaimEvidenceContract
	}
	for index := range output.Claims {
		claim := &output.Claims[index]
		claim.Subject, claim.Predicate, claim.Object = canonicalClaimPart(claim.Subject), canonicalClaimPart(claim.Predicate), canonicalClaimPart(claim.Object)
		claim.Relation, claim.ExactQuote = strings.TrimSpace(claim.Relation), strings.TrimSpace(claim.ExactQuote)
		if claim.Subject == "" || claim.Predicate == "" || claim.Object == "" || claim.ExactQuote == "" ||
			claim.RelationScore < 0 || claim.RelationScore > 1 || len(claim.Qualifiers) > 16 {
			return automaticClaimEvidenceOutput{}, ErrInvalidAutomaticClaimEvidenceContract
		}
		for qualifierIndex := range claim.Qualifiers {
			claim.Qualifiers[qualifierIndex].Key = canonicalClaimPart(claim.Qualifiers[qualifierIndex].Key)
			claim.Qualifiers[qualifierIndex].Value = canonicalClaimPart(claim.Qualifiers[qualifierIndex].Value)
			if claim.Qualifiers[qualifierIndex].Key == "" || claim.Qualifiers[qualifierIndex].Value == "" {
				return automaticClaimEvidenceOutput{}, ErrInvalidAutomaticClaimEvidenceContract
			}
		}
	}
	return output, nil
}

func automaticClaimEvidenceTargetMatches(target AutomaticClaimEvidenceTargetDTO, command AutomaticClaimEvidenceCommand, decisionAt time.Time) bool {
	artifact := target.Artifact
	return target.MicroEventID == command.MicroEventID && target.EventVersion == command.ExpectedEventVersion &&
		strings.TrimSpace(target.EventKey) != "" && target.DecisionAt.Equal(decisionAt) &&
		artifact.DocumentID > 0 && artifact.DocumentVersionID == command.DocumentVersionID && artifact.ArtifactType == "plaintext" &&
		artifact.MIMEType == "text/plain; charset=utf-8" && lowerHexDigest(artifact.TransformerProfileSHA256) &&
		lowerHexDigest(artifact.PlaintextSHA256) && artifact.SizeBytes > 0 && artifact.RetentionUntil.After(decisionAt)
}

func automaticClaimEvidenceTargetEquivalent(left, right AutomaticClaimEvidenceTargetDTO) bool {
	return left.MicroEventID == right.MicroEventID && left.EventVersion == right.EventVersion && left.EventKey == right.EventKey &&
		left.Artifact == right.Artifact && right.ExternalModelAllowed
}

func automaticClaimEvidenceProjectionMatches(value AutomaticClaimEvidenceProjectionDTO, artifact AutomaticClaimEvidenceArtifactDTO) bool {
	return value.Plaintext != "" && utf8.ValidString(value.Plaintext) && value.MIMEType == artifact.MIMEType &&
		value.SHA256 == artifact.PlaintextSHA256 && value.SizeBytes == artifact.SizeBytes && int64(len(value.Plaintext)) == value.SizeBytes
}

func boundedAtomicClaimEvidenceBody(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end], true
}
