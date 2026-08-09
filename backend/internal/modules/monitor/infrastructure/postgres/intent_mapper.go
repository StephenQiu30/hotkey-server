package postgres

import (
	"database/sql"
	"fmt"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func (record intentAnalysisRunRecord) applicationDTO() monitorapplication.IntentRunDTO {
	return monitorapplication.IntentRunDTO{
		ID: record.ID, Kind: record.Kind, MonitorID: record.MonitorID, DraftID: record.DraftID,
		DraftResourceVersion: record.DraftResourceVersion, InputHash: record.InputHash, Status: record.Status,
		QueuedAt: record.QueuedAt.UTC(), StartedAt: nullableIntentTime(record.StartedAt),
		CompletedAt: nullableIntentTime(record.CompletedAt), InvalidatedAt: nullableIntentTime(record.InvalidatedAt),
		FailureReason: record.FailureReason.String,
	}
}

func (record intentDraftCandidateRecord) applicationDTO() monitorapplication.ExpansionCandidateDTO {
	return monitorapplication.ExpansionCandidateDTO{
		ID: record.Candidate.CandidateID, Value: record.Candidate.Value, Source: record.Candidate.Source,
		Reason: record.Candidate.Reason, ModelVersion: record.Candidate.ModelVersion,
		PromptVersion: record.Candidate.PromptVersion, InputHash: record.Candidate.InputHash,
		Similarity: record.Candidate.Similarity, Risk: record.Candidate.Risk, ApprovalStatus: record.Status,
		ReviewerUserID: nullableIntentInt64(record.Reviewer), ReviewedAt: nullableIntentTime(record.Reviewed),
		ReviewNote: record.Note,
	}
}

func nullableIntentTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time.UTC()
	return &timestamp
}

func nullableIntentInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	number := value.Int64
	return &number
}

func validateIntentRecordHash(value string) error {
	if len(value) != 64 {
		return fmt.Errorf("hash must contain 64 lowercase hexadecimal characters")
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return fmt.Errorf("hash must contain 64 lowercase hexadecimal characters")
		}
	}
	return nil
}
