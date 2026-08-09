package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/jobs"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

var intentAnalysisProfilePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,63}$`)

type IntentRepository struct {
	runtime *database.Runtime
	jobs    *queue.Store
}

var (
	_ monitorapplication.IntentDraftRepository         = (*IntentRepository)(nil)
	_ monitorapplication.IntentDraftRevisionRepository = (*IntentRepository)(nil)
	_ monitorapplication.CurrentIntentDraftRepository  = (*IntentRepository)(nil)
	_ monitorapplication.IntentRunRepository           = (*IntentRepository)(nil)
	_ monitorapplication.IntentAnalysisTaskRepository  = (*IntentRepository)(nil)
	_ monitorapplication.IntentRunStatusRepository     = (*IntentRepository)(nil)
	_ monitorapplication.IntentControlAuthorizer       = (*IntentRepository)(nil)
)

func NewIntentRepository(runtime *database.Runtime) (*IntentRepository, error) {
	if runtime == nil || runtime.SQL == nil || runtime.GORM == nil {
		return nil, fmt.Errorf("monitor intent database runtime is required")
	}
	return &IntentRepository{runtime: runtime, jobs: queue.NewStore(runtime)}, nil
}

type intentExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *IntentRepository) withIntentTransaction(ctx context.Context, operation func(context.Context, intentExecutor) error) error {
	if repository == nil || repository.runtime == nil || repository.jobs == nil {
		return fmt.Errorf("monitor intent repository is unavailable")
	}
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction.SQL)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		return operation(transactionCtx, transaction.SQL)
	})
}

func (repository *IntentRepository) intentExecutor(ctx context.Context) intentExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func mapIntentDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	return databaserepository.MapError(err)
}

func validIntentRepositoryRunTask(request monitorapplication.ReserveIntentRunDTO) bool {
	if request.IdempotencyKey == "" || len([]byte(request.IdempotencyKey)) > 128 || request.IdempotencyKey != trimIntentRepositoryText(request.IdempotencyKey) ||
		strings.ContainsAny(request.IdempotencyKey, "\x00\r\n") ||
		request.RequestedAt.IsZero() || request.Task.MonitorID <= 0 || request.Task.DraftID <= 0 || request.Task.DraftResourceVersion <= 0 ||
		validateIntentRecordHash(request.RequestHash) != nil || validateIntentRecordHash(request.Task.InputHash) != nil ||
		!intentAnalysisProfilePattern.MatchString(request.Task.AnalysisProfile) {
		return false
	}
	switch request.Task.Kind {
	case "expansion":
		return request.Task.AnalysisProfile == monitorapplication.IntentExpansionProfile && request.Task.SampleLimit == 0
	case "preview":
		return request.Task.SampleLimit >= 1 && request.Task.SampleLimit <= 200
	default:
		return false
	}
}

func trimIntentRepositoryText(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\r' || value[start] == '\n') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\r' || value[end-1] == '\n') {
		end--
	}
	return value[start:end]
}

func sameIntentRunLifecycle(left, right monitorapplication.IntentRunDTO) bool {
	return left.ID == right.ID && left.Kind == right.Kind && left.MonitorID == right.MonitorID && left.DraftID == right.DraftID &&
		left.DraftResourceVersion == right.DraftResourceVersion && left.InputHash == right.InputHash && left.Status == right.Status &&
		left.QueuedAt.Equal(right.QueuedAt) && sameIntentOptionalTime(left.StartedAt, right.StartedAt) &&
		sameIntentOptionalTime(left.CompletedAt, right.CompletedAt) && sameIntentOptionalTime(left.InvalidatedAt, right.InvalidatedAt) &&
		left.FailureReason == right.FailureReason
}

func sameIntentOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func sameIntentReservation(record intentAnalysisRunRecord, request monitorapplication.ReserveIntentRunDTO) bool {
	return record.RequestHash == request.RequestHash && record.Kind == request.Task.Kind && record.MonitorID == request.Task.MonitorID &&
		record.DraftID == request.Task.DraftID && record.DraftResourceVersion == request.Task.DraftResourceVersion &&
		record.InputHash == request.Task.InputHash && record.AnalysisProfile == request.Task.AnalysisProfile && record.SampleLimit == request.Task.SampleLimit
}

func sameIntentDraftDTO(left, right monitorapplication.IntentDraftDTO) bool {
	return reflect.DeepEqual(left, right)
}

func intentDraftDefinitionMatches(left, right monitorapplication.IntentDraftDTO) bool {
	return left.MonitorID == right.MonitorID && left.DraftID == right.DraftID && left.Objective == right.Objective &&
		reflect.DeepEqual(left.Clauses, right.Clauses) && reflect.DeepEqual(left.Entities, right.Entities) && reflect.DeepEqual(left.Examples, right.Examples)
}

func validIntentCandidateReviewMutation(current, next monitorapplication.IntentDraftDTO) bool {
	if !intentDraftDefinitionMatches(current, next) || len(current.Candidates) != len(next.Candidates) {
		return false
	}
	changed := 0
	for index := range current.Candidates {
		before, after := current.Candidates[index], next.Candidates[index]
		immutableBefore, immutableAfter := before, after
		immutableBefore.ApprovalStatus, immutableBefore.ReviewerUserID, immutableBefore.ReviewedAt, immutableBefore.ReviewNote = "", nil, nil, ""
		immutableAfter.ApprovalStatus, immutableAfter.ReviewerUserID, immutableAfter.ReviewedAt, immutableAfter.ReviewNote = "", nil, nil, ""
		if !reflect.DeepEqual(immutableBefore, immutableAfter) {
			return false
		}
		if reflect.DeepEqual(before, after) {
			continue
		}
		changed++
		if before.ApprovalStatus != "pending" || (after.ApprovalStatus != "approved" && after.ApprovalStatus != "rejected") ||
			after.ReviewerUserID == nil || *after.ReviewerUserID <= 0 || after.ReviewedAt == nil {
			return false
		}
	}
	return changed == 1
}

func encodeIntentRunJobArgs(runID int64, request monitorapplication.ReserveIntentRunDTO) ([]byte, error) {
	return monitorjobs.EncodeIntentAnalysisJobArgs(monitorjobs.IntentAnalysisJobArgs{
		RunID: runID, DraftID: request.Task.DraftID, DraftResourceVersion: request.Task.DraftResourceVersion,
	})
}

func intentRunNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return monitorapplication.ErrIntentRunNotFound
	}
	return mapIntentDatabaseError(err)
}
