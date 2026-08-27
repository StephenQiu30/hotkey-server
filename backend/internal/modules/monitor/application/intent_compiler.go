package application

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"golang.org/x/text/unicode/norm"
)

const (
	IntentCompilerVersion                   = "monitor-intent-compiler-v1"
	IntentSearchNormalizationProfileVersion = ingestionapplication.CanonicalDocumentSearchNormalizationProfileVersion
	IntentSemanticGenerationUnavailable     = "semantic_generation_unavailable"
	maximumCompiledIntentClauses            = 128
	maximumCompiledObjectivePhraseBytes     = 2048
)

type CompiledIntentClauseDTO struct {
	Operator        string
	Field           string
	Value           string
	NormalizedValue string
	Origin          string
}

type CompiledIntentEntityDTO struct {
	CanonicalID       string
	Aliases           []string
	NormalizedAliases []string
}

type PersistPreviewCompiledProfileDTO struct {
	Task                              IntentAnalysisTaskDTO
	CompilerVersion                   string
	MatchingAlgorithmVersion          string
	LexicalAlgorithmVersion           string
	SemanticAlgorithmVersion          string
	StructuredAlgorithmVersion        string
	SearchNormalizationProfileVersion string
	SemanticState                     string
	SemanticUnavailableReason         string
	ProfileHash                       string
	Clauses                           []CompiledIntentClauseDTO
	Entities                          []CompiledIntentEntityDTO
	ReadyAt                           time.Time
}

type PersistPreviewCompiledProfileReceiptDTO struct {
	CompiledProfileID         int64
	ConfigVersionID           int64
	IntentRevisionID          int64
	Status                    string
	SemanticState             string
	SemanticUnavailableReason string
	Reused                    bool
}

type CompiledIntentProfileDTO struct {
	CompiledProfileID                 int64
	MonitorID                         int64
	Purpose                           string
	ConfigVersionID                   int64
	PreviewRunID                      int64
	DraftID                           int64
	DraftResourceVersion              int64
	IntentRevisionID                  int64
	CompilerVersion                   string
	MatchingAlgorithmVersion          string
	LexicalAlgorithmVersion           string
	SemanticAlgorithmVersion          string
	StructuredAlgorithmVersion        string
	SearchNormalizationProfileVersion string
	SemanticState                     string
	SemanticUnavailableReason         string
	ProfileHash                       string
}

type CompilePreviewIntentProfileCommand struct {
	Preview PreparedIntentPreviewDTO
}

type CompilePreviewIntentProfileResult struct {
	Profile CompiledIntentProfileDTO
	Reused  bool
}

type IntentCompiler struct {
	profiles   CompiledIntentProfileRepository
	embeddings CompiledIntentEmbeddingProducer
	clock      IntentClock
}

func NewIntentCompiler(profiles CompiledIntentProfileRepository, embeddings CompiledIntentEmbeddingProducer, clock IntentClock) (*IntentCompiler, error) {
	if profiles == nil || embeddings == nil || clock == nil {
		return nil, fmt.Errorf("%w: intent compiler dependencies are required", ErrInvalidIntentContract)
	}
	return &IntentCompiler{profiles: profiles, embeddings: embeddings, clock: clock}, nil
}

