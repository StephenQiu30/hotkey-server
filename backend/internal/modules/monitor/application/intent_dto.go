package application

import "time"

// IntentClauseDTO is an Application-owned intent clause. Transport and
// persistence adapters map their own representations explicitly.
type IntentClauseDTO struct {
	Operator string
	Field    string
	Value    string
}

// IntentEntityDTO is an Application-owned entity reference.
type IntentEntityDTO struct {
	CanonicalID   string
	DisplayName   string
	Aliases       []string
	AmbiguityNote string
}

type IntentExampleDTO struct {
	Label string
	Text  string
}

type ExpansionCandidateDTO struct {
	ID             string
	Value          string
	Source         string
	Reason         string
	ModelVersion   string
	PromptVersion  string
	InputHash      string
	Similarity     float64
	Risk           string
	ApprovalStatus string
	ReviewerUserID *int64
	ReviewedAt     *time.Time
	ReviewNote     string
}

type IntentDraftDTO struct {
	MonitorID       int64
	DraftID         int64
	ResourceVersion int64
	Objective       string
	Clauses         []IntentClauseDTO
	Entities        []IntentEntityDTO
	Examples        []IntentExampleDTO
	Candidates      []ExpansionCandidateDTO
}

type IntentRunDTO struct {
	ID                   int64
	Kind                 string
	MonitorID            int64
	DraftID              int64
	DraftResourceVersion int64
	InputHash            string
	Status               string
	QueuedAt             time.Time
	StartedAt            *time.Time
	CompletedAt          *time.Time
	InvalidatedAt        *time.Time
	FailureReason        string
}

type ExpansionRunDTO struct {
	Run        IntentRunDTO
	Candidates []ExpansionCandidateDTO
}

type PreviewRecallSignalDTO struct {
	Channel string
	Rank    int
	Score   float64
}

// PreviewSampleDTO keeps explainability channels separate. A fused score is
// not represented as a probability and hard exclusions remain explicit.
type PreviewSampleDTO struct {
	DocumentVersionID int64
	Title             string
	Decision          string
	RecallSignals     []PreviewRecallSignalDTO
	Reasons           []string
	ExclusionReasons  []string
}

type IntentPreviewDTO struct {
	Samples             []PreviewSampleDTO
	EstimatedAlertCount int
	Warnings            []string
}

type PreviewRunDTO struct {
	Run     IntentRunDTO
	Preview *IntentPreviewDTO
}

type ReplaceIntentDraftCommand struct {
	MonitorID               int64
	DraftID                 int64
	ExpectedResourceVersion int64
	Objective               string
	Clauses                 []IntentClauseDTO
	Entities                []IntentEntityDTO
	Examples                []IntentExampleDTO
}

type ReplaceIntentDraftResult struct {
	Draft IntentDraftDTO
}

type ReadIntentDraftQuery struct {
	MonitorID int64
	DraftID   int64
}

type ReadIntentDraftResult struct {
	Draft IntentDraftDTO
}

// IntentActorDTO is the Application-owned authorization fact. Transport maps
// the authenticated platform subject into this POJO instead of leaking an
// identity Domain type into the Monitor Intent use cases.
type IntentActorDTO struct {
	UserID int64
}

type IntentControlOperation string

const (
	IntentControlReadDraft       IntentControlOperation = "read_draft"
	IntentControlReplaceDraft    IntentControlOperation = "replace_draft"
	IntentControlSubmitExpansion IntentControlOperation = "submit_expansion"
	IntentControlReadExpansion   IntentControlOperation = "read_expansion"
	IntentControlReviewCandidate IntentControlOperation = "review_candidate"
	IntentControlSubmitPreview   IntentControlOperation = "submit_preview"
	IntentControlReadPreview     IntentControlOperation = "read_preview"
)

type AuthorizeIntentControlQueryDTO struct {
	ActorUserID int64
	MonitorID   int64
	Operation   IntentControlOperation
}

type ReadCurrentIntentDraftRepositoryQuery struct {
	MonitorID int64
}

