package application

import "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"

const (
	AnalysisStatusPending = "pending_analysis"

	AnalysisReasonModelUnavailable = "ai_model_unavailable"
	AnalysisReasonBudgetExhausted  = "ai_budget_exhausted"
	AnalysisReasonRateLimited      = "ai_provider_rate_limited"
	AnalysisReasonProviderFailure  = "ai_provider_unavailable"
	AnalysisReasonProviderTimeout  = "ai_provider_timeout"
	AnalysisReasonOutputInvalid    = "ai_output_invalid"
	AnalysisReasonRunInProgress    = "ai_in_progress"
	AnalysisReasonLeaseExpired     = "ai_run_lease_expired"
)

// AnalysisPendingReason maps only registered, operational AI outcomes to a
// bounded status reason. Invalid caller contracts and storage errors remain
// hard failures rather than being hidden as model degradation.
func AnalysisPendingReason(err error) (string, bool) {
	code, known := domain.CodeOf(err)
	if !known {
		return "", false
	}
	switch code {
	case domain.CodeAIModelUnavailable:
		return AnalysisReasonModelUnavailable, true
	case domain.CodeAIBudgetExhausted:
		return AnalysisReasonBudgetExhausted, true
	case domain.CodeAIProviderRateLimited:
		return AnalysisReasonRateLimited, true
	case domain.CodeAIProviderTransient:
		return AnalysisReasonProviderFailure, true
	case domain.CodeAIProviderTimeout:
		return AnalysisReasonProviderTimeout, true
	case domain.CodeAIOutputInvalid:
		return AnalysisReasonOutputInvalid, true
	case domain.CodeAIRunInProgress:
		return AnalysisReasonRunInProgress, true
	case domain.CodeAIRunLeaseExpired:
		return AnalysisReasonLeaseExpired, true
	default:
		return "", false
	}
}

func IsAnalysisPending(status string) bool {
	// "degraded" remains readable while existing callers migrate to the
	// explicit pending_analysis state.
	return status == AnalysisStatusPending || status == "degraded"
}

func pendingStructuredExecution(run domain.Run, err error) (StructuredExecutionResult, bool) {
	reason, ok := AnalysisPendingReason(err)
	if !ok {
		return StructuredExecutionResult{}, false
	}
	if code, known := domain.CodeOf(err); known && run.ID > 0 {
		run.Status = domain.RunStatusFailed
		run.ErrorCode = &code
		run.ReservedCost = "0.0000"
		run.LeaseExpiresAt = nil
		run.StructuredResult = nil
	}
	return StructuredExecutionResult{Status: AnalysisStatusPending, ReasonCode: reason, Run: run}, true
}