func (compiler *IntentCompiler) CompilePreview(ctx context.Context, command CompilePreviewIntentProfileCommand) (CompilePreviewIntentProfileResult, error) {
	if compiler == nil || compiler.profiles == nil || compiler.embeddings == nil || compiler.clock == nil {
		return CompilePreviewIntentProfileResult{}, ErrInvalidIntentContract
	}
	task := command.Preview.Task
	if task.Run.Kind != "preview" || task.Run.RunID <= 0 || task.Run.MonitorID <= 0 || task.Run.DraftID <= 0 ||
		task.Run.DraftResourceVersion <= 0 || task.SampleLimit < 1 || task.SampleLimit > 200 || !validIntentApplicationSHA256(task.Run.InputHash) {
		return CompilePreviewIntentProfileResult{}, ErrInvalidIntentContract
	}
	draft, err := intentDraftFromDTO(command.Preview.Draft)
	if err != nil {
		return CompilePreviewIntentProfileResult{}, err
	}
	if draft.MonitorID() != task.Run.MonitorID || draft.DraftID() != task.Run.DraftID || draft.ResourceVersion() != task.Run.DraftResourceVersion ||
		intentPreviewInputHash(draft, task.AnalysisProfile, task.SampleLimit) != task.Run.InputHash {
		return CompilePreviewIntentProfileResult{}, invalidIntentContract(fmt.Errorf("preview compile owner is not exact"))
	}
	clauses, err := compileIntentClauses(command.Preview.Draft)
	if err != nil {
		return CompilePreviewIntentProfileResult{}, err
	}
	entities, err := compileIntentEntities(command.Preview.Draft.Entities)
	if err != nil {
		return CompilePreviewIntentProfileResult{}, err
	}
	profileHash := compiledIntentProfileHash(task, clauses, entities)
	readyAt := compiler.clock.Now().UTC()
	if readyAt.IsZero() {
		return CompilePreviewIntentProfileResult{}, invalidIntentContract(fmt.Errorf("intent compiler clock returned zero"))
	}
	persist := PersistPreviewCompiledProfileDTO{
		Task: task, CompilerVersion: IntentCompilerVersion,
		MatchingAlgorithmVersion:          ingestionapplication.HybridRecallMatchingAlgorithmVersion,
		LexicalAlgorithmVersion:           ingestionapplication.LexicalRecallAlgorithmVersion,
		SemanticAlgorithmVersion:          ingestionapplication.SemanticRecallAlgorithmVersion,
		StructuredAlgorithmVersion:        ingestionapplication.StructuredRecallAlgorithmVersion,
		SearchNormalizationProfileVersion: IntentSearchNormalizationProfileVersion,
		SemanticState:                     ingestionapplication.SemanticRecallStateUnavailable,
		SemanticUnavailableReason:         IntentSemanticGenerationUnavailable,
		ProfileHash:                       profileHash, Clauses: clauses, Entities: entities, ReadyAt: readyAt,
	}
	receipt, err := compiler.profiles.PersistPreviewCompiledProfile(ctx, persist)
	if err != nil {
		return CompilePreviewIntentProfileResult{}, err
	}
	if receipt.CompiledProfileID <= 0 || receipt.ConfigVersionID <= 0 || receipt.IntentRevisionID <= 0 ||
		(receipt.Status != "building" && receipt.Status != "ready") {
		return CompilePreviewIntentProfileResult{}, invalidIntentContract(fmt.Errorf("compiled profile receipt is incomplete"))
	}
	semanticState, semanticReason := receipt.SemanticState, receipt.SemanticUnavailableReason
	if receipt.Status == "building" {
		if semanticState != IntentSemanticStateReady {
			embeddingInput := compiledIntentEmbeddingInput(command.Preview.Draft, clauses, entities)
			embeddingHash := intentRunHash("compiled-intent-embedding-v1", embeddingInput)
			produced, produceErr := compiler.embeddings.ProduceCompiledIntentEmbedding(ctx, ProduceCompiledIntentEmbeddingCommand{
				CompiledProfileID: receipt.CompiledProfileID, ConfigVersionID: receipt.ConfigVersionID,
				InputHash: embeddingHash, Input: embeddingInput,
			})
			if produceErr != nil {
				return CompilePreviewIntentProfileResult{}, produceErr
			}
			if err := validateProducedCompiledIntentEmbedding(ProduceCompiledIntentEmbeddingCommand{
				CompiledProfileID: receipt.CompiledProfileID, ConfigVersionID: receipt.ConfigVersionID,
				InputHash: embeddingHash, Input: embeddingInput,
			}, produced); err != nil {
				return CompilePreviewIntentProfileResult{}, err
			}
			semanticState, semanticReason = IntentSemanticStateReady, ""
			if produced.Availability == IntentEmbeddingAvailabilityDegraded {
				semanticState, semanticReason = IntentSemanticStateUnavailable, produced.UnavailableReason
			}
		}
		completed, completeErr := compiler.profiles.CompletePreviewCompiledProfile(ctx, CompletePreviewCompiledProfileDTO{
			CompiledProfileID: receipt.CompiledProfileID, ConfigVersionID: receipt.ConfigVersionID,
			IntentRevisionID: receipt.IntentRevisionID, ProfileHash: profileHash,
			SemanticState: semanticState, SemanticUnavailableReason: semanticReason, ReadyAt: readyAt,
		})
		if completeErr != nil {
			return CompilePreviewIntentProfileResult{}, completeErr
		}
		if completed.CompiledProfileID != receipt.CompiledProfileID || completed.Status != "ready" ||
			completed.SemanticState != semanticState || completed.SemanticUnavailableReason != semanticReason {
			return CompilePreviewIntentProfileResult{}, ErrCompiledIntentProfileConflict
		}
		receipt.Reused = receipt.Reused && completed.Reused
	} else if !validCompletedIntentSemanticState(semanticState, semanticReason) {
		return CompilePreviewIntentProfileResult{}, ErrCompiledIntentProfileConflict
	}
	return CompilePreviewIntentProfileResult{Profile: CompiledIntentProfileDTO{
		CompiledProfileID: receipt.CompiledProfileID, MonitorID: task.Run.MonitorID, Purpose: "preview",
		ConfigVersionID: receipt.ConfigVersionID, PreviewRunID: task.Run.RunID, DraftID: task.Run.DraftID,
		DraftResourceVersion: task.Run.DraftResourceVersion, IntentRevisionID: receipt.IntentRevisionID,
		CompilerVersion: persist.CompilerVersion, MatchingAlgorithmVersion: persist.MatchingAlgorithmVersion,
		LexicalAlgorithmVersion: persist.LexicalAlgorithmVersion, SemanticAlgorithmVersion: persist.SemanticAlgorithmVersion,
		StructuredAlgorithmVersion:        persist.StructuredAlgorithmVersion,
		SearchNormalizationProfileVersion: persist.SearchNormalizationProfileVersion,
		SemanticState:                     semanticState, SemanticUnavailableReason: semanticReason,
		ProfileHash: profileHash,
	}, Reused: receipt.Reused}, nil
}