type InitializeCurrentIntentDraftMutationDTO struct {
	Initial IntentDraftDTO
}

type IntentRunStatusLookupDTO struct {
	MonitorID int64
	RunID     int64
}

type ReadCurrentIntentDraftQuery struct {
	Actor     IntentActorDTO
	MonitorID int64
}

type ReadCurrentIntentDraftResult struct {
	Draft IntentDraftDTO
}

type PutCurrentIntentDraftCommand struct {
	Actor                   IntentActorDTO
	MonitorID               int64
	ExpectedResourceVersion int64
	Objective               string
	Clauses                 []IntentClauseDTO
	Entities                []IntentEntityDTO
	Examples                []IntentExampleDTO
}

type PutCurrentIntentDraftResult struct {
	Draft   IntentDraftDTO
	Created bool
}

type ReviewCurrentExpansionCandidateCommand struct {
	Actor                   IntentActorDTO
	MonitorID               int64
	CandidateID             string
	ExpectedResourceVersion int64
	Decision                string
	Note                    string
	IdempotencyKey          string
}

type SubmitCurrentExpansionRunCommand struct {
	Actor                   IntentActorDTO
	MonitorID               int64
	ExpectedResourceVersion int64
	IdempotencyKey          string
	ExpansionProfile        string
}

type SubmitCurrentPreviewRunCommand struct {
	Actor                   IntentActorDTO
	MonitorID               int64
	ExpectedResourceVersion int64
	IdempotencyKey          string
	EvaluatorProfile        string
	SampleLimit             int
}

type ReadIntentExpansionRunQuery struct {
	Actor     IntentActorDTO
	MonitorID int64
	RunID     int64
}

type ReadIntentPreviewRunQuery struct {
	Actor     IntentActorDTO
	MonitorID int64
	RunID     int64
}

type ReviewExpansionCandidateCommand struct {
	MonitorID               int64
	DraftID                 int64
	CandidateID             string
	ExpectedResourceVersion int64
	Decision                string
	ReviewerUserID          int64
	Note                    string
	IdempotencyKey          string
}

type ReviewExpansionCandidateResult struct {
	Draft  IntentDraftDTO
	Reused bool
}

type SubmitExpansionRunCommand struct {
	MonitorID               int64
	DraftID                 int64
	ExpectedResourceVersion int64
	IdempotencyKey          string
	ExpansionProfile        string
}

type SubmitExpansionRunResult struct {
	Run    IntentRunDTO
	Reused bool
}

type ReadExpansionRunQuery struct {
	MonitorID            int64
	DraftID              int64
	DraftResourceVersion int64
	RunID                int64
}

type ReadExpansionRunResult struct {
	Expansion ExpansionRunDTO
}

type SubmitPreviewRunCommand struct {
	MonitorID               int64
	DraftID                 int64
	ExpectedResourceVersion int64
	IdempotencyKey          string
	EvaluatorProfile        string
	SampleLimit             int
}

type SubmitPreviewRunResult struct {
	Run    IntentRunDTO
	Reused bool
}

type ReadPreviewRunQuery struct {
	MonitorID            int64
	DraftID              int64
	DraftResourceVersion int64
	RunID                int64
}

type ReadPreviewRunResult struct {
	Preview PreviewRunDTO
}

// IntentRunReferenceDTO is the immutable identity carried by a durable worker
// task. DraftID and DraftResourceVersion are both required because resource
// versions can restart when a new configuration draft is created.
type IntentRunReferenceDTO struct {
	RunID                int64
	Kind                 string
	MonitorID            int64
	DraftID              int64
	DraftResourceVersion int64
	InputHash            string
}

// IntentAnalysisTaskDTO is the complete durable run fact reloaded by the
// worker from PostgreSQL. Queue payloads deliberately carry only RunID,
// DraftID, and DraftResourceVersion; profile, hash, kind, and monitor scope
// are never trusted from River arguments.
type IntentAnalysisTaskDTO struct {
	Run             IntentRunReferenceDTO
	AnalysisProfile string
	SampleLimit     int
}

