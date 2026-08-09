package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrIntentRunTransition = errors.New("intent analysis run transition is invalid")

type IntentRunKind string

const (
	IntentRunExpansion IntentRunKind = "expansion"
	IntentRunPreview   IntentRunKind = "preview"
)

func (kind IntentRunKind) Valid() bool {
	return kind == IntentRunExpansion || kind == IntentRunPreview
}

type IntentRunStatus string

const (
	IntentRunQueued      IntentRunStatus = "queued"
	IntentRunRunning     IntentRunStatus = "running"
	IntentRunSucceeded   IntentRunStatus = "succeeded"
	IntentRunFailed      IntentRunStatus = "failed"
	IntentRunInvalidated IntentRunStatus = "invalidated"
)

func (status IntentRunStatus) Valid() bool {
	switch status {
	case IntentRunQueued, IntentRunRunning, IntentRunSucceeded, IntentRunFailed, IntentRunInvalidated:
		return true
	default:
		return false
	}
}

// IntentAnalysisRun models expansion and preview with one strict lifecycle.
// Result payloads remain Application DTOs; Domain owns only identity,
// version-affinity and lifecycle facts.
type IntentAnalysisRun struct {
	id                   int64
	kind                 IntentRunKind
	monitorID            int64
	draftID              int64
	draftResourceVersion int64
	inputHash            string
	status               IntentRunStatus
	queuedAt             time.Time
	startedAt            *time.Time
	completedAt          *time.Time
	invalidatedAt        *time.Time
	failureReason        string
}

func NewIntentAnalysisRun(id int64, kind IntentRunKind, monitorID, draftID, draftResourceVersion int64, inputHash string, queuedAt time.Time) (IntentAnalysisRun, error) {
	return RestoreIntentAnalysisRun(id, kind, monitorID, draftID, draftResourceVersion, inputHash, IntentRunQueued, queuedAt, nil, nil, nil, "")
}

func RestoreIntentAnalysisRun(id int64, kind IntentRunKind, monitorID, draftID, draftResourceVersion int64, inputHash string, status IntentRunStatus, queuedAt time.Time, startedAt, completedAt, invalidatedAt *time.Time, failureReason string) (IntentAnalysisRun, error) {
	if id <= 0 || !kind.Valid() || monitorID <= 0 || draftID <= 0 || draftResourceVersion <= 0 || !validIntentSHA256(inputHash) || !status.Valid() || queuedAt.IsZero() {
		return IntentAnalysisRun{}, fmt.Errorf("%w: run identity is invalid", ErrIntentRunTransition)
	}
	queuedAt = queuedAt.UTC()
	startedAt = copyRunTime(startedAt)
	completedAt = copyRunTime(completedAt)
	invalidatedAt = copyRunTime(invalidatedAt)
	failureReason = normalizeText(failureReason)
	if strings.ContainsRune(failureReason, '\x00') || len([]byte(failureReason)) > 1000 {
		return IntentAnalysisRun{}, fmt.Errorf("%w: run failure reason is invalid", ErrIntentRunTransition)
	}
	if startedAt != nil && startedAt.Before(queuedAt) || completedAt != nil && completedAt.Before(queuedAt) || startedAt != nil && completedAt != nil && completedAt.Before(*startedAt) || invalidatedAt != nil && invalidatedAt.Before(queuedAt) || startedAt != nil && invalidatedAt != nil && invalidatedAt.Before(*startedAt) || completedAt != nil && invalidatedAt != nil && invalidatedAt.Before(*completedAt) {
		return IntentAnalysisRun{}, fmt.Errorf("%w: run timestamps are out of order", ErrIntentRunTransition)
	}
	switch status {
	case IntentRunQueued:
		if startedAt != nil || completedAt != nil || invalidatedAt != nil || failureReason != "" {
			return IntentAnalysisRun{}, ErrIntentRunTransition
		}
	case IntentRunRunning:
		if startedAt == nil || completedAt != nil || invalidatedAt != nil || failureReason != "" {
			return IntentAnalysisRun{}, ErrIntentRunTransition
		}
	case IntentRunSucceeded:
		if startedAt == nil || completedAt == nil || invalidatedAt != nil || failureReason != "" {
			return IntentAnalysisRun{}, ErrIntentRunTransition
		}
	case IntentRunFailed:
		if completedAt == nil || invalidatedAt != nil || failureReason == "" {
			return IntentAnalysisRun{}, ErrIntentRunTransition
		}
	case IntentRunInvalidated:
		if invalidatedAt == nil || failureReason != "" && completedAt == nil || completedAt != nil && failureReason == "" && startedAt == nil {
			return IntentAnalysisRun{}, ErrIntentRunTransition
		}
	}
	return IntentAnalysisRun{
		id: id, kind: kind, monitorID: monitorID, draftID: draftID, draftResourceVersion: draftResourceVersion,
		inputHash: inputHash, status: status, queuedAt: queuedAt,
		startedAt: startedAt, completedAt: completedAt, invalidatedAt: invalidatedAt, failureReason: failureReason,
	}, nil
}

