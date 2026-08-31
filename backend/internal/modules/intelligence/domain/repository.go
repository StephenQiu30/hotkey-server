package domain

import (
	"context"
	"encoding/json"
	"time"
)

// RunClaim contains the plain execution facts needed to reserve one AI run.
// Persistence adapters must re-read profile and budget policy under their own
// transaction; callers never pass database or provider-specific objects.
type RunClaim struct {
	TaskType                                                            TaskType
	WorkspaceKey, SkillID, TargetType, RuntimeVersion                   string
	TargetID, TargetVersion, ModelProfileID                             int64
	OwningJobID                                                         *int64
	PromptVersion, InputSchemaVersion, SchemaVersion, ParametersVersion string
	InputHash, EvidenceSetHash                                          string
	Now                                                                 time.Time
}

type ClaimedRun struct {
	Run    Run
	Reused bool
}

type StructuredRunCompletion struct {
	RunID      int64
	Result     json.RawMessage
	Usage      Usage
	LatencyMS  int64
	FinishedAt time.Time
}

// RunExecutionRepository is the persistence port needed by structured model
// execution. It deliberately contains no SQL, transaction, or adapter types.
type RunExecutionRepository interface {
	EligibleProfiles(context.Context, TaskType) ([]ModelProfile, error)
	Claim(context.Context, RunClaim) (ClaimedRun, error)
	Transition(context.Context, int64, RunStatus, time.Time) (Run, error)
	BeginRepair(context.Context, int64, time.Time) error
	CompleteStructured(context.Context, StructuredRunCompletion) (Run, error)
	Fail(context.Context, int64, int, time.Time) (Run, error)
}

type RunLeaseRepository interface {
	ReclaimExpired(context.Context, time.Time) (int, error)
}

type ModelProfileRepository interface {
	CreateProfile(context.Context, *ModelProfile) error
	ListProfilePage(context.Context, ModelProfileListQuery) (ModelProfilePage, error)
	GetProfile(context.Context, int64) (ModelProfile, error)
	UpdateProfile(context.Context, ModelProfile, int64) (ModelProfile, error)
	SoftDeleteProfile(context.Context, int64, int64) (ModelProfile, error)
	RestoreProfile(context.Context, int64, int64) (ModelProfile, error)
}

type EmbeddingTarget string

const (
	EmbeddingTargetContent EmbeddingTarget = "content"
	EmbeddingTargetMonitor EmbeddingTarget = "monitor"
	EmbeddingTargetEvent   EmbeddingTarget = "event"
	EmbeddingTargetTopic   EmbeddingTarget = "topic"
)

type EmbeddingWrite struct {
	Target                                        EmbeddingTarget
	TargetID, ModelProfileID, ModelProfileVersion int64
	ModelVersion, InputHash, QueryText            string
	Vector                                        []float32
}

type EmbeddingMatch struct {
	TargetID, ModelProfileVersion int64
	ModelVersion                  string
	Distance                      float64
}

type EmbeddingCompletion struct {
	RunID      int64
	Write      EmbeddingWrite
	Usage      Usage
	LatencyMS  int64
	FinishedAt time.Time
}

// ProjectedEmbeddingCompletion binds an owning module's projection to the AI
// run settlement. The callback receives only a transaction-bearing context.
type ProjectedEmbeddingCompletion struct {
	RunID, TargetID, ModelProfileID, ModelProfileVersion int64
	TargetType, ModelVersion, InputHash                  string
	Vector                                               []float32
	Usage                                                Usage
	LatencyMS                                            int64
	FinishedAt                                           time.Time
}

type ProjectedEmbeddingCommit func(context.Context) error

type EmbeddingExecutionRepository interface {
	EligibleProfiles(context.Context, TaskType) ([]ModelProfile, error)
	Claim(context.Context, RunClaim) (ClaimedRun, error)
	Transition(context.Context, int64, RunStatus, time.Time) (Run, error)
	ActiveEmbeddingForRun(context.Context, EmbeddingTarget, int64, int64) ([]float32, error)
	CompleteEmbedding(context.Context, EmbeddingCompletion) (int64, error)
	CompleteProjectedEmbedding(context.Context, ProjectedEmbeddingCompletion, ProjectedEmbeddingCommit) error
}

type EmbeddingQueryRepository interface {
	ActiveEmbedding(context.Context, EmbeddingTarget, int64, int64, int64, string) ([]float32, bool, error)
	NearestPublishedMonitorEmbeddings(context.Context, int64, int64, string, []float32, int) ([]EmbeddingMatch, error)
}
