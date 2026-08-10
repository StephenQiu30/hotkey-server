package application

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

type ReadPublishableIntentProfileQuery struct {
	MonitorID       int64
	ConfigVersionID int64
}

// PublishableIntentProfileDTO is a complete immutable preview snapshot. A
// repository returns Exists=false only when the configuration has no v2
// intent draft; an existing intent without a successful exact preview is an
// error and must never fall back to legacy rules.
type PublishableIntentProfileDTO struct {
	Exists                            bool
	MonitorID                         int64
	ConfigVersionID                   int64
	PreviewRunID                      int64
	DraftID                           int64
	DraftResourceVersion              int64
	IntentRevisionID                  int64
	PreviewCompiledProfileID          int64
	CompilerVersion                   string
	MatchingAlgorithmVersion          string
	LexicalAlgorithmVersion           string
	SemanticAlgorithmVersion          string
	StructuredAlgorithmVersion        string
	SearchNormalizationProfileVersion string
	SemanticState                     string
	SemanticUnavailableReason         string
	PreviewProfileHash                string
	Clauses                           []CompiledIntentClauseDTO
	Entities                          []CompiledIntentEntityDTO
}

type StagePublishedIntentProfileDTO struct {
	MonitorID                         int64
	ConfigVersionID                   int64
	IntentRevisionID                  int64
	SourcePreviewRunID                int64
	SourcePreviewCompiledProfileID    int64
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
}

type StagePublishedIntentProfileReceiptDTO struct {
	CompiledProfileID int64
	Reused            bool
}

type CompletePublishedIntentProfileDTO struct {
	MonitorID               int64
	ConfigVersionID         int64
	CompiledProfileID       int64
	PreviousConfigVersionID int64
	ProfileHash             string
	PublishedAt             time.Time
}

type CompiledCollectionTermDTO struct {
	Value    string
	Excluded bool
}

type IntentPublicationDTO struct {
	Enabled            bool
	MonitorID          int64
	ConfigVersionID    int64
	CompiledProfileID  int64
	IntentRevisionID   int64
	SourcePreviewRunID int64
	ProfileHash        string
	CollectionTerms    []CompiledCollectionTermDTO
	LocaleClauses      []IntentClauseDTO
}

type PrepareIntentPublicationCommand struct {
	MonitorID       int64
	ConfigVersionID int64
}

type PrepareIntentPublicationResult struct {
	Publication IntentPublicationDTO
}

type PreviewIntentPublicationCommand struct {
	MonitorID       int64
	ConfigVersionID int64
}

type PreviewIntentPublicationResult struct {
	Enabled         bool
	ProfileHash     string
	CollectionTerms []CompiledCollectionTermDTO
	LocaleClauses   []IntentClauseDTO
}

type CompleteIntentPublicationCommand struct {
	Publication             IntentPublicationDTO
	PreviousConfigVersionID int64
	PublishedAt             time.Time
}

// SchedulePublishedIntentBackfillCommand contains only immutable publication
// identities. The scheduler must enqueue a durable, paginated backfill; it
// must not copy intent text, document text, or query expressions into a job.
type SchedulePublishedIntentBackfillCommand struct {
	MonitorID         int64
	MonitorVersionID  int64
	CompiledProfileID int64
}

type SchedulePublishedIntentBackfillResult struct {
	MonitorID         int64
	MonitorVersionID  int64
	CompiledProfileID int64
	JobID             int64
	Created           bool
}

type PublishedIntentBackfillScheduler interface {
	SchedulePublishedIntentBackfill(context.Context, SchedulePublishedIntentBackfillCommand) (SchedulePublishedIntentBackfillResult, error)
}

type IntentPublicationService struct {
	repository IntentPublicationRepository
	backfills  PublishedIntentBackfillScheduler
}

func NewIntentPublicationService(repository IntentPublicationRepository, backfills PublishedIntentBackfillScheduler) (*IntentPublicationService, error) {
	if repository == nil || backfills == nil {
		return nil, fmt.Errorf("%w: intent publication repository and backfill scheduler are required", ErrInvalidIntentContract)
	}
	return &IntentPublicationService{repository: repository, backfills: backfills}, nil
}

