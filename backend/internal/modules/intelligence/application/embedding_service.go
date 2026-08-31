package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

type EmbeddingServiceDependencies struct {
	Runs       domain.EmbeddingExecutionRepository
	Providers  *ProviderRegistry
	RunService *RunService
}

// EmbeddingService delegates terminal persistence to CompleteEmbedding, which
// is the only production path that can activate a vector.
type EmbeddingService struct {
	runs       domain.EmbeddingExecutionRepository
	providers  *ProviderRegistry
	runService *RunService
}

func NewEmbeddingService(dependencies EmbeddingServiceDependencies) (*EmbeddingService, error) {
	if dependencies.Runs == nil || dependencies.Providers == nil || dependencies.RunService == nil {
		return nil, fmt.Errorf("AI embedding service dependencies are required")
	}
	return &EmbeddingService{runs: dependencies.Runs, providers: dependencies.Providers, runService: dependencies.RunService}, nil
}

type EmbeddingExecutionInput struct {
	Target                                                              domain.EmbeddingTarget
	TargetID, TargetVersion                                             int64
	PromptVersion, InputSchemaVersion, SchemaVersion, ParametersVersion string
	InputHash, EvidenceSetHash                                          string
	Input, QueryText                                                    string
}

type EmbeddingResult struct {
	Status, ReasonCode string
	Run                domain.Run
	Vector             []float32
	Reused             bool
}

func (service *EmbeddingService) Execute(ctx context.Context, input EmbeddingExecutionInput) (EmbeddingResult, error) {
	if service == nil || service.runs == nil || service.providers == nil || service.runService == nil ||
		!validEmbeddingTarget(input.Target) || input.TargetID <= 0 || input.TargetVersion <= 0 || strings.TrimSpace(input.Input) == "" {
		return EmbeddingResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	profiles, err := service.runs.EligibleProfiles(ctx, domain.TaskTypeEmbedding)
	if err != nil {
		return EmbeddingResult{}, err
	}
	var budgetError error
	for _, profile := range profiles {
		provider, available := service.providers.Resolve(profile.Provider)
		if !available {
			continue
		}
		claim, err := service.runs.Claim(ctx, domain.RunClaim{
			TaskType: domain.TaskTypeEmbedding, WorkspaceKey: DefaultWorkspaceKey, SkillID: EmbeddingSkillID,
			TargetType: string(input.Target), TargetID: input.TargetID, TargetVersion: input.TargetVersion,
			RuntimeVersion: StructuredRuntimeVersion, ModelProfileID: profile.ID,
			PromptVersion: input.PromptVersion, InputSchemaVersion: input.InputSchemaVersion, SchemaVersion: input.SchemaVersion,
			ParametersVersion: input.ParametersVersion, InputHash: input.InputHash, EvidenceSetHash: input.EvidenceSetHash, Now: service.runService.now(),
			OwningJobID: owningJobID(ctx),
		})
		if err != nil {
			if code, known := domain.CodeOf(err); known && (code == domain.CodeAIModelUnavailable || code == domain.CodeAIBudgetExhausted) {
				if code == domain.CodeAIBudgetExhausted {
					budgetError = err
				}
				continue
			}
			return EmbeddingResult{}, err
		}
		if claim.Reused {
			vector, err := service.runs.ActiveEmbeddingForRun(ctx, input.Target, input.TargetID, claim.Run.ID)
			if err != nil {
				return EmbeddingResult{}, err
			}
			if vector == nil {
				return EmbeddingResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
			}
			return EmbeddingResult{Status: "succeeded", Run: claim.Run, Vector: vector, Reused: true}, nil
		}
		started := service.runService.now()
		response, err := service.embed(ctx, claim.Run.ID, profile, provider, input.Input)
		if err != nil {
			return EmbeddingResult{}, err
		}
		if len(response.Vectors) != 1 || response.ModelVersion != profile.ModelVersion {
			_ = service.runService.fail(ctx, claim.Run.ID, domain.CodeAIModelProfileInvalid)
			return EmbeddingResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
		}
		if err := domain.ValidateEmbedding(response.Vectors[0]); err != nil {
			_ = service.runService.fail(ctx, claim.Run.ID, domain.CodeAIEmbeddingInvalid)
			return EmbeddingResult{}, err
		}
		if _, err := response.Usage.TotalTokens(); err != nil {
			_ = service.runService.fail(ctx, claim.Run.ID, domain.CodeAIModelProfileInvalid)
			return EmbeddingResult{}, err
		}
		completedAt := service.runService.now()
		if _, err := service.runs.CompleteEmbedding(ctx, domain.EmbeddingCompletion{
			RunID: claim.Run.ID,
			Write: domain.EmbeddingWrite{
				Target: input.Target, TargetID: input.TargetID, ModelProfileID: profile.ID, ModelProfileVersion: profile.Version,
				ModelVersion: profile.ModelVersion, InputHash: input.InputHash, QueryText: input.QueryText, Vector: response.Vectors[0],
			},
			Usage: response.Usage, LatencyMS: elapsedMilliseconds(started, completedAt), FinishedAt: completedAt,
		}); err != nil {
			return EmbeddingResult{}, err
		}
		return EmbeddingResult{Status: "succeeded", Run: domain.Run{ID: claim.Run.ID, Status: domain.RunStatusSucceeded}, Vector: append([]float32(nil), response.Vectors[0]...)}, nil
	}
	if budgetError != nil {
		return EmbeddingResult{}, budgetError
	}
	return EmbeddingResult{Status: "degraded", ReasonCode: degradedReasonModelUnavailable}, nil
}

func (service *EmbeddingService) embed(ctx context.Context, runID int64, profile domain.ModelProfile, provider domain.Provider, input string) (domain.EmbeddingResponse, error) {
	for {
		if _, err := service.runs.Transition(ctx, runID, domain.RunStatusRunning, service.runService.now()); err != nil {
			return domain.EmbeddingResponse{}, err
		}
		providerCtx, cancel := providerCallContext(ctx, profile.TimeoutSeconds)
		response, err := provider.Embed(providerCtx, domain.EmbeddingRequest{
			ModelName: profile.ModelName, ModelVersion: profile.ModelVersion, Dimensions: domain.EmbeddingDimensions, Inputs: []string{input},
		})
		cancel()
		if err == nil {
			if _, err := service.runs.Transition(ctx, runID, domain.RunStatusValidating, service.runService.now()); err != nil {
				return domain.EmbeddingResponse{}, err
			}
			return response, nil
		}
		if err := service.runService.retryOrFail(ctx, runID, profile, err); err != nil {
			return domain.EmbeddingResponse{}, err
		}
	}
}

func validEmbeddingTarget(target domain.EmbeddingTarget) bool {
	return target == domain.EmbeddingTargetContent || target == domain.EmbeddingTargetMonitor ||
		target == domain.EmbeddingTargetEvent || target == domain.EmbeddingTargetTopic
}