type ReadIntentAnalysisTaskQuery struct {
	RunID                int64
	DraftID              int64
	DraftResourceVersion int64
}

type ReadIntentAnalysisTaskResult struct {
	Task IntentAnalysisTaskDTO
}

type ReadIntentDraftRevisionQuery struct {
	MonitorID       int64
	DraftID         int64
	ResourceVersion int64
}

// PreparedIntentExpansionDTO contains only Application POJOs. The exact
// append-only draft revision is copied so an Infrastructure processor cannot
// observe a later mutable draft pointer or a Monitor Domain entity.
type PreparedIntentExpansionDTO struct {
	Task  IntentAnalysisTaskDTO
	Draft IntentDraftDTO
}

type PrepareIntentExpansionQuery struct {
	Task IntentAnalysisTaskDTO
}

type PrepareIntentExpansionResult struct {
	Expansion PreparedIntentExpansionDTO
}

// PreparedIntentPreviewDTO freezes the exact append-only draft revision used
// by a durable preview run. Infrastructure receives no editable pointer and
// cannot substitute legacy monitor_rules.
type PreparedIntentPreviewDTO struct {
	Task  IntentAnalysisTaskDTO
	Draft IntentDraftDTO
}

type PrepareIntentPreviewQuery struct {
	Task IntentAnalysisTaskDTO
}

type PrepareIntentPreviewResult struct {
	Preview PreparedIntentPreviewDTO
}

type StartIntentRunCommand struct {
	Run IntentRunReferenceDTO
}

type StartIntentRunResult struct {
	Run    IntentRunDTO
	Reused bool
}

type FailIntentRunCommand struct {
	Run    IntentRunReferenceDTO
	Reason string
}

type FailIntentRunResult struct {
	Run    IntentRunDTO
	Reused bool
}

type CompleteExpansionRunCommand struct {
	Run        IntentRunReferenceDTO
	Candidates []ExpansionCandidateDTO
}

type CompleteExpansionRunResult struct {
	Expansion ExpansionRunDTO
	Reused    bool
}

type CompletePreviewRunCommand struct {
	Run     IntentRunReferenceDTO
	Preview IntentPreviewDTO
}

type CompletePreviewRunResult struct {
	Preview PreviewRunDTO
	Reused  bool
}

type IntentDraftMutationKind string

const (
	IntentDraftMutationReplace         IntentDraftMutationKind = "replace"
	IntentDraftMutationCandidateReview IntentDraftMutationKind = "candidate_review"
	IntentDraftMutationExpansionResult IntentDraftMutationKind = "expansion_result"
)

// IntentDraftMutationDTO is the repository-port write shape. Implementations
// must compare ExpectedResourceVersion, store Next, and invalidate all runs
// bound to older draft versions in the same database transaction.
type IntentDraftMutationDTO struct {
	Kind                    IntentDraftMutationKind
	ExpectedDraftID         int64
	ExpectedResourceVersion int64
	Next                    IntentDraftDTO
	InvalidatedAt           time.Time
	IdempotencyKey          string
	CommandFingerprint      string
}

type IntentDraftMutationLookupDTO struct {
	MonitorID      int64
	DraftID        int64
	IdempotencyKey string
}

type IntentDraftMutationReceiptDTO struct {
	Draft              IntentDraftDTO
	CommandFingerprint string
	Created            bool
}

type IntentRunTaskDTO struct {
	Kind                 string
	MonitorID            int64
	DraftID              int64
	DraftResourceVersion int64
	InputHash            string
	AnalysisProfile      string
	SampleLimit          int
}

// ReserveIntentRunDTO is the atomic run/job reservation shape. RequestHash is
// compared for Idempotency-Key reuse; it is not a user-visible relevance hash.
type ReserveIntentRunDTO struct {
	IdempotencyKey string
	RequestHash    string
	RequestedAt    time.Time
	Task           IntentRunTaskDTO
}