func (service *IntentPublicationService) Prepare(ctx context.Context, command PrepareIntentPublicationCommand) (PrepareIntentPublicationResult, error) {
	if service == nil || service.repository == nil || command.MonitorID <= 0 || command.ConfigVersionID <= 0 {
		return PrepareIntentPublicationResult{}, ErrInvalidIntentContract
	}
	candidate, err := service.repository.ReadPublishableIntentProfile(ctx, ReadPublishableIntentProfileQuery(command))
	if err != nil {
		return PrepareIntentPublicationResult{}, err
	}
	if !candidate.Exists {
		return PrepareIntentPublicationResult{Publication: IntentPublicationDTO{}}, nil
	}
	if err := validatePublishableIntentProfile(candidate, command); err != nil {
		return PrepareIntentPublicationResult{}, err
	}
	terms, locales, err := compiledIntentPublicationInputs(candidate.Clauses, candidate.Entities)
	if err != nil {
		return PrepareIntentPublicationResult{}, err
	}
	profileHash := publishedIntentProfileHash(candidate)
	stage := StagePublishedIntentProfileDTO{
		MonitorID: candidate.MonitorID, ConfigVersionID: candidate.ConfigVersionID,
		IntentRevisionID: candidate.IntentRevisionID, SourcePreviewRunID: candidate.PreviewRunID,
		SourcePreviewCompiledProfileID: candidate.PreviewCompiledProfileID,
		CompilerVersion:                candidate.CompilerVersion, MatchingAlgorithmVersion: candidate.MatchingAlgorithmVersion,
		LexicalAlgorithmVersion: candidate.LexicalAlgorithmVersion, SemanticAlgorithmVersion: candidate.SemanticAlgorithmVersion,
		StructuredAlgorithmVersion:        candidate.StructuredAlgorithmVersion,
		SearchNormalizationProfileVersion: candidate.SearchNormalizationProfileVersion,
		SemanticState:                     candidate.SemanticState, SemanticUnavailableReason: candidate.SemanticUnavailableReason,
		ProfileHash: profileHash, Clauses: cloneCompiledIntentClauses(candidate.Clauses),
		Entities: cloneCompiledIntentEntities(candidate.Entities),
	}
	receipt, err := service.repository.StagePublishedIntentProfile(ctx, stage)
	if err != nil {
		return PrepareIntentPublicationResult{}, err
	}
	if receipt.CompiledProfileID <= 0 {
		return PrepareIntentPublicationResult{}, ErrIntentPublicationUnavailable
	}
	return PrepareIntentPublicationResult{Publication: IntentPublicationDTO{
		Enabled: true, MonitorID: candidate.MonitorID, ConfigVersionID: candidate.ConfigVersionID,
		CompiledProfileID: receipt.CompiledProfileID, IntentRevisionID: candidate.IntentRevisionID,
		SourcePreviewRunID: candidate.PreviewRunID, ProfileHash: profileHash,
		CollectionTerms: terms, LocaleClauses: locales,
	}}, nil
}

// Preview is read-only. It resolves the exact successful draft preview and
// returns the same compiled collection inputs that Prepare will stage during
// publication, without creating a published profile or queue job.
func (service *IntentPublicationService) Preview(ctx context.Context, command PreviewIntentPublicationCommand) (PreviewIntentPublicationResult, error) {
	if service == nil || service.repository == nil || command.MonitorID <= 0 || command.ConfigVersionID <= 0 {
		return PreviewIntentPublicationResult{}, ErrInvalidIntentContract
	}
	candidate, err := service.repository.ReadPublishableIntentProfile(ctx, ReadPublishableIntentProfileQuery(command))
	if err != nil {
		return PreviewIntentPublicationResult{}, err
	}
	if !candidate.Exists {
		return PreviewIntentPublicationResult{}, nil
	}
	if err := validatePublishableIntentProfile(candidate, PrepareIntentPublicationCommand(command)); err != nil {
		return PreviewIntentPublicationResult{}, err
	}
	terms, locales, err := compiledIntentPublicationInputs(candidate.Clauses, candidate.Entities)
	if err != nil {
		return PreviewIntentPublicationResult{}, err
	}
	return PreviewIntentPublicationResult{
		Enabled: true, ProfileHash: publishedIntentProfileHash(candidate),
		CollectionTerms: terms, LocaleClauses: locales,
	}, nil
}

