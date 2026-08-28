package jobs

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type userNotificationProjector interface {
	ProjectUserNotification(context.Context, application.ProjectUserNotificationCommand) (application.ProjectUserNotificationResult, error)
}

type OutboxProjectionHandler struct {
	projector userNotificationProjector
}

func NewOutboxProjectionHandler(service *application.Service) (*OutboxProjectionHandler, error) {
	return newOutboxProjectionHandler(service)
}

func newOutboxProjectionHandler(projector userNotificationProjector) (*OutboxProjectionHandler, error) {
	if projector == nil {
		return nil, fmt.Errorf("notification outbox projector is required")
	}
	return &OutboxProjectionHandler{projector: projector}, nil
}

func (handler *OutboxProjectionHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindProjectUserNotification); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.projector == nil {
		return queue.NewRetryableError(fmt.Errorf("notification outbox projector is unavailable"))
	}
	expectedHash := userNotificationProjectionInputHash(job.Payload.EntityID, job.Payload.EntityVersion)
	if job.Payload.InputHash != expectedHash {
		return queue.NewPermanentError(fmt.Errorf("notification projection input hash is invalid"))
	}
	_, err := handler.projector.ProjectUserNotification(ctx, application.ProjectUserNotificationCommand{
		OutboxEventID: job.Payload.EntityID,
		OutboxVersion: job.Payload.EntityVersion,
	})
	return queue.ClassifyHandlerError(ctx, err)
}

func userNotificationProjectionJob(outboxEventID, outboxVersion int64, scheduledAt time.Time) queue.Job {
	inputHash := userNotificationProjectionInputHash(outboxEventID, outboxVersion)
	return queue.Job{
		Kind: queue.KindProjectUserNotification,
		UniqueKey: queue.StableJobKey(
			queue.KindProjectUserNotification,
			outboxEventID,
			outboxVersion,
			inputHash,
		),
		Payload: queue.Payload{
			EntityID:      outboxEventID,
			EntityVersion: outboxVersion,
			InputHash:     inputHash,
		},
		ScheduledAt: scheduledAt.UTC(),
		MaxAttempts: 5,
		Priority:    6,
	}
}

func userNotificationProjectionInputHash(outboxEventID, outboxVersion int64) string {
	return queue.StableJobHash(
		queue.KindProjectUserNotification,
		strconv.FormatInt(outboxEventID, 10),
		strconv.FormatInt(outboxVersion, 10),
	)
}
