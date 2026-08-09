package postgres

import (
	"database/sql"
	"time"
)

type intentScanner interface {
	Scan(...any) error
}

type intentDraftRecord struct {
	ID, ResourceVersion, MonitorID, ConfigVersionID int64
	CreatedAt, UpdatedAt                            time.Time
}

type intentDraftRevisionRecord struct {
	ID, DraftID, MonitorID, ConfigVersionID, ResourceVersion int64
	Objective                                                string
	CreatedAt                                                time.Time
}

type intentExpansionCandidateRecord struct {
	ID, DraftID, IntroducedResourceVersion int64
	CandidateID, Value, Source, Reason     string
	ModelVersion, PromptVersion, InputHash string
	Similarity                             float64
	Risk                                   string
}

type intentDraftCandidateRecord struct {
	Candidate intentExpansionCandidateRecord
	Status    string
	Reviewer  sql.NullInt64
	Reviewed  sql.NullTime
	Note      string
}

type intentMutationReceiptRecord struct {
	ID, MonitorID, DraftID, ExpectedVersion, ResultVersion int64
	Kind, IdempotencyKey, Fingerprint                      string
	CreatedAt                                              time.Time
}

type intentAnalysisRunRecord struct {
	ID, MonitorID, DraftID, DraftResourceVersion, RiverJobID int64
	Kind, InputHash, AnalysisProfile, RequestHash            string
	SampleLimit                                              int
	IdempotencyKey, Status                                   string
	QueuedAt                                                 time.Time
	StartedAt, CompletedAt, InvalidatedAt                    sql.NullTime
	FailureReason, ResultFingerprint                         sql.NullString
}

const intentAnalysisRunColumns = `
id,monitor_id,draft_id,draft_resource_version,river_job_id,
kind,input_hash,profile_version,sample_limit,request_hash,idempotency_key,status,
queued_at,started_at,completed_at,invalidated_at,failure_reason,result_fingerprint`

func scanIntentAnalysisRun(scanner intentScanner) (intentAnalysisRunRecord, error) {
	var record intentAnalysisRunRecord
	err := scanner.Scan(
		&record.ID, &record.MonitorID, &record.DraftID, &record.DraftResourceVersion, &record.RiverJobID,
		&record.Kind, &record.InputHash, &record.AnalysisProfile, &record.SampleLimit, &record.RequestHash,
		&record.IdempotencyKey, &record.Status, &record.QueuedAt, &record.StartedAt, &record.CompletedAt,
		&record.InvalidatedAt, &record.FailureReason, &record.ResultFingerprint,
	)
	return record, err
}