func (service *IntentPublicationService) Complete(ctx context.Context, command CompleteIntentPublicationCommand) error {
	publication := command.Publication
	if service == nil || service.repository == nil || service.backfills == nil || !publication.Enabled || publication.MonitorID <= 0 ||
		publication.ConfigVersionID <= 0 || publication.CompiledProfileID <= 0 || !validIntentApplicationSHA256(publication.ProfileHash) ||
		command.PreviousConfigVersionID < 0 || command.PublishedAt.IsZero() {
		return ErrInvalidIntentContract
	}
	if err := service.repository.CompletePublishedIntentProfile(ctx, CompletePublishedIntentProfileDTO{
		MonitorID: publication.MonitorID, ConfigVersionID: publication.ConfigVersionID,
		CompiledProfileID: publication.CompiledProfileID, PreviousConfigVersionID: command.PreviousConfigVersionID,
		ProfileHash: publication.ProfileHash, PublishedAt: command.PublishedAt.UTC(),
	}); err != nil {
		return err
	}
	receipt, err := service.backfills.SchedulePublishedIntentBackfill(ctx, SchedulePublishedIntentBackfillCommand{
		MonitorID: publication.MonitorID, MonitorVersionID: publication.ConfigVersionID,
		CompiledProfileID: publication.CompiledProfileID,
	})
	if err != nil {
		return err
	}
	if receipt.MonitorID != publication.MonitorID || receipt.MonitorVersionID != publication.ConfigVersionID ||
		receipt.CompiledProfileID != publication.CompiledProfileID || receipt.JobID <= 0 {
		return ErrIntentPublicationUnavailable
	}
	return nil
}

func validatePublishableIntentProfile(candidate PublishableIntentProfileDTO, command PrepareIntentPublicationCommand) error {
	if candidate.MonitorID != command.MonitorID || candidate.ConfigVersionID != command.ConfigVersionID ||
		candidate.PreviewRunID <= 0 || candidate.DraftID <= 0 || candidate.DraftResourceVersion <= 0 || candidate.IntentRevisionID <= 0 ||
		candidate.PreviewCompiledProfileID <= 0 || !validIntentApplicationSHA256(candidate.PreviewProfileHash) ||
		!validCompletedIntentSemanticState(candidate.SemanticState, candidate.SemanticUnavailableReason) ||
		candidate.CompilerVersion != IntentCompilerVersion || candidate.SearchNormalizationProfileVersion != IntentSearchNormalizationProfileVersion ||
		len(candidate.Clauses) > maximumCompiledIntentClauses || len(candidate.Entities) > 64 {
		return ErrIntentPublicationUnavailable
	}
	if candidate.MatchingAlgorithmVersion != ingestionapplication.HybridRecallMatchingAlgorithmVersion ||
		candidate.LexicalAlgorithmVersion != ingestionapplication.LexicalRecallAlgorithmVersion ||
		candidate.SemanticAlgorithmVersion != ingestionapplication.SemanticRecallAlgorithmVersion ||
		candidate.StructuredAlgorithmVersion != ingestionapplication.StructuredRecallAlgorithmVersion {
		return ErrIntentPublicationUnavailable
	}
	return nil
}

