package bootstrap

import (
	"context"
	"strings"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type requestBoundaryAuditAdapter struct {
	writer *operationspostgres.AuditWriter
}

func newAuditedHTTPRouter(
	readiness httptransport.Readiness,
	metrics *observability.Metrics,
	telemetry *observability.Telemetry,
	logger *zap.Logger,
	cfg config.Config,
	writer *operationspostgres.AuditWriter,
) *gin.Engine {
	return httptransport.NewRouterWithRequestBoundaryAudit(
		readiness, metrics, telemetry, logger, cfg,
		requestBoundaryAuditAdapter{writer: writer},
	)
}

func (adapter requestBoundaryAuditAdapter) WriteRequestBoundaryRejection(ctx context.Context, rejection httptransport.RequestBoundaryRejection) error {
	if adapter.writer == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	reasonCode := strings.TrimSpace(string(rejection.Class)) + "_" + strings.TrimSpace(string(rejection.Reason))
	return adapter.writer.WriteIndependent(auditContext, operationsdomain.AuditEntry{
		ActorType:    "system",
		Action:       operationsdomain.ActionRequestBoundaryRejected,
		ResourceType: "request_boundary",
		RequestID:    rejection.RequestID,
		TraceID:      rejection.TraceID,
		After: map[string]any{
			"boundary_profile_version": rejection.ProfileVersion,
			"reason_code":              reasonCode,
		},
		Result: operationsdomain.AuditResultDenied,
		IPHash: rejection.ClientHash,
	})
}
