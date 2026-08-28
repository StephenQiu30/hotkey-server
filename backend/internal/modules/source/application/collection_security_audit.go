package application

import (
	"context"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

// CollectionSecurityAuditWriter appends a security rejection outside the
// collection failure transaction so a rejected network action cannot erase
// its own audit evidence.
type CollectionSecurityAuditWriter interface {
	WriteIndependent(context.Context, operationsdomain.AuditEntry) error
}

func writeCollectionSecurityRejection(ctx context.Context, writer CollectionSecurityAuditWriter, runID int64, cause error) error {
	if writer == nil || runID <= 0 {
		return nil
	}
	reason := domain.CollectionSecurityRejectionReason(cause)
	if reason == "" {
		return nil
	}
	return writer.WriteIndependent(context.WithoutCancel(ctx), operationsdomain.AuditEntry{
		ActorType: "system", Action: operationsdomain.ActionCollectionSecurityRejected,
		ResourceType: "collection_run", ResourceID: runID,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx),
		After: map[string]any{"reason_code": reason}, Result: operationsdomain.AuditResultDenied,
	})
}