type IntentRunReservationDTO struct {
	Run     IntentRunDTO
	Created bool
}

// IntentRunTransitionDTO is a compare-and-swap write shape. Repositories must
// compare the complete Expected identity and lifecycle before storing Next.
type IntentRunTransitionDTO struct {
	Expected IntentRunDTO
	Next     IntentRunDTO
}

type IntentRunTransitionReceiptDTO struct {
	Run     IntentRunDTO
	Changed bool
}

// CompleteExpansionRunMutationDTO is one transaction boundary: finish the
// run, persist its pending candidates, CAS the exact draft, and invalidate
// runs bound to the superseded draft version. ResultFingerprint makes a
// duplicate worker delivery idempotent while conflicting output fails closed.
type CompleteExpansionRunMutationDTO struct {
	Transition        IntentRunTransitionDTO
	DraftMutation     IntentDraftMutationDTO
	Candidates        []ExpansionCandidateDTO
	ResultFingerprint string
}

type CompleteExpansionRunReceiptDTO struct {
	Expansion         ExpansionRunDTO
	Draft             IntentDraftDTO
	ResultFingerprint string
	Changed           bool
}

// CompletePreviewRunMutationDTO atomically transitions a running preview and
// stores its explainable result. No partially visible successful run is valid.
type CompletePreviewRunMutationDTO struct {
	Transition        IntentRunTransitionDTO
	Preview           IntentPreviewDTO
	ResultFingerprint string
}

type CompletePreviewRunReceiptDTO struct {
	Preview           PreviewRunDTO
	ResultFingerprint string
	Changed           bool
}

func cloneIntentDraftDTO(source IntentDraftDTO) IntentDraftDTO {
	result := source
	result.Clauses = append([]IntentClauseDTO(nil), source.Clauses...)
	result.Entities = make([]IntentEntityDTO, len(source.Entities))
	for index, entity := range source.Entities {
		entity.Aliases = append([]string(nil), entity.Aliases...)
		result.Entities[index] = entity
	}
	result.Examples = append([]IntentExampleDTO(nil), source.Examples...)
	result.Candidates = make([]ExpansionCandidateDTO, len(source.Candidates))
	for index, candidate := range source.Candidates {
		candidate.ReviewerUserID = copyIntentInt64(candidate.ReviewerUserID)
		candidate.ReviewedAt = copyIntentTime(candidate.ReviewedAt)
		result.Candidates[index] = candidate
	}
	return result
}

func cloneIntentRunDTO(source IntentRunDTO) IntentRunDTO {
	result := source
	result.StartedAt = copyIntentTime(source.StartedAt)
	result.CompletedAt = copyIntentTime(source.CompletedAt)
	result.InvalidatedAt = copyIntentTime(source.InvalidatedAt)
	return result
}

func cloneExpansionRunDTO(source ExpansionRunDTO) ExpansionRunDTO {
	result := source
	result.Run = cloneIntentRunDTO(source.Run)
	result.Candidates = cloneIntentDraftDTO(IntentDraftDTO{Candidates: source.Candidates}).Candidates
	return result
}

func clonePreviewRunDTO(source PreviewRunDTO) PreviewRunDTO {
	result := source
	result.Run = cloneIntentRunDTO(source.Run)
	if source.Preview == nil {
		return result
	}
	preview := *source.Preview
	preview.Warnings = append([]string(nil), source.Preview.Warnings...)
	preview.Samples = make([]PreviewSampleDTO, len(source.Preview.Samples))
	for index, sample := range source.Preview.Samples {
		sample.RecallSignals = append([]PreviewRecallSignalDTO(nil), sample.RecallSignals...)
		sample.Reasons = append([]string(nil), sample.Reasons...)
		sample.ExclusionReasons = append([]string(nil), sample.ExclusionReasons...)
		preview.Samples[index] = sample
	}
	result.Preview = &preview
	return result
}

func copyIntentInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyIntentTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
