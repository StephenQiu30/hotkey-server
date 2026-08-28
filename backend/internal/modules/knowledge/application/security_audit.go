package application

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

type VaultSecurityAuditWriter interface {
	WriteIndependent(context.Context, operationsdomain.AuditEntry) error
}

func writeVaultSecurityRejection(ctx context.Context, writer VaultSecurityAuditWriter, documentID int64, err error) error {
	if writer == nil || documentID <= 0 {
		return nil
	}
	reason := domain.VaultRejectionReason(err)
	if reason == "" {
		return nil
	}
	return writer.WriteIndependent(ctx, operationsdomain.AuditEntry{
		ActorType: "system", Action: operationsdomain.ActionKnowledgeProjectionRejected,
		ResourceType: "knowledge_document", ResourceID: documentID,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx),
		After: map[string]any{"reason_code": reason}, Result: operationsdomain.AuditResultDenied,
	})
}