func (run IntentAnalysisRun) ID() int64                   { return run.id }
func (run IntentAnalysisRun) Kind() IntentRunKind         { return run.kind }
func (run IntentAnalysisRun) MonitorID() int64            { return run.monitorID }
func (run IntentAnalysisRun) DraftID() int64              { return run.draftID }
func (run IntentAnalysisRun) DraftResourceVersion() int64 { return run.draftResourceVersion }
func (run IntentAnalysisRun) InputHash() string           { return run.inputHash }
func (run IntentAnalysisRun) Status() IntentRunStatus     { return run.status }
func (run IntentAnalysisRun) QueuedAt() time.Time         { return run.queuedAt }
func (run IntentAnalysisRun) StartedAt() *time.Time       { return copyRunTime(run.startedAt) }
func (run IntentAnalysisRun) CompletedAt() *time.Time     { return copyRunTime(run.completedAt) }
func (run IntentAnalysisRun) InvalidatedAt() *time.Time {
	return copyRunTime(run.invalidatedAt)
}
func (run IntentAnalysisRun) FailureReason() string { return run.failureReason }

func (run IntentAnalysisRun) Start(startedAt time.Time) (IntentAnalysisRun, error) {
	if run.status != IntentRunQueued || startedAt.IsZero() || startedAt.Before(run.queuedAt) {
		return IntentAnalysisRun{}, ErrIntentRunTransition
	}
	startedAt = startedAt.UTC()
	return RestoreIntentAnalysisRun(run.id, run.kind, run.monitorID, run.draftID, run.draftResourceVersion, run.inputHash, IntentRunRunning, run.queuedAt, &startedAt, nil, nil, "")
}

func (run IntentAnalysisRun) Succeed(completedAt time.Time) (IntentAnalysisRun, error) {
	if run.status != IntentRunRunning || completedAt.IsZero() || run.startedAt == nil || completedAt.Before(*run.startedAt) {
		return IntentAnalysisRun{}, ErrIntentRunTransition
	}
	completedAt = completedAt.UTC()
	return RestoreIntentAnalysisRun(run.id, run.kind, run.monitorID, run.draftID, run.draftResourceVersion, run.inputHash, IntentRunSucceeded, run.queuedAt, run.startedAt, &completedAt, nil, "")
}

func (run IntentAnalysisRun) Fail(reason string, completedAt time.Time) (IntentAnalysisRun, error) {
	if run.status != IntentRunQueued && run.status != IntentRunRunning || completedAt.IsZero() {
		return IntentAnalysisRun{}, ErrIntentRunTransition
	}
	completedAt = completedAt.UTC()
	return RestoreIntentAnalysisRun(run.id, run.kind, run.monitorID, run.draftID, run.draftResourceVersion, run.inputHash, IntentRunFailed, run.queuedAt, run.startedAt, &completedAt, nil, reason)
}

func (run IntentAnalysisRun) InvalidateForDraft(currentDraftID, currentDraftResourceVersion int64, invalidatedAt time.Time) (IntentAnalysisRun, bool, error) {
	if currentDraftID <= 0 || currentDraftResourceVersion <= 0 || invalidatedAt.IsZero() {
		return IntentAnalysisRun{}, false, ErrIntentRunTransition
	}
	if currentDraftID == run.draftID && currentDraftResourceVersion == run.draftResourceVersion || run.status == IntentRunInvalidated {
		return run, false, nil
	}
	if invalidatedAt.Before(run.queuedAt) {
		return IntentAnalysisRun{}, false, ErrIntentRunTransition
	}
	invalidatedAt = invalidatedAt.UTC()
	invalidated, err := RestoreIntentAnalysisRun(run.id, run.kind, run.monitorID, run.draftID, run.draftResourceVersion, run.inputHash, IntentRunInvalidated, run.queuedAt, run.startedAt, run.completedAt, &invalidatedAt, run.failureReason)
	if err != nil {
		return IntentAnalysisRun{}, false, err
	}
	return invalidated, true, nil
}

func (run IntentAnalysisRun) UsableForDraft(draftID, draftResourceVersion int64) bool {
	return draftID > 0 && draftResourceVersion > 0 && run.status == IntentRunSucceeded && run.draftID == draftID && run.draftResourceVersion == draftResourceVersion
}

func copyRunTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
