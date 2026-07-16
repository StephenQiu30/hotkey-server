package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

var ErrTransactionRequired = errors.New("operations audit writer requires a caller transaction")

type AuditWriter struct{ runtime *database.Runtime }

var _ operationsapplication.AuditWriter = (*AuditWriter)(nil)

func NewAuditWriter(runtime *database.Runtime) *AuditWriter { return &AuditWriter{runtime: runtime} }

// Write intentionally does not open a transaction. Monitor/Source services
// must call it from Runtime.WithinTransaction so the business fact and audit
// row succeed or roll back together.
func (writer *AuditWriter) Write(ctx context.Context, entry operationsdomain.AuditEntry) error {
	if writer == nil || writer.runtime == nil || writer.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	transaction, ok := database.TransactionFromContext(ctx)
	if !ok {
		return ErrTransactionRequired
	}
	if err := entry.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	before, err := marshalMetadata(entry.Before)
	if err != nil {
		return err
	}
	after, err := marshalMetadata(entry.After)
	if err != nil {
		return err
	}
	_, err = transaction.SQL.ExecContext(ctx, `
INSERT INTO audit_logs (actor_type, actor_id, action, resource_type, resource_id, request_id, trace_id, before_data, after_data, result, ip_hash)
VALUES ($1, NULLIF($2, 0), $3, $4, NULLIF($5, 0), NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10, NULLIF($11, ''))`,
		entry.ActorType, entry.ActorID, string(entry.Action), entry.ResourceType, entry.ResourceID, entry.RequestID, entry.TraceID, before, after, string(entry.Result), entry.IPHash)
	return sharedrepository.MapError(err)
}

func marshalMetadata(metadata map[string]any) ([]byte, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	if err := operationsdomain.ValidateMetadata(metadata); err != nil {
		return nil, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("%w: encode audit metadata: %v", sharedrepository.ErrInvalidInput, err)
	}
	return encoded, nil
}

var _ interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} = (*sql.Tx)(nil)
