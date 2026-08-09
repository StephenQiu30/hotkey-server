package application

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidIntentContract      = errors.New("monitor intent contract is invalid")
	ErrIntentDraftNotFound        = errors.New("monitor intent draft was not found")
	ErrIntentRunNotFound          = errors.New("monitor intent run was not found")
	ErrIntentVersionConflict      = errors.New("monitor intent resource version conflict")
	ErrIntentMutationNotFound     = errors.New("monitor intent mutation was not found")
	ErrExpansionCandidateNotFound = errors.New("monitor expansion candidate was not found")
	ErrExpansionDecisionConflict  = errors.New("monitor expansion candidate already has a decision")
	ErrIntentIdempotencyConflict  = errors.New("intent action idempotency key conflicts with prior input")
	ErrIntentRunStateConflict     = errors.New("intent analysis run state conflicts with the requested transition")
	ErrIntentRunResultConflict    = errors.New("intent analysis run already has different terminal output")
	ErrIntentAuthorizationDenied  = errors.New("monitor intent authorization denied")
)

// IntentDraftRepository is deliberately expressed only in Application DTOs.
// SaveAndInvalidateRuns must CAS both DraftID and ResourceVersion, invalidate
// older runs in the same transaction, and atomically reserve candidate-review
// IdempotencyKey receipts. Same key plus the same CommandFingerprint returns
// the original receipt; the same key with another fingerprint conflicts. A
// future PostgreSQL adapter must map these DTOs to private Records and must not
// reuse legacy monitor_rules as a lossy persistence representation.
type IntentDraftRepository interface {
	Find(context.Context, ReadIntentDraftQuery) (IntentDraftDTO, error)
	FindMutation(context.Context, IntentDraftMutationLookupDTO) (IntentDraftMutationReceiptDTO, error)
	SaveAndInvalidateRuns(context.Context, IntentDraftMutationDTO) (IntentDraftMutationReceiptDTO, error)
}

// IntentDraftRevisionRepository reads one append-only revision. Implementations
// must never substitute the draft's current resource version.
type IntentDraftRevisionRepository interface {
	FindIntentDraftRevision(context.Context, ReadIntentDraftRevisionQuery) (IntentDraftDTO, error)
}

// CurrentIntentDraftRepository resolves the intent attached to the monitor's
// current draft configuration. It must never fall back to legacy monitor_rules
// or to an intent belonging to a historical configuration version.
type CurrentIntentDraftRepository interface {
	FindCurrent(context.Context, ReadCurrentIntentDraftRepositoryQuery) (IntentDraftDTO, error)
	InitializeCurrent(context.Context, InitializeCurrentIntentDraftMutationDTO) (IntentDraftDTO, error)
}

// IntentRunStatusRepository resolves a run from the monitor-scoped status URL.
// Implementations return the exact stored DraftID and resource version; the
// Application layer revalidates that identity before exposing the result.
type IntentRunStatusRepository interface {
	FindExpansionStatus(context.Context, IntentRunStatusLookupDTO) (ExpansionRunDTO, error)
	FindPreviewStatus(context.Context, IntentRunStatusLookupDTO) (PreviewRunDTO, error)
}

// IntentAnalysisAvailability gates job reservation. A deployment without a
// real expansion generator or preview evaluator fails before creating a run;
// it must not enqueue work that no production handler can process.
type IntentAnalysisAvailability interface {
	Available(kind string) bool
}

// IntentControlAuthorizer reloads durable identity facts for every use case.
// Transport role claims are only a first-line filter and are never sufficient
// authorization for Application calls or idempotent replay.
type IntentControlAuthorizer interface {
	AuthorizeIntentControl(context.Context, AuthorizeIntentControlQueryDTO) error
}

// IntentRunRepository owns every durable async boundary. ReserveAndEnqueue
// prevents a visible queued run without its River job. SaveTransition applies
// lifecycle CAS for start/fail. CompletePreview stores the result with the
// success transition. CompleteExpansion additionally advances the exact draft
// and invalidates superseded runs in the same transaction. Duplicate terminal
// writes with the same ResultFingerprint return Changed=false; another result
// for the same run returns ErrIntentRunResultConflict.
type IntentRunRepository interface {
	ReserveAndEnqueue(context.Context, ReserveIntentRunDTO) (IntentRunReservationDTO, error)
	FindExpansion(context.Context, ReadExpansionRunQuery) (ExpansionRunDTO, error)
	FindPreview(context.Context, ReadPreviewRunQuery) (PreviewRunDTO, error)
	SaveTransition(context.Context, IntentRunTransitionDTO) (IntentRunTransitionReceiptDTO, error)
	CompleteExpansion(context.Context, CompleteExpansionRunMutationDTO) (CompleteExpansionRunReceiptDTO, error)
	CompletePreview(context.Context, CompletePreviewRunMutationDTO) (CompletePreviewRunReceiptDTO, error)
}

// IntentAnalysisTaskRepository resolves the minimal queue identity to the
// complete stored run facts. This prevents kind, input hash, model profile,
// Monitor scope, or preview parameters from being injected through River.
type IntentAnalysisTaskRepository interface {
	FindIntentAnalysisTask(context.Context, ReadIntentAnalysisTaskQuery) (IntentAnalysisTaskDTO, error)
}

type IntentClock interface {
	Now() time.Time
}

type IntentServiceDependencies struct {
	Drafts    IntentDraftRepository
	Revisions IntentDraftRevisionRepository
	Runs      IntentRunRepository
	Tasks     IntentAnalysisTaskRepository
	Clock     IntentClock
}

type IntentControlDependencies struct {
	Intents       *IntentService
	CurrentDrafts CurrentIntentDraftRepository
	RunStatuses   IntentRunStatusRepository
	Analysis      IntentAnalysisAvailability
	Authorizer    IntentControlAuthorizer
}