func compileIntentClauses(draft IntentDraftDTO) ([]CompiledIntentClauseDTO, error) {
	result := make([]CompiledIntentClauseDTO, 0, len(draft.Clauses)+len(draft.Candidates)+1)
	seen := make(map[string]struct{}, cap(result))
	appendClause := func(operator, field, value, origin string) error {
		normalized := normalizeCompiledIntentValue(value)
		if normalized == "" || len([]byte(value)) > maximumCompiledObjectivePhraseBytes {
			return invalidIntentContract(fmt.Errorf("compiled clause is empty or too large"))
		}
		key := operator + "\x00" + field + "\x00" + normalized
		if _, duplicate := seen[key]; duplicate {
			return nil
		}
		if len(result) >= maximumCompiledIntentClauses {
			return invalidIntentContract(fmt.Errorf("compiled intent has too many clauses"))
		}
		seen[key] = struct{}{}
		result = append(result, CompiledIntentClauseDTO{Operator: operator, Field: field, Value: value, NormalizedValue: normalized, Origin: origin})
		return nil
	}
	for _, item := range draft.Clauses {
		if err := appendClause(item.Operator, item.Field, item.Value, "intent_clause"); err != nil {
			return nil, err
		}
	}
	for _, candidate := range draft.Candidates {
		if candidate.ApprovalStatus != "approved" {
			continue
		}
		if err := appendClause("should", "term", candidate.Value, "approved_candidate"); err != nil {
			return nil, err
		}
	}
	for _, phrase := range splitCompiledObjective(draft.Objective) {
		if err := appendClause("should", "phrase", phrase, "objective_derived"); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func compileIntentEntities(items []IntentEntityDTO) ([]CompiledIntentEntityDTO, error) {
	result := make([]CompiledIntentEntityDTO, 0, len(items))
	globalAliases := make(map[string]string)
	for _, item := range items {
		aliases := append([]string{item.DisplayName}, item.Aliases...)
		aliases = sortedUniqueCompiledValues(aliases)
		for _, alias := range aliases {
			key := normalizeCompiledIntentValue(alias)
			if prior, found := globalAliases[key]; found && prior != item.CanonicalID {
				return nil, invalidIntentContract(fmt.Errorf("compiled entity alias is ambiguous"))
			}
			globalAliases[key] = item.CanonicalID
		}
		normalizedAliases := make([]string, 0, len(aliases))
		for _, alias := range aliases {
			normalizedAliases = append(normalizedAliases, normalizeCompiledIntentValue(alias))
		}
		result = append(result, CompiledIntentEntityDTO{CanonicalID: item.CanonicalID, Aliases: aliases, NormalizedAliases: normalizedAliases})
	}
	return result, nil
}

func splitCompiledObjective(value string) []string {
	value = strings.TrimSpace(norm.NFC.String(value))
	if value == "" {
		return nil
	}
	if len([]byte(value)) <= maximumCompiledObjectivePhraseBytes {
		return []string{value}
	}
	result := make([]string, 0, len([]byte(value))/maximumCompiledObjectivePhraseBytes+1)
	for len(value) > 0 {
		end := len(value)
		if end > maximumCompiledObjectivePhraseBytes {
			end = maximumCompiledObjectivePhraseBytes
			for end > 0 && !utf8.RuneStart(value[end]) {
				end--
			}
		}
		part := strings.TrimSpace(value[:end])
		if part != "" {
			result = append(result, part)
		}
		value = strings.TrimSpace(value[end:])
	}
	return result
}

func normalizeCompiledIntentValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(norm.NFC.String(strings.TrimSpace(value))), " "))
}

