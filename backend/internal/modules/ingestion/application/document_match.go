package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

var (
	ErrInvalidDocumentMatchContract     = errors.New("document match contract is invalid")
	ErrDocumentMatchRerankerUnavailable = errors.New("document match reranker is unavailable")
	ErrDocumentMatchAuthorizationDenied = errors.New("document match review is not authorized")
)

type RelevanceDecisionProfileDTO struct {
	ID, Version, EvaluationRunID              int64
	MatchingAlgorithmVersion, RerankerVersion string
	CalibrationVersion                        string
	Status                                    string
	RejectThreshold, AcceptThreshold          float64
	CalibrationSlope, CalibrationIntercept    float64
}

type ReadRelevanceDecisionProfileQuery struct{ RelevanceProfileID int64 }

type DocumentMatchSignalDTO struct {
	Channel, AlgorithmVersion string
	Rank                      int
	RawScore                  float64
}

type DocumentMatchDecisionDTO struct {
	ID, MonitorID, MonitorVersionID, CompiledProfileID int64
	DocumentVersionID, RelevanceProfileID              int64
	MatchingAlgorithmVersion, RerankerVersion          string
	CalibrationVersion, InputHash                      string
	RRFScore                                           float64
	RelevanceProbability                               *float64
	Decision                                           string
	Degraded                                           bool
	ReasonCodes                                        []string
	Signals                                            []DocumentMatchSignalDTO
	CreatedAt                                          time.Time
}

type PersistAutomaticDocumentMatchCommand struct {
	MonitorID, MonitorVersionID, CompiledProfileID int64
	DocumentVersionID, RelevanceProfileID          int64
	MatchingAlgorithmVersion, RerankerVersion      string
	CalibrationVersion, InputHash                  string
	RRFScore                                       float64
	RelevanceProbability                           *float64
	Decision                                       string
	Degraded                                       bool
	ReasonCodes                                    []string
	Signals                                        []DocumentMatchSignalDTO
	DecidedAt                                      time.Time
}

type DocumentMatchOverrideDTO struct {
	ID, MatchDecisionID, Sequence, MonitorID, MonitorVersionID, DocumentVersionID int64
	Decision, PreviousEffectiveDecision, ReasonCode, Note                         string
	ActorUserID                                                                   int64
	CreatedAt                                                                     time.Time
}

type AppendDocumentMatchOverrideCommand struct {
	ActorUserID, MonitorID, MatchDecisionID int64
	ExpectedSequence                        int64
	Decision, ReasonCode, Note              string
	IdempotencyKey, CommandFingerprint      string
	DecidedAt                               time.Time
}

type AuthorizeDocumentMatchReviewQuery struct {
	ActorUserID, MonitorID, MatchDecisionID int64
}

type EvaluatePublishedDocumentMatchesCommand struct {
	MonitorID, MonitorVersionID, CompiledProfileID, RelevanceProfileID int64
}

type EvaluatePublishedDocumentMatchesResult struct{ Decisions []DocumentMatchDecisionDTO }

type OverrideDocumentMatchCommand struct {
	ActorUserID, MonitorID, MatchDecisionID int64
	ExpectedSequence                        int64
	Decision, ReasonCode, Note              string
	IdempotencyKey                          string
}

type OverrideDocumentMatchResult struct {
	Override DocumentMatchOverrideDTO
	Reused   bool
}

// ScheduleAcceptedDocumentMatchProjectionCommand contains only immutable
// database identities. The queue adapter must reread every clustering and
// evidence fact from its owning module.
type ScheduleAcceptedDocumentMatchProjectionCommand struct {
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
	EffectiveSequence       int64
}

type ScheduleAcceptedDocumentMatchProjectionResult struct {
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
	EffectiveSequence       int64
	JobID                   int64
	Created                 bool
}

type AcceptedDocumentMatchProjectionScheduler interface {
	ScheduleAcceptedDocumentMatchProjection(context.Context, ScheduleAcceptedDocumentMatchProjectionCommand) (ScheduleAcceptedDocumentMatchProjectionResult, error)
}

type PublishedDocumentRecall interface {
	Recall(context.Context, HybridRecallQuery) (HybridRecallResult, error)
}

type RelevanceDecisionProfileReader interface {
	ReadRelevanceDecisionProfile(context.Context, ReadRelevanceDecisionProfileQuery) (RelevanceDecisionProfileDTO, error)
}

type DocumentMatchRepository interface {
	PersistAutomaticDocumentMatches(context.Context, []PersistAutomaticDocumentMatchCommand) ([]DocumentMatchDecisionDTO, error)
	AppendDocumentMatchOverride(context.Context, AppendDocumentMatchOverrideCommand) (DocumentMatchOverrideDTO, bool, error)
}

