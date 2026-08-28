package application

import (
	"context"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

// ContentSecurityAuditWriter persists a rejected publication attempt outside
// the report transaction so rollback cannot erase the security evidence.
type ContentSecurityAuditWriter interface {
	WriteIndependent(context.Context, operationsdomain.AuditEntry) error
}

func writeContentSecurityRejection(ctx context.Context, writer ContentSecurityAuditWriter, actorID, reportID int64) error {
	if writer == nil || actorID <= 0 || reportID <= 0 {
		return nil
	}
	return writer.WriteIndependent(context.WithoutCancel(ctx), operationsdomain.AuditEntry{
		ActorType: "user", ActorID: actorID,
		Action:       operationsdomain.ActionReportContentRejected,
		ResourceType: "report", ResourceID: reportID,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx),
		After:  map[string]any{"reason_code": "report_content_unsafe"},
		Result: operationsdomain.AuditResultDenied,
	})
}