func sortedUniqueCompiledValues(values []string) []string {
	byKey := make(map[string]string, len(values))
	for _, value := range values {
		value = strings.TrimSpace(norm.NFC.String(value))
		key := normalizeCompiledIntentValue(value)
		if key != "" {
			byKey[key] = value
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func compiledIntentProfileHash(task IntentAnalysisTaskDTO, clauses []CompiledIntentClauseDTO, entities []CompiledIntentEntityDTO) string {
	parts := []string{
		// A Compiled Profile identifies immutable inputs and compiler facts, not
		// the execution attempt that happened to materialize them. Keeping the
		// transient run ID out makes retries of the exact draft/version converge
		// on the same snapshot hash while the profile row still retains provenance
		// through preview_run_id.
		"compiled-intent-profile-v1", strconv.FormatInt(task.Run.MonitorID, 10),
		strconv.FormatInt(task.Run.DraftID, 10), strconv.FormatInt(task.Run.DraftResourceVersion, 10), task.Run.InputHash,
		IntentCompilerVersion, ingestionapplication.HybridRecallMatchingAlgorithmVersion, ingestionapplication.LexicalRecallAlgorithmVersion,
		ingestionapplication.SemanticRecallAlgorithmVersion, ingestionapplication.StructuredRecallAlgorithmVersion,
		IntentSearchNormalizationProfileVersion,
		strconv.Itoa(len(clauses)),
	}
	for _, item := range clauses {
		parts = append(parts, item.Operator, item.Field, item.Value, item.NormalizedValue, item.Origin)
	}
	parts = append(parts, strconv.Itoa(len(entities)))
	for _, item := range entities {
		parts = append(parts, item.CanonicalID, strconv.Itoa(len(item.Aliases)))
		for index, alias := range item.Aliases {
			parts = append(parts, alias, item.NormalizedAliases[index])
		}
	}
	return intentRunHash(parts...)
}

func compiledIntentEmbeddingInput(draft IntentDraftDTO, clauses []CompiledIntentClauseDTO, entities []CompiledIntentEntityDTO) string {
	parts := []string{"objective", normalizeCompiledIntentValue(draft.Objective), "clauses"}
	for _, clause := range clauses {
		parts = append(parts, clause.Operator, clause.Field, clause.NormalizedValue)
	}
	parts = append(parts, "entities")
	for _, entity := range entities {
		parts = append(parts, entity.CanonicalID)
		parts = append(parts, entity.NormalizedAliases...)
	}
	return strings.Join(parts, "\n")
}

func validCompletedIntentSemanticState(state, reason string) bool {
	return state == IntentSemanticStateReady && reason == "" ||
		state == IntentSemanticStateUnavailable && (reason == IntentSemanticModelUnavailable || reason == IntentSemanticGenerationUnavailable)
}
