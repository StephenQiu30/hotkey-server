package http

import (
	"fmt"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func intentDraftRequestCommand(request ReplaceIntentDraftRequestDTO) (string, []monitorapplication.IntentClauseDTO, []monitorapplication.IntentEntityDTO, []monitorapplication.IntentExampleDTO) {
	clauses := make([]monitorapplication.IntentClauseDTO, 0, len(request.Clauses))
	for _, item := range request.Clauses {
		clauses = append(clauses, monitorapplication.IntentClauseDTO{Operator: item.Operator, Field: item.Field, Value: item.Value})
	}
	entities := make([]monitorapplication.IntentEntityDTO, 0, len(request.Entities))
	for _, item := range request.Entities {
		entities = append(entities, monitorapplication.IntentEntityDTO{
			CanonicalID: item.CanonicalID, DisplayName: item.DisplayName,
			Aliases: append([]string(nil), item.Aliases...), AmbiguityNote: item.AmbiguityNote,
		})
	}
	examples := make([]monitorapplication.IntentExampleDTO, 0, len(request.Examples))
	for _, item := range request.Examples {
		examples = append(examples, monitorapplication.IntentExampleDTO{Label: item.Label, Text: item.Text})
	}
	return request.Objective, clauses, entities, examples
}

func intentDraftResponseDTO(draft monitorapplication.IntentDraftDTO) IntentDraftResponseDTO {
	response := IntentDraftResponseDTO{
		MonitorID: draft.MonitorID, DraftID: draft.DraftID, ResourceVersion: draft.ResourceVersion,
		Objective: draft.Objective, Clauses: make([]IntentClauseResponseDTO, 0, len(draft.Clauses)),
		Entities:   make([]IntentEntityResponseDTO, 0, len(draft.Entities)),
		Examples:   make([]IntentExampleResponseDTO, 0, len(draft.Examples)),
		Candidates: make([]IntentExpansionCandidateResponseDTO, 0, len(draft.Candidates)),
	}
	for _, item := range draft.Clauses {
		response.Clauses = append(response.Clauses, IntentClauseResponseDTO{Operator: item.Operator, Field: item.Field, Value: item.Value})
	}
	for _, item := range draft.Entities {
		response.Entities = append(response.Entities, IntentEntityResponseDTO{
			CanonicalID: item.CanonicalID, DisplayName: item.DisplayName,
			Aliases: append([]string(nil), item.Aliases...), AmbiguityNote: item.AmbiguityNote,
		})
	}
	for _, item := range draft.Examples {
		response.Examples = append(response.Examples, IntentExampleResponseDTO{Label: item.Label, Text: item.Text})
	}
	for _, item := range draft.Candidates {
		response.Candidates = append(response.Candidates, intentExpansionCandidateResponseDTO(item))
	}
	return response
}

func intentExpansionCandidateResponseDTO(item monitorapplication.ExpansionCandidateDTO) IntentExpansionCandidateResponseDTO {
	return IntentExpansionCandidateResponseDTO{
		ID: item.ID, Value: item.Value, Source: item.Source, Reason: item.Reason,
		ModelVersion: item.ModelVersion, PromptVersion: item.PromptVersion, InputHash: item.InputHash,
		Similarity: item.Similarity, Risk: item.Risk, ApprovalStatus: item.ApprovalStatus,
		ReviewerUserID: item.ReviewerUserID, ReviewedAt: item.ReviewedAt, ReviewNote: item.ReviewNote,
	}
}

func intentRunStatusURL(run monitorapplication.IntentRunDTO) string {
	return fmt.Sprintf("/api/v1/monitors/%d/draft/%s-runs/%d", run.MonitorID, run.Kind, run.ID)
}

func intentRunAcceptedResponseDTO(run monitorapplication.IntentRunDTO, reused bool) IntentRunAcceptedResponseDTO {
	return IntentRunAcceptedResponseDTO{
		RunID: run.ID, Kind: run.Kind, MonitorID: run.MonitorID, DraftID: run.DraftID,
		ResourceVersion: run.DraftResourceVersion, InputHash: run.InputHash, Status: run.Status,
		StatusURL: intentRunStatusURL(run), Reused: reused,
	}
}

func intentAnalysisRunResponseDTO(run monitorapplication.IntentRunDTO) IntentAnalysisRunResponseDTO {
	return IntentAnalysisRunResponseDTO{
		RunID: run.ID, Kind: run.Kind, MonitorID: run.MonitorID, DraftID: run.DraftID,
		ResourceVersion: run.DraftResourceVersion, InputHash: run.InputHash, Status: run.Status,
		StatusURL: intentRunStatusURL(run), QueuedAt: run.QueuedAt, StartedAt: run.StartedAt,
		CompletedAt: run.CompletedAt, InvalidatedAt: run.InvalidatedAt, FailureCode: safeIntentFailureCode(run.FailureReason),
	}
}

func intentExpansionRunStatusResponseDTO(expansion monitorapplication.ExpansionRunDTO) IntentExpansionRunStatusResponseDTO {
	response := IntentExpansionRunStatusResponseDTO{
		IntentAnalysisRunResponseDTO: intentAnalysisRunResponseDTO(expansion.Run),
		Candidates:                   make([]IntentExpansionCandidateResponseDTO, 0, len(expansion.Candidates)),
	}
	for _, item := range expansion.Candidates {
		response.Candidates = append(response.Candidates, intentExpansionCandidateResponseDTO(item))
	}
	return response
}

func intentPreviewRunStatusResponseDTO(preview monitorapplication.PreviewRunDTO) IntentPreviewRunStatusResponseDTO {
	response := IntentPreviewRunStatusResponseDTO{IntentAnalysisRunResponseDTO: intentAnalysisRunResponseDTO(preview.Run)}
	if preview.Preview == nil {
		return response
	}
	projection := &IntentPreviewResponseDTO{
		Samples:             make([]IntentPreviewSampleResponseDTO, 0, len(preview.Preview.Samples)),
		EstimatedAlertCount: preview.Preview.EstimatedAlertCount,
		Warnings:            append([]string(nil), preview.Preview.Warnings...),
	}
	for _, item := range preview.Preview.Samples {
		sample := IntentPreviewSampleResponseDTO{
			DocumentVersionID: item.DocumentVersionID, Title: item.Title, Decision: item.Decision,
			RecallSignals: make([]IntentPreviewRecallSignalResponseDTO, 0, len(item.RecallSignals)),
			Reasons:       append([]string(nil), item.Reasons...), ExclusionReasons: append([]string(nil), item.ExclusionReasons...),
		}
		for _, signal := range item.RecallSignals {
			sample.RecallSignals = append(sample.RecallSignals, IntentPreviewRecallSignalResponseDTO{Channel: signal.Channel, Rank: signal.Rank, RawScore: signal.Score})
		}
		projection.Samples = append(projection.Samples, sample)
	}
	response.Preview = projection
	return response
}

func safeIntentFailureCode(reason string) string {
	switch reason {
	case "":
		return ""
	case "expansion_processor_failed", "preview_processor_failed":
		return reason
	default:
		return "analysis_failed"
	}
}