type DocumentMatchReviewAuthorizer interface {
	AuthorizeDocumentMatchReview(context.Context, AuthorizeDocumentMatchReviewQuery) error
}

type DocumentMatchClock interface{ Now() time.Time }

// DocumentMatchReranker is optional only while the selected profile is not
// active. It must return one exact probability per candidate and cannot invent
// a candidate outside the Hybrid Recall result.
type DocumentMatchReranker interface {
	RerankDocumentMatches(context.Context, RerankDocumentMatchesQuery) ([]RerankedDocumentMatchDTO, error)
}

type RerankDocumentMatchesQuery struct {
	MonitorID, MonitorVersionID, CompiledProfileID, RelevanceProfileID int64
	MatchingAlgorithmVersion, RerankerVersion, CalibrationVersion      string
	CalibrationSlope, CalibrationIntercept                             float64
	Candidates                                                         []HybridRecallCandidateDTO
}

type RerankedDocumentMatchDTO struct {
	DocumentVersionID    int64
	RelevanceProbability float64
	ReasonCodes          []string
	Degraded             bool
}

type PublishedDocumentMatchService struct {
	recall     PublishedDocumentRecall
	profiles   RelevanceDecisionProfileReader
	repository DocumentMatchRepository
	reranker   DocumentMatchReranker
	clock      DocumentMatchClock
}

func NewPublishedDocumentMatchService(recall PublishedDocumentRecall, profiles RelevanceDecisionProfileReader, repository DocumentMatchRepository, reranker DocumentMatchReranker, clock DocumentMatchClock) (*PublishedDocumentMatchService, error) {
	if recall == nil || profiles == nil || repository == nil || clock == nil {
		return nil, fmt.Errorf("%w: published match dependencies are required", ErrInvalidDocumentMatchContract)
	}
	return &PublishedDocumentMatchService{recall: recall, profiles: profiles, repository: repository, reranker: reranker, clock: clock}, nil
}

