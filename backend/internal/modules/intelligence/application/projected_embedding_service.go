package application

import (
	"context"
	"strings"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

const (
	ProjectedEmbeddingReasonModelUnavailable       = "embedding_model_unavailable"
	ProjectedEmbeddingTargetDocumentVersion        = "document_version"
	ProjectedEmbeddingTargetMonitorCompiledProfile = "monitor_compiled_profile"
)

type ProjectedEmbeddingExecutionInput struct {
	TargetType, PromptVersion, InputSchemaVersion, SchemaVersion, ParametersVersion string
	TargetID, TargetVersion                                                         int64
	InputHash, EvidenceSetHash, Input                                               string
}

type GeneratedEmbeddingDTO struct {
	TargetType                                             string
	TargetID, ModelProfileID, ModelProfileVersion, AIRunID int64
	ModelVersion, InputHash                                string
	Vector                                                 []float32
	CreatedAt                                              time.Time
}

type GeneratedEmbeddingVerificationQuery struct {
	TargetType                                             string
	TargetID, ModelProfileID, ModelProfileVersion, AIRunID int64
	ModelVersion, InputHash                                string
}

type ProjectedEmbeddingSink interface {
	CommitGeneratedEmbedding(context.Context, GeneratedEmbeddingDTO) error
	VerifyGeneratedEmbedding(context.Context, GeneratedEmbeddingVerificationQuery) error
}

type ProjectedEmbeddingExecutionResult struct {
	Status, ReasonCode                                     string
	TargetID, ModelProfileID, ModelProfileVersion, AIRunID int64
	ModelVersion, InputHash                                string
	Reused                                                 bool
}

// ExecuteProjectedEmbedding produces a vector and delegates the business
// projection to its owning module inside the same database transaction that
// settles the AI run. No vector is returned after commit or written to a job.
func (service *EmbeddingService) ExecuteProjectedEmbedding(ctx context.Context, input ProjectedEmbeddingExecutionInput, sink ProjectedEmbeddingSink) (ProjectedEmbeddingExecutionResult, error) {
	if service == nil || service.runs == nil || service.providers == nil || service.runService == nil || sink == nil ||
		(input.TargetType != ProjectedEmbeddingTargetDocumentVersion && input.TargetType != ProjectedEmbeddingTargetMonitorCompiledProfile) || input.TargetID <= 0 || input.TargetVersion <= 0 ||
		strings.TrimSpace(input.Input) == "" || !validProjectedEmbeddingHash(input.InputHash) {
		return ProjectedEmbeddingExecutionResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	profiles, err := service.runs.EligibleProfiles(ctx, intelligencedomain.TaskTypeEmbedding)
	if err != nil {
		return ProjectedEmbeddingExecutionResult{}, err
	}
	var budgetError error
	for _, profile := range profiles {
		provider, available := service.providers.Resolve(profile.Provider)
		if !available {
			continue
		}
		claim, err := service.runs.Claim(ctx, intelligencedomain.RunClaim{
			TaskType: intelligencedomain.TaskTypeEmbedding, WorkspaceKey: DefaultWorkspaceKey, SkillID: EmbeddingSkillID,
			TargetType: input.TargetType, TargetID: input.TargetID, TargetVersion: input.TargetVersion, RuntimeVersion: StructuredRuntimeVersion,
			ModelProfileID: profile.ID, PromptVersion: input.PromptVersion, InputSchemaVersion: input.InputSchemaVersion,
			SchemaVersion: input.SchemaVersion, ParametersVersion: input.ParametersVersion,
			InputHash: input.InputHash, EvidenceSetHash: input.EvidenceSetHash, Now: service.runService.now(),
			OwningJobID: owningJobID(ctx),
		})
		if err != nil {
			if code, known := intelligencedomain.CodeOf(err); known && (code == intelligencedomain.CodeAIModelUnavailable || code == intelligencedomain.CodeAIBudgetExhausted) {
				if code == intelligencedomain.CodeAIBudgetExhausted {
					budgetError = err
				}
				continue
			}
			return ProjectedEmbeddingExecutionResult{}, err
		}
		base := ProjectedEmbeddingExecutionResult{
			Status: "succeeded", TargetID: input.TargetID, ModelProfileID: profile.ID,
			ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
			AIRunID: claim.Run.ID, InputHash: input.InputHash, Reused: claim.Reused,
		}
		if claim.Reused {
			if err := sink.VerifyGeneratedEmbedding(ctx, GeneratedEmbeddingVerificationQuery{
				TargetType: input.TargetType, TargetID: input.TargetID, ModelProfileID: profile.ID,
				ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
				AIRunID: claim.Run.ID, InputHash: input.InputHash,
			}); err != nil {
				return ProjectedEmbeddingExecutionResult{}, err
			}
			return base, nil
		}
		started := service.runService.now()
		response, err := service.embed(ctx, claim.Run.ID, profile, provider, input.Input)
		if err != nil {
			return ProjectedEmbeddingExecutionResult{}, err
		}
		if len(response.Vectors) != 1 || response.ModelVersion != profile.ModelVersion {
			_ = service.runService.fail(ctx, claim.Run.ID, intelligencedomain.CodeAIModelProfileInvalid)
			return ProjectedEmbeddingExecutionResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		vector := append([]float32(nil), response.Vectors[0]...)
		if err := intelligencedomain.ValidateEmbedding(vector); err != nil {
			_ = service.runService.fail(ctx, claim.Run.ID, intelligencedomain.CodeAIEmbeddingInvalid)
			return ProjectedEmbeddingExecutionResult{}, err
		}
		completedAt := service.runService.now()
		generated := GeneratedEmbeddingDTO{
			TargetType: input.TargetType, TargetID: input.TargetID, ModelProfileID: profile.ID,
			ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
			AIRunID: claim.Run.ID, InputHash: input.InputHash, Vector: vector, CreatedAt: completedAt,
		}
		err = service.runs.CompleteProjectedEmbedding(ctx, intelligencedomain.ProjectedEmbeddingCompletion{
			RunID: claim.Run.ID, TargetType: input.TargetType, TargetID: input.TargetID,
			ModelProfileID: profile.ID, ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
			InputHash: input.InputHash, Vector: vector, Usage: response.Usage,
			LatencyMS: elapsedMilliseconds(started, completedAt), FinishedAt: completedAt,
		}, func(transactionCtx context.Context) error {
			return sink.CommitGeneratedEmbedding(transactionCtx, generated)
		})
		if err != nil {
			_ = service.runService.fail(ctx, claim.Run.ID, intelligencedomain.CodeAIModelProfileInvalid)
			return ProjectedEmbeddingExecutionResult{}, err
		}
		return base, nil
	}
	if budgetError != nil {
		return ProjectedEmbeddingExecutionResult{}, budgetError
	}
	return ProjectedEmbeddingExecutionResult{Status: "degraded", ReasonCode: ProjectedEmbeddingReasonModelUnavailable, TargetID: input.TargetID, InputHash: input.InputHash}, nil
}

func validProjectedEmbeddingHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
