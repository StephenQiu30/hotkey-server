package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
)

var intentProfilePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,63}$`)

// IntentExpansionProfile names the implemented normalization, filtering, and
// risk-interpretation parameters. It is not an AI model selector; the actual
// selected model is recorded only by the completed Intelligence run.
const IntentExpansionProfile = "monitor-intent-expansion-v1"

type IntentService struct {
	drafts    IntentDraftRepository
	revisions IntentDraftRevisionRepository
	runs      IntentRunRepository
	tasks     IntentAnalysisTaskRepository
	clock     IntentClock
}

func NewIntentService(dependencies IntentServiceDependencies) (*IntentService, error) {
	if dependencies.Drafts == nil || dependencies.Runs == nil || dependencies.Clock == nil {
		return nil, fmt.Errorf("%w: intent service dependencies are required", ErrInvalidIntentContract)
	}
	if dependencies.Revisions == nil {
		dependencies.Revisions, _ = dependencies.Drafts.(IntentDraftRevisionRepository)
	}
	if dependencies.Tasks == nil {
		dependencies.Tasks, _ = dependencies.Runs.(IntentAnalysisTaskRepository)
	}
	return &IntentService{
		drafts: dependencies.Drafts, revisions: dependencies.Revisions,
		runs: dependencies.Runs, tasks: dependencies.Tasks, clock: dependencies.Clock,
	}, nil
}

func (service *IntentService) ReadDraft(ctx context.Context, query ReadIntentDraftQuery) (ReadIntentDraftResult, error) {
	draft, err := service.loadDraft(ctx, query.MonitorID, query.DraftID)
	if err != nil {
		return ReadIntentDraftResult{}, err
	}
	return ReadIntentDraftResult{Draft: intentDraftToDTO(draft)}, nil
}

func (service *IntentService) ReplaceDraft(ctx context.Context, command ReplaceIntentDraftCommand) (ReplaceIntentDraftResult, error) {
	if command.MonitorID <= 0 || command.DraftID <= 0 || command.ExpectedResourceVersion <= 0 {
		return ReplaceIntentDraftResult{}, ErrInvalidIntentContract
	}
	definition, err := intentDefinitionFromDTO(command.Objective, command.Clauses, command.Entities, command.Examples)
	if err != nil {
		return ReplaceIntentDraftResult{}, err
	}
	current, err := service.loadDraft(ctx, command.MonitorID, command.DraftID)
	if err != nil {
		return ReplaceIntentDraftResult{}, err
	}
	next, err := current.ReplaceDefinition(command.ExpectedResourceVersion, definition)
	if err != nil {
		return ReplaceIntentDraftResult{}, translateIntentDomainError(err)
	}
	persisted, err := service.saveDraftMutation(ctx, IntentDraftMutationReplace, current.ResourceVersion(), next)
	if err != nil {
		return ReplaceIntentDraftResult{}, err
	}
	return ReplaceIntentDraftResult{Draft: intentDraftToDTO(persisted)}, nil
}

func (service *IntentService) ReviewCandidate(ctx context.Context, command ReviewExpansionCandidateCommand) (ReviewExpansionCandidateResult, error) {
	if command.MonitorID <= 0 || command.DraftID <= 0 || command.ExpectedResourceVersion <= 0 || command.CandidateID == "" || command.ReviewerUserID <= 0 {
		return ReviewExpansionCandidateResult{}, ErrInvalidIntentContract
	}
	idempotencyKey, err := validateIntentIdempotencyKey(command.IdempotencyKey)
	if err != nil {
		return ReviewExpansionCandidateResult{}, err
	}
	commandFingerprint := intentRunHash(
		"candidate-review-command-v1", strconv.FormatInt(command.MonitorID, 10),
		strconv.FormatInt(command.DraftID, 10), command.CandidateID, strconv.FormatInt(command.ExpectedResourceVersion, 10),
		command.Decision, strconv.FormatInt(command.ReviewerUserID, 10), command.Note,
	)
	prior, err := service.drafts.FindMutation(ctx, IntentDraftMutationLookupDTO{MonitorID: command.MonitorID, DraftID: command.DraftID, IdempotencyKey: idempotencyKey})
	if err == nil {
		persisted, receiptErr := validateIntentMutationReceipt(prior, command.MonitorID, command.DraftID, commandFingerprint, nil)
		if receiptErr != nil {
			return ReviewExpansionCandidateResult{}, receiptErr
		}
		return ReviewExpansionCandidateResult{Draft: intentDraftToDTO(persisted), Reused: true}, nil
	}
	if !errors.Is(err, ErrIntentMutationNotFound) {
		return ReviewExpansionCandidateResult{}, err
	}
	current, err := service.loadDraft(ctx, command.MonitorID, command.DraftID)
	if err != nil {
		return ReviewExpansionCandidateResult{}, err
	}
	now, err := service.now()
	if err != nil {
		return ReviewExpansionCandidateResult{}, err
	}
	next, err := current.ReviewCandidate(
		command.ExpectedResourceVersion, command.CandidateID,
		domain.ExpansionDecision(command.Decision), command.ReviewerUserID, now, command.Note,
	)
	if err != nil {
		return ReviewExpansionCandidateResult{}, translateIntentDomainError(err)
	}
	receipt, err := service.drafts.SaveAndInvalidateRuns(ctx, IntentDraftMutationDTO{
		Kind: IntentDraftMutationCandidateReview, ExpectedDraftID: current.DraftID(), ExpectedResourceVersion: current.ResourceVersion(),
		Next: intentDraftToDTO(next), InvalidatedAt: now,
		IdempotencyKey: idempotencyKey, CommandFingerprint: commandFingerprint,
	})
	if err != nil {
		return ReviewExpansionCandidateResult{}, err
	}
	persisted, err := validateIntentMutationReceipt(receipt, command.MonitorID, command.DraftID, commandFingerprint, &next)
	if err != nil {
		return ReviewExpansionCandidateResult{}, err
	}
	return ReviewExpansionCandidateResult{Draft: intentDraftToDTO(persisted), Reused: !receipt.Created}, nil
}

func (service *IntentService) SubmitExpansionRun(ctx context.Context, command SubmitExpansionRunCommand) (SubmitExpansionRunResult, error) {
	draft, err := service.versionedDraft(ctx, command.MonitorID, command.DraftID, command.ExpectedResourceVersion)
	if err != nil {
		return SubmitExpansionRunResult{}, err
	}
	expansionProfile, err := validateIntentExpansionProfile(command.ExpansionProfile)
	if err != nil {
		return SubmitExpansionRunResult{}, err
	}
	inputHash := intentExpansionInputHash(draft, expansionProfile)
	run, reused, err := service.reserveRun(ctx, command.IdempotencyKey, domain.IntentRunExpansion, draft, inputHash, expansionProfile, 0)
	if err != nil {
		return SubmitExpansionRunResult{}, err
	}
	return SubmitExpansionRunResult{Run: run, Reused: reused}, nil
}

func (service *IntentService) SubmitPreviewRun(ctx context.Context, command SubmitPreviewRunCommand) (SubmitPreviewRunResult, error) {
	draft, err := service.versionedDraft(ctx, command.MonitorID, command.DraftID, command.ExpectedResourceVersion)
	if err != nil {
		return SubmitPreviewRunResult{}, err
	}
	profile, err := validateIntentProfile(command.EvaluatorProfile)
	if err != nil {
		return SubmitPreviewRunResult{}, err
	}
	if command.SampleLimit < 1 || command.SampleLimit > 200 {
		return SubmitPreviewRunResult{}, ErrInvalidIntentContract
	}
	inputHash := intentRunHash(
		"preview-input-v1", strconv.FormatInt(draft.DraftID(), 10),
		strconv.FormatInt(draft.ResourceVersion(), 10), draft.MatchingFingerprint(), profile,
		strconv.Itoa(command.SampleLimit),
	)
	run, reused, err := service.reserveRun(ctx, command.IdempotencyKey, domain.IntentRunPreview, draft, inputHash, profile, command.SampleLimit)
	if err != nil {
		return SubmitPreviewRunResult{}, err
	}
	return SubmitPreviewRunResult{Run: run, Reused: reused}, nil
}

func (service *IntentService) ReadExpansionRun(ctx context.Context, query ReadExpansionRunQuery) (ReadExpansionRunResult, error) {
	if err := validateIntentRunQuery(query.MonitorID, query.DraftID, query.DraftResourceVersion, query.RunID); err != nil {
		return ReadExpansionRunResult{}, err
	}
	stored, err := service.runs.FindExpansion(ctx, query)
	if err != nil {
		return ReadExpansionRunResult{}, err
	}
	run, err := validateStoredIntentRun(stored.Run, query.MonitorID, query.DraftID, query.DraftResourceVersion, query.RunID, domain.IntentRunExpansion)
	if err != nil {
		return ReadExpansionRunResult{}, err
	}
	if err := validateIntentRunResultVisibility(run, len(stored.Candidates) != 0, false); err != nil {
		return ReadExpansionRunResult{}, err
	}
	candidates := make([]ExpansionCandidateDTO, 0, len(stored.Candidates))
	for _, item := range stored.Candidates {
		candidate, candidateErr := expansionCandidateFromDTO(item)
		if candidateErr != nil {
			return ReadExpansionRunResult{}, candidateErr
		}
		if candidate.Provenance().InputHash() != run.InputHash() {
			return ReadExpansionRunResult{}, invalidIntentContract(fmt.Errorf("expansion candidate belongs to another run input"))
		}
		candidates = append(candidates, expansionCandidateToDTO(candidate))
	}
	return ReadExpansionRunResult{Expansion: ExpansionRunDTO{Run: intentRunToDTO(run), Candidates: candidates}}, nil
}

func (service *IntentService) ReadPreviewRun(ctx context.Context, query ReadPreviewRunQuery) (ReadPreviewRunResult, error) {
	if err := validateIntentRunQuery(query.MonitorID, query.DraftID, query.DraftResourceVersion, query.RunID); err != nil {
		return ReadPreviewRunResult{}, err
	}
	stored, err := service.runs.FindPreview(ctx, query)
	if err != nil {
		return ReadPreviewRunResult{}, err
	}
	run, err := validateStoredIntentRun(stored.Run, query.MonitorID, query.DraftID, query.DraftResourceVersion, query.RunID, domain.IntentRunPreview)
	if err != nil {
		return ReadPreviewRunResult{}, err
	}
	if err := validateIntentRunResultVisibility(run, stored.Preview != nil, true); err != nil {
		return ReadPreviewRunResult{}, err
	}
	if err := validatePreviewDTO(stored.Preview); err != nil {
		return ReadPreviewRunResult{}, err
	}
	canonical := clonePreviewRunDTO(PreviewRunDTO{Run: intentRunToDTO(run), Preview: stored.Preview})
	return ReadPreviewRunResult{Preview: canonical}, nil
}

func (service *IntentService) loadDraft(ctx context.Context, monitorID, draftID int64) (domain.IntentDraft, error) {
	if monitorID <= 0 || draftID <= 0 {
		return domain.IntentDraft{}, ErrInvalidIntentContract
	}
	stored, err := service.drafts.Find(ctx, ReadIntentDraftQuery{MonitorID: monitorID, DraftID: draftID})
	if err != nil {
		return domain.IntentDraft{}, err
	}
	draft, err := intentDraftFromDTO(stored)
	if err != nil {
		return domain.IntentDraft{}, err
	}
	if draft.MonitorID() != monitorID || draft.DraftID() != draftID {
		return domain.IntentDraft{}, invalidIntentContract(fmt.Errorf("repository returned another monitor's draft"))
	}
	return draft, nil
}

func (service *IntentService) versionedDraft(ctx context.Context, monitorID, expectedDraftID, expectedResourceVersion int64) (domain.IntentDraft, error) {
	if expectedDraftID <= 0 || expectedResourceVersion <= 0 {
		return domain.IntentDraft{}, ErrInvalidIntentContract
	}
	draft, err := service.loadDraft(ctx, monitorID, expectedDraftID)
	if err != nil {
		return domain.IntentDraft{}, err
	}
	if draft.DraftID() != expectedDraftID || draft.ResourceVersion() != expectedResourceVersion {
		return domain.IntentDraft{}, ErrIntentVersionConflict
	}
	return draft, nil
}

func (service *IntentService) saveDraftMutation(ctx context.Context, kind IntentDraftMutationKind, expectedVersion int64, next domain.IntentDraft) (domain.IntentDraft, error) {
	now, err := service.now()
	if err != nil {
		return domain.IntentDraft{}, err
	}
	canonicalNext := intentDraftToDTO(next)
	receipt, err := service.drafts.SaveAndInvalidateRuns(ctx, IntentDraftMutationDTO{
		Kind: kind, ExpectedDraftID: next.DraftID(), ExpectedResourceVersion: expectedVersion,
		Next: canonicalNext, InvalidatedAt: now,
	})
	if err != nil {
		return domain.IntentDraft{}, err
	}
	if !receipt.Created || receipt.CommandFingerprint != "" {
		return domain.IntentDraft{}, invalidIntentContract(fmt.Errorf("non-idempotent mutation returned an invalid receipt"))
	}
	persisted, err := intentDraftFromDTO(receipt.Draft)
	if err != nil {
		return domain.IntentDraft{}, err
	}
	if !reflect.DeepEqual(intentDraftToDTO(persisted), canonicalNext) {
		return domain.IntentDraft{}, invalidIntentContract(fmt.Errorf("repository changed intent mutation facts"))
	}
	return persisted, nil
}

func validateIntentMutationReceipt(receipt IntentDraftMutationReceiptDTO, monitorID, draftID int64, commandFingerprint string, expectedNext *domain.IntentDraft) (domain.IntentDraft, error) {
	if receipt.CommandFingerprint != commandFingerprint {
		return domain.IntentDraft{}, ErrIntentIdempotencyConflict
	}
	persisted, err := intentDraftFromDTO(receipt.Draft)
	if err != nil {
		return domain.IntentDraft{}, err
	}
	if persisted.MonitorID() != monitorID || persisted.DraftID() != draftID {
		return domain.IntentDraft{}, invalidIntentContract(fmt.Errorf("idempotent receipt belongs to another monitor"))
	}
	if receipt.Created && expectedNext != nil && !reflect.DeepEqual(intentDraftToDTO(persisted), intentDraftToDTO(*expectedNext)) {
		return domain.IntentDraft{}, invalidIntentContract(fmt.Errorf("repository changed candidate review facts"))
	}
	return persisted, nil
}

func (service *IntentService) reserveRun(ctx context.Context, idempotencyKey string, kind domain.IntentRunKind, draft domain.IntentDraft, inputHash, profile string, sampleLimit int) (IntentRunDTO, bool, error) {
	key, err := validateIntentIdempotencyKey(idempotencyKey)
	if err != nil {
		return IntentRunDTO{}, false, err
	}
	now, err := service.now()
	if err != nil {
		return IntentRunDTO{}, false, err
	}
	requestHash := intentRunHash(
		"intent-run-request-v1", string(kind), strconv.FormatInt(draft.MonitorID(), 10),
		strconv.FormatInt(draft.DraftID(), 10), strconv.FormatInt(draft.ResourceVersion(), 10),
		inputHash, profile, strconv.Itoa(sampleLimit),
	)
	reservation, err := service.runs.ReserveAndEnqueue(ctx, ReserveIntentRunDTO{
		IdempotencyKey: key, RequestHash: requestHash, RequestedAt: now,
		Task: IntentRunTaskDTO{
			Kind: string(kind), MonitorID: draft.MonitorID(), DraftID: draft.DraftID(), DraftResourceVersion: draft.ResourceVersion(),
			InputHash: inputHash, AnalysisProfile: profile, SampleLimit: sampleLimit,
		},
	})
	if err != nil {
		return IntentRunDTO{}, false, err
	}
	run, err := intentRunFromDTO(reservation.Run)
	if err != nil {
		return IntentRunDTO{}, false, err
	}
	identityMatches := run.Kind() == kind && run.MonitorID() == draft.MonitorID() && run.DraftID() == draft.DraftID() && run.DraftResourceVersion() == draft.ResourceVersion() && run.InputHash() == inputHash
	if !identityMatches && !reservation.Created {
		return IntentRunDTO{}, false, ErrIntentIdempotencyConflict
	}
	if !identityMatches || reservation.Created && run.Status() != domain.IntentRunQueued {
		return IntentRunDTO{}, false, invalidIntentContract(fmt.Errorf("run reservation does not match requested task"))
	}
	return intentRunToDTO(run), !reservation.Created, nil
}

func (service *IntentService) now() (time.Time, error) {
	now := service.clock.Now()
	if now.IsZero() {
		return time.Time{}, invalidIntentContract(fmt.Errorf("intent clock returned a zero time"))
	}
	return now.UTC(), nil
}

func validateStoredIntentRun(item IntentRunDTO, monitorID, draftID, draftResourceVersion, runID int64, kind domain.IntentRunKind) (domain.IntentAnalysisRun, error) {
	run, err := intentRunFromDTO(item)
	if err != nil {
		return domain.IntentAnalysisRun{}, err
	}
	if run.MonitorID() != monitorID || run.DraftID() != draftID || run.DraftResourceVersion() != draftResourceVersion || run.ID() != runID || run.Kind() != kind {
		return domain.IntentAnalysisRun{}, invalidIntentContract(fmt.Errorf("repository returned another intent run"))
	}
	return run, nil
}

func validateIntentRunQuery(monitorID, draftID, draftResourceVersion, runID int64) error {
	if monitorID <= 0 || draftID <= 0 || draftResourceVersion <= 0 || runID <= 0 {
		return ErrInvalidIntentContract
	}
	return nil
}

func validatePreviewDTO(preview *IntentPreviewDTO) error {
	if preview == nil {
		return nil
	}
	if preview.EstimatedAlertCount < 0 || len(preview.Samples) > 200 {
		return invalidIntentContract(fmt.Errorf("preview summary is invalid"))
	}
	for _, sample := range preview.Samples {
		if sample.DocumentVersionID <= 0 || sample.Title == "" ||
			(sample.Decision != "accepted" && sample.Decision != "review" && sample.Decision != "rejected") {
			return invalidIntentContract(fmt.Errorf("preview sample is invalid"))
		}
		for _, signal := range sample.RecallSignals {
			if signal.Channel == "" || signal.Rank <= 0 || math.IsNaN(signal.Score) || math.IsInf(signal.Score, 0) {
				return invalidIntentContract(fmt.Errorf("preview recall signal is invalid"))
			}
		}
	}
	return nil
}

func validateIntentProfile(value string) (string, error) {
	if !intentProfilePattern.MatchString(value) {
		return "", ErrInvalidIntentContract
	}
	return value, nil
}

func validateIntentExpansionProfile(value string) (string, error) {
	if value != IntentExpansionProfile {
		return "", ErrInvalidIntentContract
	}
	return value, nil
}

func validateIntentIdempotencyKey(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 128 || strings.ContainsAny(value, "\x00\r\n") {
		return "", ErrInvalidIntentContract
	}
	return value, nil
}

func intentRunHash(parts ...string) string {
	digest := sha256.New()
	for _, part := range parts {
		writeIntentApplicationHashPart(digest, part)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func writeIntentApplicationHashPart(target hash.Hash, value string) {
	_, _ = io.WriteString(target, strconv.Itoa(len([]byte(value))))
	_, _ = io.WriteString(target, ":")
	_, _ = io.WriteString(target, value)
	_, _ = io.WriteString(target, "\n")
}

func translateIntentDomainError(err error) error {
	switch {
	case errors.Is(err, domain.ErrIntentVersionConflict):
		return ErrIntentVersionConflict
	case errors.Is(err, domain.ErrExpansionCandidateNotFound):
		return ErrExpansionCandidateNotFound
	case errors.Is(err, domain.ErrExpansionAlreadyReviewed):
		return ErrExpansionDecisionConflict
	case errors.Is(err, domain.ErrIntentRunTransition):
		return ErrIntentRunStateConflict
	default:
		return invalidIntentContract(err)
	}
}