func (service *PublishedDocumentMatchService) EvaluatePublished(ctx context.Context, command EvaluatePublishedDocumentMatchesCommand) (EvaluatePublishedDocumentMatchesResult, error) {
	if service == nil || command.MonitorID <= 0 || command.MonitorVersionID <= 0 || command.CompiledProfileID <= 0 || command.RelevanceProfileID <= 0 {
		return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
	}
	profileDTO, err := service.profiles.ReadRelevanceDecisionProfile(ctx, ReadRelevanceDecisionProfileQuery{RelevanceProfileID: command.RelevanceProfileID})
	if err != nil {
		return EvaluatePublishedDocumentMatchesResult{}, err
	}
	profile, err := relevanceProfileFromDTO(profileDTO)
	if err != nil || profile.MatchingAlgorithmVersion != HybridRecallMatchingAlgorithmVersion {
		return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
	}
	if profile.Status == ingestiondomain.RelevanceProfileActive && service.reranker == nil {
		return EvaluatePublishedDocumentMatchesResult{}, ErrDocumentMatchRerankerUnavailable
	}
	recalled, err := service.recall.Recall(ctx, HybridRecallQuery{
		MonitorID: command.MonitorID, Purpose: "published", ConfigVersionID: command.MonitorVersionID,
		MonitorVersionID: command.MonitorVersionID, CompiledProfileID: command.CompiledProfileID,
	})
	if err != nil {
		return EvaluatePublishedDocumentMatchesResult{}, err
	}
	if recalled.MonitorID != command.MonitorID || recalled.Purpose != "published" || recalled.ConfigVersionID != command.MonitorVersionID ||
		recalled.MonitorVersionID != command.MonitorVersionID || recalled.CompiledProfileID != command.CompiledProfileID ||
		recalled.MatchingAlgorithmVersion != profile.MatchingAlgorithmVersion {
		return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
	}
	reranked := make(map[int64]RerankedDocumentMatchDTO, len(recalled.Candidates))
	if service.reranker != nil && len(recalled.Candidates) > 0 {
		candidateIDs := make(map[int64]struct{}, len(recalled.Candidates))
		for _, candidate := range recalled.Candidates {
			if candidate.DocumentVersionID <= 0 {
				return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
			}
			candidateIDs[candidate.DocumentVersionID] = struct{}{}
		}
		values, rerankErr := service.reranker.RerankDocumentMatches(ctx, RerankDocumentMatchesQuery{
			MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
			CompiledProfileID: command.CompiledProfileID, RelevanceProfileID: command.RelevanceProfileID,
			MatchingAlgorithmVersion: profile.MatchingAlgorithmVersion, RerankerVersion: profile.RerankerVersion,
			CalibrationVersion: profile.CalibrationVersion, CalibrationSlope: profile.CalibrationSlope,
			CalibrationIntercept: profile.CalibrationIntercept,
			Candidates:           cloneHybridRecallCandidates(recalled.Candidates),
		})
		if rerankErr != nil {
			if profile.Status == ingestiondomain.RelevanceProfileActive {
				return EvaluatePublishedDocumentMatchesResult{}, rerankErr
			}
		} else {
			if len(values) != len(candidateIDs) {
				return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
			}
			for _, value := range values {
				if _, recalledCandidate := candidateIDs[value.DocumentVersionID]; !recalledCandidate ||
					math.IsNaN(value.RelevanceProbability) || math.IsInf(value.RelevanceProbability, 0) ||
					value.RelevanceProbability < 0 || value.RelevanceProbability > 1 {
					return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
				}
				if _, duplicate := reranked[value.DocumentVersionID]; duplicate {
					return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
				}
				reranked[value.DocumentVersionID] = value
			}
		}
	}
	decidedAt := service.clock.Now().UTC()
	if decidedAt.IsZero() {
		return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
	}
	commands := make([]PersistAutomaticDocumentMatchCommand, 0, len(recalled.Candidates))
	for _, candidate := range recalled.Candidates {
		probability, degraded := (*float64)(nil), recalled.Degraded
		reasons := append([]string{}, recalled.DegradationReasons...)
		if rerankedValue, found := reranked[candidate.DocumentVersionID]; found {
			value := normalizeDocumentMatchProbability(rerankedValue.RelevanceProbability)
			probability = &value
			degraded = degraded || rerankedValue.Degraded
			reasons = append(reasons, rerankedValue.ReasonCodes...)
		} else {
			degraded = true
			reasons = append(reasons, "relevance_reranker_unavailable")
		}
		decision, decideErr := ingestiondomain.DecideDocumentMatch(profile, probability, degraded, false)
		if decideErr != nil {
			return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
		}
		signals, signalErr := documentMatchSignals(candidate.Signals)
		if signalErr != nil {
			return EvaluatePublishedDocumentMatchesResult{}, signalErr
		}
		reasons, reasonErr := sortedDocumentMatchReasons(reasons)
		if reasonErr != nil {
			return EvaluatePublishedDocumentMatchesResult{}, reasonErr
		}
		persist := PersistAutomaticDocumentMatchCommand{
			MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID, CompiledProfileID: command.CompiledProfileID,
			DocumentVersionID: candidate.DocumentVersionID, RelevanceProfileID: command.RelevanceProfileID,
			MatchingAlgorithmVersion: profile.MatchingAlgorithmVersion, RerankerVersion: profile.RerankerVersion,
			CalibrationVersion: profile.CalibrationVersion, RelevanceProbability: probability, Decision: string(decision),
			RRFScore: normalizeDocumentMatchScore(candidate.RRFScore), Degraded: degraded, ReasonCodes: reasons, Signals: signals, DecidedAt: decidedAt,
		}
		persist.InputHash = documentMatchInputHash(persist)
		commands = append(commands, persist)
	}
	decisions, err := service.repository.PersistAutomaticDocumentMatches(ctx, commands)
	if err != nil {
		return EvaluatePublishedDocumentMatchesResult{}, err
	}
	if len(decisions) != len(commands) {
		return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
	}
	for index := range decisions {
		if !sameDocumentMatchDecisionReceipt(decisions[index], commands[index]) {
			return EvaluatePublishedDocumentMatchesResult{}, ErrInvalidDocumentMatchContract
		}
	}
	return EvaluatePublishedDocumentMatchesResult{Decisions: decisions}, nil
}

func sameDocumentMatchDecisionReceipt(value DocumentMatchDecisionDTO, command PersistAutomaticDocumentMatchCommand) bool {
	if value.ID <= 0 || value.CreatedAt.IsZero() || value.MonitorID != command.MonitorID || value.MonitorVersionID != command.MonitorVersionID ||
		value.CompiledProfileID != command.CompiledProfileID || value.DocumentVersionID != command.DocumentVersionID ||
		value.RelevanceProfileID != command.RelevanceProfileID || value.MatchingAlgorithmVersion != command.MatchingAlgorithmVersion ||
		value.RerankerVersion != command.RerankerVersion || value.CalibrationVersion != command.CalibrationVersion ||
		value.InputHash != command.InputHash || value.RRFScore != command.RRFScore || value.Decision != command.Decision ||
		value.Degraded != command.Degraded || !sameDocumentMatchProbability(value.RelevanceProbability, command.RelevanceProbability) ||
		len(value.ReasonCodes) != len(command.ReasonCodes) || len(value.Signals) != len(command.Signals) {
		return false
	}
	for index := range value.ReasonCodes {
		if value.ReasonCodes[index] != command.ReasonCodes[index] {
			return false
		}
	}
	for index := range value.Signals {
		if value.Signals[index] != command.Signals[index] {
			return false
		}
	}
	return true
}

