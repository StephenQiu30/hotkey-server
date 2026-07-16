package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	sharedclock "github.com/StephenQiu30/hotkey-server/internal/shared/clock"
)

const degradedReasonModelUnavailable = "ai_model_unavailable"

type SleepFunc func(context.Context, time.Duration) error

type RunServiceDependencies struct {
	Runs      *intelligencepostgres.Repository
	Providers *ProviderRegistry
	Schemas   *SchemaRegistry
	Clock     sharedclock.Clock
	Sleep     SleepFunc
}

// RunService owns network orchestration, but never a database transaction
// while an adapter is called. The repository owns every terminal write.
type RunService struct {
	runs      *intelligencepostgres.Repository
	providers *ProviderRegistry
	schemas   *SchemaRegistry
	clock     sharedclock.Clock
	sleep     SleepFunc
}

func NewRunService(dependencies RunServiceDependencies) (*RunService, error) {
	if dependencies.Runs == nil || dependencies.Providers == nil || dependencies.Schemas == nil {
		return nil, fmt.Errorf("AI run service dependencies are required")
	}
	if dependencies.Clock == nil {
		dependencies.Clock = sharedclock.System{}
	}
	if dependencies.Sleep == nil {
		dependencies.Sleep = waitForRetry
	}
	return &RunService{runs: dependencies.Runs, providers: dependencies.Providers, schemas: dependencies.Schemas, clock: dependencies.Clock, sleep: dependencies.Sleep}, nil
}

type StructuredExecutionInput struct {
	TaskType                                                            domain.TaskType
	TargetType                                                          string
	TargetID                                                            int64
	PromptVersion, InputSchemaVersion, SchemaVersion, ParametersVersion string
	InputHash, EvidenceSetHash                                          string
	Input                                                               json.RawMessage
}

type StructuredExecutionResult struct {
	Status, ReasonCode string
	Run                domain.Run
	Result             json.RawMessage
	Reused             bool
}