func compiledIntentPublicationInputs(clauses []CompiledIntentClauseDTO, entities []CompiledIntentEntityDTO) ([]CompiledCollectionTermDTO, []IntentClauseDTO, error) {
	terms := make([]CompiledCollectionTermDTO, 0, len(clauses)+len(entities)*2)
	locales := make([]IntentClauseDTO, 0)
	termSeen := make(map[string]struct{})
	appendTerm := func(value string, excluded bool) {
		key := strconv.FormatBool(excluded) + "\x00" + normalizeCompiledIntentValue(value)
		if _, duplicate := termSeen[key]; duplicate {
			return
		}
		termSeen[key] = struct{}{}
		terms = append(terms, CompiledCollectionTermDTO{Value: value, Excluded: excluded})
	}
	for _, clause := range clauses {
		if !validCompiledIntentClauseForPublication(clause) {
			return nil, nil, ErrIntentPublicationUnavailable
		}
		switch clause.Field {
		case "term", "phrase", "action", "location":
			appendTerm(clause.Value, clause.Operator == "must_not")
		case "language", "region":
			locales = append(locales, IntentClauseDTO{Operator: clause.Operator, Field: clause.Field, Value: clause.Value})
		}
	}
	for _, entity := range entities {
		if entity.CanonicalID == "" || len(entity.Aliases) != len(entity.NormalizedAliases) {
			return nil, nil, ErrIntentPublicationUnavailable
		}
		for _, alias := range entity.Aliases {
			appendTerm(alias, false)
		}
	}
	hasInclude := false
	for _, term := range terms {
		hasInclude = hasInclude || !term.Excluded
	}
	if !hasInclude {
		return nil, nil, ErrIntentPublicationUnavailable
	}
	return terms, locales, nil
}

func validCompiledIntentClauseForPublication(clause CompiledIntentClauseDTO) bool {
	return clause.Value != "" && clause.NormalizedValue == normalizeCompiledIntentValue(clause.Value) &&
		validCompiledIntentClauseIdentity(clause.Operator, clause.Field, clause.Origin)
}

func validCompiledIntentClauseIdentity(operator, field, origin string) bool {
	validOperator := operator == "must" || operator == "should" || operator == "must_not"
	validField := field == "term" || field == "phrase" || field == "action" || field == "location" || field == "language" || field == "region" || field == "source" || field == "time_window"
	validOrigin := origin == "intent_clause" || origin == "objective_derived" || origin == "approved_candidate"
	return validOperator && validField && validOrigin
}

func publishedIntentProfileHash(candidate PublishableIntentProfileDTO) string {
	parts := []string{
		"published-compiled-intent-profile-v1", strconv.FormatInt(candidate.MonitorID, 10),
		strconv.FormatInt(candidate.ConfigVersionID, 10), strconv.FormatInt(candidate.IntentRevisionID, 10),
		strconv.FormatInt(candidate.PreviewRunID, 10), strconv.FormatInt(candidate.PreviewCompiledProfileID, 10),
		candidate.PreviewProfileHash, candidate.CompilerVersion, candidate.MatchingAlgorithmVersion,
		candidate.LexicalAlgorithmVersion, candidate.SemanticAlgorithmVersion, candidate.StructuredAlgorithmVersion,
		candidate.SearchNormalizationProfileVersion, candidate.SemanticState, candidate.SemanticUnavailableReason,
		strconv.Itoa(len(candidate.Clauses)),
	}
	for _, clause := range candidate.Clauses {
		parts = append(parts, clause.Operator, clause.Field, clause.Value, clause.NormalizedValue, clause.Origin)
	}
	parts = append(parts, strconv.Itoa(len(candidate.Entities)))
	for _, entity := range candidate.Entities {
		parts = append(parts, entity.CanonicalID, strconv.Itoa(len(entity.Aliases)))
		for index, alias := range entity.Aliases {
			parts = append(parts, alias, entity.NormalizedAliases[index])
		}
	}
	return intentRunHash(parts...)
}

func cloneCompiledIntentClauses(items []CompiledIntentClauseDTO) []CompiledIntentClauseDTO {
	return append([]CompiledIntentClauseDTO(nil), items...)
}

func cloneCompiledIntentEntities(items []CompiledIntentEntityDTO) []CompiledIntentEntityDTO {
	result := make([]CompiledIntentEntityDTO, len(items))
	for index, item := range items {
		result[index] = CompiledIntentEntityDTO{
			CanonicalID: item.CanonicalID, Aliases: append([]string(nil), item.Aliases...),
			NormalizedAliases: append([]string(nil), item.NormalizedAliases...),
		}
	}
	return result
}

func sortedCompiledCollectionTerms(items []CompiledCollectionTermDTO) []CompiledCollectionTermDTO {
	result := append([]CompiledCollectionTermDTO(nil), items...)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Excluded != result[right].Excluded {
			return !result[left].Excluded
		}
		return strings.ToLower(result[left].Value) < strings.ToLower(result[right].Value)
	})
	return result
}