func sameDocumentMatchProbability(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

type DocumentMatchReviewService struct {
	repository DocumentMatchRepository
	authorizer DocumentMatchReviewAuthorizer
	clock      DocumentMatchClock
}

func NewDocumentMatchReviewService(repository DocumentMatchRepository, authorizer DocumentMatchReviewAuthorizer, clock DocumentMatchClock) (*DocumentMatchReviewService, error) {
	if repository == nil || authorizer == nil || clock == nil {
		return nil, fmt.Errorf("%w: match review dependencies are required", ErrInvalidDocumentMatchContract)
	}
	return &DocumentMatchReviewService{repository: repository, authorizer: authorizer, clock: clock}, nil
}

func (service *DocumentMatchReviewService) Override(ctx context.Context, command OverrideDocumentMatchCommand) (OverrideDocumentMatchResult, error) {
	if service == nil || command.ActorUserID <= 0 || command.MonitorID <= 0 || command.MatchDecisionID <= 0 || command.ExpectedSequence < 0 ||
		(command.Decision != "accepted" && command.Decision != "rejected") || !validDocumentMatchReason(command.ReasonCode) ||
		!validDocumentMatchIdempotencyKey(command.IdempotencyKey) || !validDocumentMatchNote(command.Note) {
		return OverrideDocumentMatchResult{}, ErrInvalidDocumentMatchContract
	}
	if err := service.authorizer.AuthorizeDocumentMatchReview(ctx, AuthorizeDocumentMatchReviewQuery{
		ActorUserID: command.ActorUserID, MonitorID: command.MonitorID, MatchDecisionID: command.MatchDecisionID,
	}); err != nil {
		return OverrideDocumentMatchResult{}, err
	}
	decidedAt := service.clock.Now().UTC()
	if decidedAt.IsZero() {
		return OverrideDocumentMatchResult{}, ErrInvalidDocumentMatchContract
	}
	mutation := AppendDocumentMatchOverrideCommand{
		ActorUserID: command.ActorUserID, MonitorID: command.MonitorID, MatchDecisionID: command.MatchDecisionID,
		ExpectedSequence: command.ExpectedSequence,
		Decision:         command.Decision, ReasonCode: command.ReasonCode, Note: strings.TrimSpace(command.Note),
		IdempotencyKey: command.IdempotencyKey, DecidedAt: decidedAt,
	}
	mutation.CommandFingerprint = documentMatchOverrideFingerprint(mutation)
	override, reused, err := service.repository.AppendDocumentMatchOverride(ctx, mutation)
	if err != nil {
		return OverrideDocumentMatchResult{}, err
	}
	if override.ID <= 0 || override.MatchDecisionID != command.MatchDecisionID || override.MonitorID != command.MonitorID ||
		override.MonitorVersionID <= 0 || override.DocumentVersionID <= 0 || override.Sequence != command.ExpectedSequence+1 ||
		override.Decision != command.Decision || override.ReasonCode != command.ReasonCode || override.Note != strings.TrimSpace(command.Note) ||
		override.ActorUserID != command.ActorUserID || override.CreatedAt.IsZero() {
		return OverrideDocumentMatchResult{}, ErrInvalidDocumentMatchContract
	}
	return OverrideDocumentMatchResult{Override: override, Reused: reused}, nil
}

func relevanceProfileFromDTO(value RelevanceDecisionProfileDTO) (ingestiondomain.RelevanceDecisionProfile, error) {
	profile := ingestiondomain.RelevanceDecisionProfile{
		ID: value.ID, Version: value.Version, EvaluationRunID: value.EvaluationRunID, MatchingAlgorithmVersion: value.MatchingAlgorithmVersion,
		RerankerVersion: value.RerankerVersion, CalibrationVersion: value.CalibrationVersion,
		Status: ingestiondomain.RelevanceProfileStatus(value.Status), RejectThreshold: value.RejectThreshold, AcceptThreshold: value.AcceptThreshold,
		CalibrationSlope: value.CalibrationSlope, CalibrationIntercept: value.CalibrationIntercept,
	}
	return profile, profile.Validate()
}

func documentMatchSignals(values []RecallSignalDTO) ([]DocumentMatchSignalDTO, error) {
	result := make([]DocumentMatchSignalDTO, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value.Channel == "" || value.Rank <= 0 || value.AlgorithmVersion == "" {
			return nil, ErrInvalidDocumentMatchContract
		}
		if _, duplicate := seen[value.Channel]; duplicate {
			return nil, ErrInvalidDocumentMatchContract
		}
		seen[value.Channel] = struct{}{}
		result[index] = DocumentMatchSignalDTO{Channel: value.Channel, Rank: value.Rank, RawScore: normalizeDocumentMatchScore(value.RawScore), AlgorithmVersion: value.AlgorithmVersion}
	}
	return result, nil
}

func documentMatchInputHash(command PersistAutomaticDocumentMatchCommand) string {
	parts := []string{
		"document-match-input-v1", strconv.FormatInt(command.MonitorID, 10), strconv.FormatInt(command.MonitorVersionID, 10),
		strconv.FormatInt(command.CompiledProfileID, 10), strconv.FormatInt(command.DocumentVersionID, 10),
		strconv.FormatInt(command.RelevanceProfileID, 10), command.MatchingAlgorithmVersion, command.RerankerVersion,
		command.CalibrationVersion, command.Decision, strconv.FormatBool(command.Degraded),
		strconv.FormatFloat(command.RRFScore, 'g', -1, 64),
	}
	if command.RelevanceProbability == nil {
		parts = append(parts, "probability:null")
	} else {
		parts = append(parts, "probability:"+strconv.FormatFloat(*command.RelevanceProbability, 'g', -1, 64))
	}
	parts = append(parts, command.ReasonCodes...)
	for _, signal := range command.Signals {
		parts = append(parts, signal.Channel, strconv.Itoa(signal.Rank), strconv.FormatFloat(signal.RawScore, 'g', -1, 64), signal.AlgorithmVersion)
	}
	return lengthPrefixedDocumentMatchHash(parts)
}

func documentMatchOverrideFingerprint(command AppendDocumentMatchOverrideCommand) string {
	return lengthPrefixedDocumentMatchHash([]string{
		"document-match-override-v1", strconv.FormatInt(command.ActorUserID, 10), strconv.FormatInt(command.MonitorID, 10),
		strconv.FormatInt(command.MatchDecisionID, 10), strconv.FormatInt(command.ExpectedSequence, 10), command.Decision, command.ReasonCode, command.Note,
	})
}

func lengthPrefixedDocumentMatchHash(parts []string) string {
	digest := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprintf(digest, "%d:%s\n", len([]byte(part)), part)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func sortedDocumentMatchReasons(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validDocumentMatchReason(value) {
			return nil, ErrInvalidDocumentMatchContract
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func validDocumentMatchReason(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 64 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '_' || character == ':' || character == '-') {
			continue
		}
		return false
	}
	return true
}

func validDocumentMatchIdempotencyKey(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 128 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validDocumentMatchNote(value string) bool {
	value = strings.TrimSpace(value)
	if len([]byte(value)) > 8000 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return false
		}
	}
	return true
}

func cloneHybridRecallCandidates(values []HybridRecallCandidateDTO) []HybridRecallCandidateDTO {
	result := make([]HybridRecallCandidateDTO, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Signals = append([]RecallSignalDTO(nil), value.Signals...)
	}
	return result
}

func documentMatchDecisionFromCommand(command PersistAutomaticDocumentMatchCommand, id int64) DocumentMatchDecisionDTO {
	return DocumentMatchDecisionDTO{
		ID: id, MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
		CompiledProfileID: command.CompiledProfileID, DocumentVersionID: command.DocumentVersionID,
		RelevanceProfileID: command.RelevanceProfileID, MatchingAlgorithmVersion: command.MatchingAlgorithmVersion,
		RerankerVersion: command.RerankerVersion, CalibrationVersion: command.CalibrationVersion,
		InputHash: command.InputHash, RRFScore: command.RRFScore, RelevanceProbability: command.RelevanceProbability, Decision: command.Decision,
		Degraded: command.Degraded, ReasonCodes: append([]string(nil), command.ReasonCodes...),
		Signals: append([]DocumentMatchSignalDTO(nil), command.Signals...), CreatedAt: command.DecidedAt,
	}
}

func containsDocumentMatchReason(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func normalizeDocumentMatchScore(value float64) float64 {
	return math.Round(value*1e10) / 1e10
}

func normalizeDocumentMatchProbability(value float64) float64 {
	return math.Round(value*1e7) / 1e7
}