func (service *RunService) ExecuteStructured(ctx context.Context, input StructuredExecutionInput) (StructuredExecutionResult, error) {
	if service == nil || service.runs == nil || service.providers == nil || service.schemas == nil ||
		input.TaskType != domain.TaskTypeTermExpansion || strings.TrimSpace(input.TargetType) == "" || input.TargetID <= 0 {
		return StructuredExecutionResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	if err := service.schemas.ValidateInput(input.TaskType, input.InputSchemaVersion, input.Input); err != nil {
		return StructuredExecutionResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	contract, err := service.schemas.Structured(input.TaskType, input.SchemaVersion)
	if err != nil {
		return StructuredExecutionResult{}, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	profiles, err := service.runs.EligibleProfiles(ctx, input.TaskType)
	if err != nil {
		return StructuredExecutionResult{}, err
	}
	var budgetError error
	for _, profile := range profiles {
		provider, available := service.providers.Resolve(profile.Provider)
		if !available {
			continue
		}
		claim, err := service.runs.Claim(ctx, intelligencepostgres.ClaimInput{
			TaskType: input.TaskType, TargetType: input.TargetType, TargetID: input.TargetID, ModelProfileID: profile.ID,
			PromptVersion: input.PromptVersion, InputSchemaVersion: input.InputSchemaVersion, SchemaVersion: input.SchemaVersion,
			ParametersVersion: input.ParametersVersion, InputHash: input.InputHash, EvidenceSetHash: input.EvidenceSetHash, Now: service.now(),
		})
		if err != nil {
			if code, known := domain.CodeOf(err); known && (code == domain.CodeAIModelUnavailable || code == domain.CodeAIBudgetExhausted) {
				if code == domain.CodeAIBudgetExhausted {
					budgetError = err
				}
				continue
			}
			return StructuredExecutionResult{}, err
		}
		if claim.Reused {
			if !json.Valid(claim.Run.StructuredResult) || len(claim.Run.StructuredResult) == 0 || service.schemas.ValidateOutput(input.TaskType, input.SchemaVersion, claim.Run.StructuredResult) != nil {
				return StructuredExecutionResult{}, domain.NewError(domain.CodeAIOutputInvalid)
			}
			return StructuredExecutionResult{Status: "succeeded", Run: claim.Run, Result: cloneRawJSON(claim.Run.StructuredResult), Reused: true}, nil
		}
		started := service.now()
		response, err := service.generateStructured(ctx, claim.Run.ID, profile, provider, domain.StructuredRequest{
			ModelName: profile.ModelName, ModelVersion: profile.ModelVersion, TaskType: input.TaskType,
			SchemaName: contract.SchemaName, SchemaVersion: contract.SchemaVersion, Instruction: contract.Instruction,
			Schema: contract.OutputSchema, Input: cloneRawJSON(input.Input),
		}, false)
		if err != nil {
			return StructuredExecutionResult{}, err
		}
		if err := service.schemas.ValidateOutput(input.TaskType, input.SchemaVersion, response.JSON); err != nil {
			firstUsage := response.Usage
			repair, repairErr := service.schemas.RepairForInvalidOutput(input.TaskType, input.SchemaVersion, response.JSON, false)
			if repairErr != nil || repair == nil {
				_ = service.fail(ctx, claim.Run.ID, domain.CodeAIOutputInvalid)
				return StructuredExecutionResult{}, domain.NewError(domain.CodeAIOutputInvalid)
			}
			if err := service.runs.BeginRepair(ctx, claim.Run.ID, service.now()); err != nil {
				_ = service.fail(ctx, claim.Run.ID, domain.CodeAIOutputInvalid)
				return StructuredExecutionResult{}, err
			}
			response, err = service.generateStructured(ctx, claim.Run.ID, profile, provider, domain.StructuredRequest{
				ModelName: profile.ModelName, ModelVersion: profile.ModelVersion, TaskType: input.TaskType,
				SchemaName: contract.SchemaName, SchemaVersion: contract.SchemaVersion, Instruction: contract.Instruction,
				Schema: contract.OutputSchema, Input: cloneRawJSON(input.Input), Repair: repair,
			}, true)
			if err != nil {
				return StructuredExecutionResult{}, err
			}
			if err := service.schemas.ValidateOutput(input.TaskType, input.SchemaVersion, response.JSON); err != nil {
				_ = service.fail(ctx, claim.Run.ID, domain.CodeAIOutputInvalid)
				return StructuredExecutionResult{}, domain.NewError(domain.CodeAIOutputInvalid)
			}
			usage, err := firstUsage.Add(response.Usage)
			if err != nil {
				_ = service.fail(ctx, claim.Run.ID, domain.CodeAIModelProfileInvalid)
				return StructuredExecutionResult{}, err
			}
			response.Usage = usage
		}
		completed, err := service.runs.CompleteStructured(ctx, intelligencepostgres.StructuredCompletion{
			RunID: claim.Run.ID, Result: response.JSON, Usage: response.Usage, LatencyMS: elapsedMilliseconds(started, service.now()), FinishedAt: service.now(),
		})
		if err != nil {
			return StructuredExecutionResult{}, err
		}
		return StructuredExecutionResult{Status: "succeeded", Run: completed, Result: cloneRawJSON(completed.StructuredResult)}, nil
	}
	if budgetError != nil {
		return StructuredExecutionResult{}, budgetError
	}
	return StructuredExecutionResult{Status: "degraded", ReasonCode: degradedReasonModelUnavailable}, nil
}

func (service *RunService) generateStructured(ctx context.Context, runID int64, profile domain.ModelProfile, provider domain.Provider, request domain.StructuredRequest, alreadyValidating bool) (domain.StructuredResponse, error) {
	startRunning := !alreadyValidating
	for {
		if startRunning {
			if _, err := service.runs.Transition(ctx, runID, domain.RunStatusRunning, service.now()); err != nil {
				return domain.StructuredResponse{}, err
			}
		}
		response, err := provider.GenerateStructured(ctx, request)
		if err == nil {
			if response.ModelVersion != profile.ModelVersion {
				_ = service.fail(ctx, runID, domain.CodeAIModelProfileInvalid)
				return domain.StructuredResponse{}, domain.NewError(domain.CodeAIModelProfileInvalid)
			}
			if _, err := response.Usage.TotalTokens(); err != nil {
				_ = service.fail(ctx, runID, domain.CodeAIModelProfileInvalid)
				return domain.StructuredResponse{}, err
			}
			if startRunning {
				if _, err := service.runs.Transition(ctx, runID, domain.RunStatusValidating, service.now()); err != nil {
					return domain.StructuredResponse{}, err
				}
			}
			return response, nil
		}
		if err := service.retryOrFail(ctx, runID, profile, err); err != nil {
			return domain.StructuredResponse{}, err
		}
		startRunning = true
	}
}

func (service *RunService) retryOrFail(ctx context.Context, runID int64, profile domain.ModelProfile, providerErr error) error {
	code, known := domain.CodeOf(providerErr)
	if !known {
		code = domain.CodeAIProviderTransient
	}
	if !domain.Retryable(code) {
		_ = service.fail(ctx, runID, code)
		return domain.NewError(code)
	}
	retrying, err := service.runs.Transition(ctx, runID, domain.RunStatusRetryWait, service.now())
	if err != nil {
		_ = service.fail(ctx, runID, code)
		return domain.NewError(code)
	}
	if retrying.LeaseExpiresAt == nil {
		_ = service.fail(ctx, runID, code)
		return domain.NewError(code)
	}
	retryAt := retrying.LeaseExpiresAt.Add(-time.Duration(profile.TimeoutSeconds+30) * time.Second)
	delay := retryAt.Sub(service.now())
	if delay < 0 {
		delay = 0
	}
	if err := service.sleep(ctx, delay); err != nil {
		_ = service.fail(context.Background(), runID, code)
		return domain.NewError(code)
	}
	return nil
}

func (service *RunService) fail(ctx context.Context, runID int64, code int) error {
	_, err := service.runs.Fail(ctx, runID, code, service.now())
	return err
}

func (service *RunService) now() time.Time { return service.clock.Now().UTC() }

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func elapsedMilliseconds(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func cloneRawJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
