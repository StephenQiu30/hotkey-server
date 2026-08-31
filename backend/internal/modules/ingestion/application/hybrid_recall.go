package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

const (
	LexicalRecallLimit     = 100
	SemanticRecallLimit    = 100
	StructuredRecallLimit  = 50
	FusedRecallLimit       = 200
	ReciprocalRankConstant = 60

	HybridRecallMatchingAlgorithmVersion = "rrf-k60-v1"
	LexicalRecallAlgorithmVersion        = "fts-trgm-dice-v1"
	SemanticRecallAlgorithmVersion       = "halfvec-cosine-v1"
	StructuredRecallAlgorithmVersion     = "entity-hard-rule-v1"
	SemanticRecallStateReady             = "ready"
	SemanticRecallStateUnavailable       = "unavailable"
)

var (
	ErrInvalidHybridRecallQuery = errors.New("hybrid recall query is invalid")
	// ErrSemanticRecallUnavailable is the only degradable reader failure. Data
	// integrity, invalid-input and context errors must remain visible to callers.
	ErrSemanticRecallUnavailable = errors.New("semantic recall is unavailable")
	semanticVersionPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,63}$`)
)

// RecallClauseDTO and RecallEntityDTO are Application-owned POJOs copied from
// an exact ready CompiledProfile. Infrastructure must not reconstruct these
// facts from legacy monitor_rules or an editable draft.
type RecallClauseDTO struct {
	Operator string
	Field    string
	Value    string
	Origin   string
}

type RecallEntityDTO struct {
	CanonicalID string
	Aliases     []string
}

// SemanticRecallProfileDTO pins the query vector to one immutable 1024-D
// model space. []float32 is an Application value, not a pgvector type.
type SemanticRecallProfileDTO struct {
	EmbeddingProfileID      int64
	EmbeddingProfileVersion int64
	ModelVersion            string
	QueryVector             []float32
}

type HybridRecallQuery struct {
	MonitorID            int64
	Purpose              string
	ConfigVersionID      int64
	MonitorVersionID     int64
	PreviewRunID         int64
	DraftID              int64
	DraftResourceVersion int64
	CompiledProfileID    int64
}

// ReadyRecallProfileDTO is returned only after Infrastructure proves that the
// profile belongs to the exact published MonitorVersion and immutable intent
// revision. Callers cannot supply unverified clauses or vectors.
type ReadyRecallProfileDTO struct {
	MonitorID                         int64
	Purpose                           string
	ConfigVersionID                   int64
	MonitorVersionID                  int64
	PreviewRunID                      int64
	DraftID                           int64
	DraftResourceVersion              int64
	CompiledProfileID                 int64
	MatchingAlgorithmVersion          string
	LexicalAlgorithmVersion           string
	SemanticAlgorithmVersion          string
	StructuredAlgorithmVersion        string
	SearchNormalizationProfileVersion string
	SemanticState                     string
	SemanticUnavailableReason         string
	Clauses                           []RecallClauseDTO
	Entities                          []RecallEntityDTO
	Semantic                          *SemanticRecallProfileDTO
}

type ReadyRecallProfileQuery struct {
	MonitorID            int64
	Purpose              string
	ConfigVersionID      int64
	MonitorVersionID     int64
	PreviewRunID         int64
	DraftID              int64
	DraftResourceVersion int64
	CompiledProfileID    int64
}

type RecallFilterDTO struct {
	Operator string
	Field    string
	Value    string
}

type LexicalRecallQueryDTO struct {
	ConfigVersionID, CompiledProfileID int64
	SearchNormalizationProfileVersion  string
	Must, Should, MustNot              []RecallFilterDTO
	Entities                           []RecallEntityDTO
	AlgorithmVersion                   string
	Limit                              int
}

type StructuredRecallQueryDTO struct {
	ConfigVersionID, CompiledProfileID int64
	SearchNormalizationProfileVersion  string
	Must, Should, MustNot              []RecallFilterDTO
	Entities                           []RecallEntityDTO
	AlgorithmVersion                   string
	Limit                              int
}

type SemanticRecallQueryDTO struct {
	ConfigVersionID, CompiledProfileID int64
	SearchNormalizationProfileVersion  string
	Must, MustNot                      []RecallFilterDTO
	EmbeddingProfileID                 int64
	EmbeddingProfileVersion            int64
	ModelVersion                       string
	QueryVector                        []float32
	AlgorithmVersion                   string
	Limit                              int
}

// RecallHitDTO is private to the Application/Infrastructure port. A hard
// exclusion is never fused even if another channel returned the same version.
type RecallHitDTO struct {
	DocumentVersionID int64
	Rank              int
	RawScore          float64
	HardExcluded      bool
	ExclusionReasons  []string
}

type RecallSignalDTO struct {
	Channel          string
	Rank             int
	RawScore         float64
	AlgorithmVersion string
}

type HybridRecallCandidateDTO struct {
	DocumentVersionID int64
	RRFScore          float64
	SemanticScore     *float64
	Signals           []RecallSignalDTO
}

type HybridRecallResult struct {
	MonitorID, ConfigVersionID, CompiledProfileID int64
	Purpose                                       string
	MonitorVersionID, PreviewRunID                int64
	MatchingAlgorithmVersion                      string
	Candidates                                    []HybridRecallCandidateDTO
	Degraded                                      bool
	DegradationReasons                            []string
}

// RecallPreviewDocumentDTO is a rights-safe title projection for one fused
// candidate. TitleAvailable is explicit so a preview can preserve candidate
// identity without leaking a withdrawn or non-displayable source title.
type RecallPreviewDocumentDTO struct {
	DocumentVersionID int64
	Title             string
	TitleAvailable    bool
}

type RecallPreviewDocumentQuery struct {
	DocumentVersionIDs []int64
}

type RecallPreviewDocumentResult struct {
	Documents []RecallPreviewDocumentDTO
}

type RecallPreviewDocumentReader interface {
	ReadRecallPreviewDocuments(context.Context, RecallPreviewDocumentQuery) (RecallPreviewDocumentResult, error)
}

type LexicalDocumentRecallReader interface {
	RecallLexical(context.Context, LexicalRecallQueryDTO) ([]RecallHitDTO, error)
}

type StructuredDocumentRecallReader interface {
	RecallStructured(context.Context, StructuredRecallQueryDTO) ([]RecallHitDTO, error)
}

type SemanticDocumentRecallReader interface {
	RecallSemantic(context.Context, SemanticRecallQueryDTO) ([]RecallHitDTO, error)
}

type ReadyRecallProfileReader interface {
	ReadReadyRecallProfile(context.Context, ReadyRecallProfileQuery) (ReadyRecallProfileDTO, error)
}

type HybridRecallService struct {
	profiles   ReadyRecallProfileReader
	lexical    LexicalDocumentRecallReader
	structured StructuredDocumentRecallReader
	semantic   SemanticDocumentRecallReader
}

func NewHybridRecallService(profiles ReadyRecallProfileReader, lexical LexicalDocumentRecallReader, structured StructuredDocumentRecallReader, semantic SemanticDocumentRecallReader) (*HybridRecallService, error) {
	if profiles == nil || lexical == nil || structured == nil {
		return nil, fmt.Errorf("%w: ready profile, lexical and structured readers are required", ErrInvalidHybridRecallQuery)
	}
	return &HybridRecallService{profiles: profiles, lexical: lexical, structured: structured, semantic: semantic}, nil
}

func (service *HybridRecallService) Recall(ctx context.Context, query HybridRecallQuery) (HybridRecallResult, error) {
	if service == nil || service.profiles == nil || service.lexical == nil || service.structured == nil {
		return HybridRecallResult{}, fmt.Errorf("%w: recall service is unavailable", ErrInvalidHybridRecallQuery)
	}
	if err := validateRecallOwner(query); err != nil {
		return HybridRecallResult{}, err
	}
	profile, err := service.profiles.ReadReadyRecallProfile(ctx, ReadyRecallProfileQuery(query))
	if err != nil {
		return HybridRecallResult{}, fmt.Errorf("read exact ready recall profile: %w", err)
	}
	if !readyProfileMatchesOwner(profile, query) {
		return HybridRecallResult{}, fmt.Errorf("%w: ready profile identity mismatch", ErrInvalidHybridRecallQuery)
	}
	if err := validateReadyRecallProfile(profile); err != nil {
		return HybridRecallResult{}, err
	}
	must, should, mustNot := splitRecallClauses(profile.Clauses)
	lexicalHits, err := service.lexical.RecallLexical(ctx, LexicalRecallQueryDTO{
		ConfigVersionID: query.ConfigVersionID, CompiledProfileID: query.CompiledProfileID,
		SearchNormalizationProfileVersion: profile.SearchNormalizationProfileVersion,
		Must:                              must, Should: should, MustNot: mustNot, Entities: copyRecallEntities(profile.Entities),
		AlgorithmVersion: profile.LexicalAlgorithmVersion, Limit: LexicalRecallLimit,
	})
	if err != nil {
		return HybridRecallResult{}, fmt.Errorf("lexical document recall: %w", err)
	}
	structuredHits, err := service.structured.RecallStructured(ctx, StructuredRecallQueryDTO{
		ConfigVersionID: query.ConfigVersionID, CompiledProfileID: query.CompiledProfileID,
		SearchNormalizationProfileVersion: profile.SearchNormalizationProfileVersion,
		Must:                              must, Should: should, MustNot: mustNot, Entities: copyRecallEntities(profile.Entities),
		AlgorithmVersion: profile.StructuredAlgorithmVersion, Limit: StructuredRecallLimit,
	})
	if err != nil {
		return HybridRecallResult{}, fmt.Errorf("structured document recall: %w", err)
	}

	degradation := make([]string, 0, 1)
	semanticHits := []RecallHitDTO{}
	if profile.Semantic == nil {
		degradation = append(degradation, profile.SemanticUnavailableReason)
	} else if service.semantic == nil {
		degradation = append(degradation, "semantic_reader_unavailable")
	} else {
		semanticHits, err = service.semantic.RecallSemantic(ctx, SemanticRecallQueryDTO{
			ConfigVersionID: query.ConfigVersionID, CompiledProfileID: query.CompiledProfileID,
			SearchNormalizationProfileVersion: profile.SearchNormalizationProfileVersion,
			Must:                              must, MustNot: mustNot,
			EmbeddingProfileID:      profile.Semantic.EmbeddingProfileID,
			EmbeddingProfileVersion: profile.Semantic.EmbeddingProfileVersion,
			ModelVersion:            profile.Semantic.ModelVersion,
			QueryVector:             append([]float32(nil), profile.Semantic.QueryVector...),
			AlgorithmVersion:        profile.SemanticAlgorithmVersion, Limit: SemanticRecallLimit,
		})
		if err != nil {
			if !errors.Is(err, ErrSemanticRecallUnavailable) {
				return HybridRecallResult{}, fmt.Errorf("semantic document recall: %w", err)
			}
			semanticHits = nil
			degradation = append(degradation, "semantic_recall_unavailable")
		}
	}

	if err := validateRecallHitBatch(lexicalHits, LexicalRecallLimit, ingestiondomain.RecallChannelLexical); err != nil {
		return HybridRecallResult{}, err
	}
	if err := validateRecallHitBatch(structuredHits, StructuredRecallLimit, ingestiondomain.RecallChannelStructured); err != nil {
		return HybridRecallResult{}, err
	}
	if err := validateRecallHitBatch(semanticHits, SemanticRecallLimit, ingestiondomain.RecallChannelSemantic); err != nil {
		return HybridRecallResult{}, err
	}
	excluded := collectHardExcluded(lexicalHits, structuredHits, semanticHits)
	signals := make([]ingestiondomain.RecallSignal, 0, len(lexicalHits)+len(structuredHits)+len(semanticHits))
	if err := appendRecallSignals(&signals, lexicalHits, excluded, ingestiondomain.RecallChannelLexical, profile.LexicalAlgorithmVersion, LexicalRecallLimit); err != nil {
		return HybridRecallResult{}, err
	}
	if err := appendRecallSignals(&signals, semanticHits, excluded, ingestiondomain.RecallChannelSemantic, profile.SemanticAlgorithmVersion, SemanticRecallLimit); err != nil {
		return HybridRecallResult{}, err
	}
	if err := appendRecallSignals(&signals, structuredHits, excluded, ingestiondomain.RecallChannelStructured, profile.StructuredAlgorithmVersion, StructuredRecallLimit); err != nil {
		return HybridRecallResult{}, err
	}
	fused, err := ingestiondomain.FuseRecallSignals(signals, ReciprocalRankConstant, FusedRecallLimit)
	if err != nil {
		return HybridRecallResult{}, fmt.Errorf("fuse document recall: %w", err)
	}
	candidates := make([]HybridRecallCandidateDTO, 0, len(fused))
	for _, candidate := range fused {
		dto := HybridRecallCandidateDTO{DocumentVersionID: candidate.DocumentVersionID(), RRFScore: candidate.RRFScore()}
		for _, signal := range candidate.Signals() {
			dto.Signals = append(dto.Signals, RecallSignalDTO{
				Channel: string(signal.Channel()), Rank: signal.Rank(), RawScore: signal.RawScore(), AlgorithmVersion: signal.AlgorithmVersion(),
			})
			if signal.Channel() == ingestiondomain.RecallChannelSemantic {
				score := signal.RawScore()
				dto.SemanticScore = &score
			}
		}
		candidates = append(candidates, dto)
	}
	return HybridRecallResult{
		MonitorID: query.MonitorID, Purpose: query.Purpose, ConfigVersionID: query.ConfigVersionID,
		MonitorVersionID: query.MonitorVersionID, PreviewRunID: query.PreviewRunID, CompiledProfileID: query.CompiledProfileID,
		MatchingAlgorithmVersion: profile.MatchingAlgorithmVersion, Candidates: candidates,
		Degraded: len(degradation) > 0, DegradationReasons: sortedUniqueRecallReasons(degradation),
	}, nil
}

func validateReadyRecallProfile(query ReadyRecallProfileDTO) error {
	versions := []string{query.MatchingAlgorithmVersion, query.LexicalAlgorithmVersion, query.SemanticAlgorithmVersion, query.StructuredAlgorithmVersion}
	if err := validateRecallOwner(HybridRecallQuery{
		MonitorID: query.MonitorID, Purpose: query.Purpose, ConfigVersionID: query.ConfigVersionID,
		MonitorVersionID: query.MonitorVersionID, PreviewRunID: query.PreviewRunID,
		DraftID: query.DraftID, DraftResourceVersion: query.DraftResourceVersion,
		CompiledProfileID: query.CompiledProfileID,
	}); err != nil {
		return err
	}
	for _, version := range versions {
		if !semanticVersionPattern.MatchString(strings.TrimSpace(version)) {
			return fmt.Errorf("%w: algorithm version is invalid", ErrInvalidHybridRecallQuery)
		}
	}
	if query.MatchingAlgorithmVersion != HybridRecallMatchingAlgorithmVersion ||
		query.LexicalAlgorithmVersion != LexicalRecallAlgorithmVersion ||
		query.SemanticAlgorithmVersion != SemanticRecallAlgorithmVersion ||
		query.StructuredAlgorithmVersion != StructuredRecallAlgorithmVersion {
		return fmt.Errorf("%w: compiled profile selects an unsupported recall algorithm", ErrInvalidHybridRecallQuery)
	}
	if !semanticVersionPattern.MatchString(strings.TrimSpace(query.SearchNormalizationProfileVersion)) {
		return fmt.Errorf("%w: search normalization profile version is invalid", ErrInvalidHybridRecallQuery)
	}
	if len(query.Clauses) > 128 || len(query.Entities) > 64 {
		return fmt.Errorf("%w: compiled constraints are invalid", ErrInvalidHybridRecallQuery)
	}
	seen := make(map[string]struct{}, len(query.Clauses))
	for _, clause := range query.Clauses {
		clause.Operator = strings.TrimSpace(clause.Operator)
		clause.Field = strings.TrimSpace(clause.Field)
		clause.Value = strings.TrimSpace(clause.Value)
		clause.Origin = strings.TrimSpace(clause.Origin)
		if !validRecallOperator(clause.Operator) || !validRecallField(clause.Field) || !validRecallClauseOrigin(clause.Origin) || clause.Value == "" || len(clause.Value) > 2048 {
			return fmt.Errorf("%w: compiled clause is invalid", ErrInvalidHybridRecallQuery)
		}
		identity := clause.Operator + "\x00" + clause.Field + "\x00" + strings.ToLower(clause.Value)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("%w: compiled clause is duplicated", ErrInvalidHybridRecallQuery)
		}
		seen[identity] = struct{}{}
	}
	for _, entity := range query.Entities {
		if strings.TrimSpace(entity.CanonicalID) == "" || len(entity.CanonicalID) > 256 || len(entity.Aliases) > 32 {
			return fmt.Errorf("%w: compiled entity is invalid", ErrInvalidHybridRecallQuery)
		}
	}
	if query.Semantic != nil {
		if query.Semantic.EmbeddingProfileID <= 0 || query.Semantic.EmbeddingProfileVersion <= 0 ||
			!semanticVersionPattern.MatchString(strings.TrimSpace(query.Semantic.ModelVersion)) || !validRecallVector(query.Semantic.QueryVector) {
			return fmt.Errorf("%w: exact semantic profile is invalid", ErrInvalidHybridRecallQuery)
		}
	}
	switch query.SemanticState {
	case SemanticRecallStateReady:
		if query.Semantic == nil || query.SemanticUnavailableReason != "" {
			return fmt.Errorf("%w: ready semantic profile is incomplete", ErrInvalidHybridRecallQuery)
		}
	case SemanticRecallStateUnavailable:
		if query.Semantic != nil || !validSemanticUnavailableReason(query.SemanticUnavailableReason) {
			return fmt.Errorf("%w: unavailable semantic profile is invalid", ErrInvalidHybridRecallQuery)
		}
	default:
		return fmt.Errorf("%w: semantic state is invalid", ErrInvalidHybridRecallQuery)
	}
	return nil
}

func validSemanticUnavailableReason(value string) bool {
	switch value {
	case "semantic_model_unavailable", "semantic_generation_unavailable", "semantic_policy_unavailable", "semantic_not_requested", "semantic_receipt_unavailable":
		return true
	default:
		return false
	}
}

func validateRecallOwner(query HybridRecallQuery) error {
	if query.MonitorID <= 0 || query.ConfigVersionID <= 0 || query.CompiledProfileID <= 0 {
		return fmt.Errorf("%w: exact monitor, config and compiled profile are required", ErrInvalidHybridRecallQuery)
	}
	switch query.Purpose {
	case "published":
		if query.MonitorVersionID != query.ConfigVersionID || query.PreviewRunID != 0 || query.DraftID != 0 || query.DraftResourceVersion != 0 {
			return fmt.Errorf("%w: published recall owner is invalid", ErrInvalidHybridRecallQuery)
		}
	case "preview":
		if query.MonitorVersionID != 0 || query.PreviewRunID <= 0 || query.DraftID <= 0 || query.DraftResourceVersion <= 0 {
			return fmt.Errorf("%w: preview recall owner is invalid", ErrInvalidHybridRecallQuery)
		}
	default:
		return fmt.Errorf("%w: recall purpose is invalid", ErrInvalidHybridRecallQuery)
	}
	return nil
}

func readyProfileMatchesOwner(profile ReadyRecallProfileDTO, query HybridRecallQuery) bool {
	return profile.MonitorID == query.MonitorID && profile.Purpose == query.Purpose &&
		profile.ConfigVersionID == query.ConfigVersionID && profile.MonitorVersionID == query.MonitorVersionID &&
		profile.PreviewRunID == query.PreviewRunID && profile.DraftID == query.DraftID &&
		profile.DraftResourceVersion == query.DraftResourceVersion && profile.CompiledProfileID == query.CompiledProfileID
}

func validRecallOperator(value string) bool {
	return value == "must" || value == "should" || value == "must_not"
}

func validRecallClauseOrigin(value string) bool {
	return value == "intent_clause" || value == "objective_derived" || value == "approved_candidate"
}

func validRecallField(value string) bool {
	switch value {
	case "term", "phrase", "action", "location", "language", "region", "source", "time_window":
		return true
	default:
		return false
	}
}

func validRecallVector(vector []float32) bool {
	if len(vector) != 1024 {
		return false
	}
	norm := float64(0)
	for _, value := range vector {
		converted := float64(value)
		if math.IsNaN(converted) || math.IsInf(converted, 0) {
			return false
		}
		norm += converted * converted
	}
	return norm > 0
}

func splitRecallClauses(clauses []RecallClauseDTO) (must, should, mustNot []RecallFilterDTO) {
	for _, clause := range clauses {
		item := RecallFilterDTO{Operator: clause.Operator, Field: clause.Field, Value: clause.Value}
		switch clause.Operator {
		case "must":
			must = append(must, item)
		case "should":
			should = append(should, item)
		case "must_not":
			mustNot = append(mustNot, item)
		}
	}
	return must, should, mustNot
}

func copyRecallEntities(values []RecallEntityDTO) []RecallEntityDTO {
	result := make([]RecallEntityDTO, len(values))
	for index, value := range values {
		result[index] = RecallEntityDTO{CanonicalID: value.CanonicalID, Aliases: append([]string(nil), value.Aliases...)}
	}
	return result
}

func collectHardExcluded(groups ...[]RecallHitDTO) map[int64]struct{} {
	result := make(map[int64]struct{})
	for _, hits := range groups {
		for _, hit := range hits {
			if hit.HardExcluded && hit.DocumentVersionID > 0 {
				result[hit.DocumentVersionID] = struct{}{}
			}
		}
	}
	return result
}

func appendRecallSignals(target *[]ingestiondomain.RecallSignal, hits []RecallHitDTO, excluded map[int64]struct{}, channel ingestiondomain.RecallChannel, algorithmVersion string, limit int) error {
	for _, hit := range hits {
		if hit.HardExcluded {
			continue
		}
		if _, remove := excluded[hit.DocumentVersionID]; remove {
			continue
		}
		signal, err := ingestiondomain.NewRecallSignal(hit.DocumentVersionID, channel, hit.Rank, hit.RawScore, algorithmVersion)
		if err != nil {
			return fmt.Errorf("%w: invalid %s signal", ErrInvalidHybridRecallQuery, channel)
		}
		*target = append(*target, signal)
	}
	return nil
}

func validateRecallHitBatch(hits []RecallHitDTO, limit int, channel ingestiondomain.RecallChannel) error {
	if len(hits) > limit {
		return fmt.Errorf("%w: %s reader exceeded its bound", ErrInvalidHybridRecallQuery, channel)
	}
	seenDocuments := make(map[int64]struct{}, len(hits))
	seenRanks := make(map[int]struct{}, len(hits))
	for _, hit := range hits {
		if hit.DocumentVersionID <= 0 || hit.Rank < 1 || hit.Rank > limit || math.IsNaN(hit.RawScore) || math.IsInf(hit.RawScore, 0) {
			return fmt.Errorf("%w: %s hit is invalid", ErrInvalidHybridRecallQuery, channel)
		}
		if hit.HardExcluded && len(hit.ExclusionReasons) == 0 {
			return fmt.Errorf("%w: hard exclusion is unexplained", ErrInvalidHybridRecallQuery)
		}
		if _, duplicate := seenDocuments[hit.DocumentVersionID]; duplicate {
			return fmt.Errorf("%w: %s returned a duplicate document", ErrInvalidHybridRecallQuery, channel)
		}
		if _, duplicate := seenRanks[hit.Rank]; duplicate {
			return fmt.Errorf("%w: %s returned a duplicate rank", ErrInvalidHybridRecallQuery, channel)
		}
		seenDocuments[hit.DocumentVersionID] = struct{}{}
		seenRanks[hit.Rank] = struct{}{}
	}
	return nil
}

// keep deterministic degradation and output orders if more optional channels
// are introduced later.
func sortedUniqueRecallReasons(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; !duplicate {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
