package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type userNotificationProjectorStub struct {
	command application.ProjectUserNotificationCommand
	err     error
}

func (stub *userNotificationProjectorStub) ProjectUserNotification(_ context.Context, command application.ProjectUserNotificationCommand) (application.ProjectUserNotificationResult, error) {
	stub.command = command
	return application.ProjectUserNotificationResult{UserNotificationID: 17, Created: true}, stub.err
}

func TestOutboxProjectionHandlerProjectsExactFactAndAcceptsReplay(t *testing.T) {
	projector := &userNotificationProjectorStub{}
	handler, err := newOutboxProjectionHandler(projector)
	if err != nil {
		t.Fatal(err)
	}
	job := userNotificationProjectionJob(7, 1, time.Now().UTC())
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if projector.command.OutboxEventID != 7 || projector.command.OutboxVersion != 1 {
		t.Fatalf("project command = %#v", projector.command)
	}
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle(replay) error = %v", err)
	}
}

func TestOutboxProjectionHandlerRejectsForgedHashAndClassifiesRepositoryFailure(t *testing.T) {
	projector := &userNotificationProjectorStub{}
	handler, _ := newOutboxProjectionHandler(projector)
	job := userNotificationProjectionJob(7, 1, time.Now().UTC())
	job.Payload.InputHash = "forged"
	if err := handler.Handle(context.Background(), job); !errors.Is(err, queue.ErrPermanent) {
		t.Fatalf("forged job error = %v", err)
	}
	projector.err = sharedrepository.ErrUnavailable
	job = userNotificationProjectionJob(7, 1, time.Now().UTC())
	if err := handler.Handle(context.Background(), job); !errors.Is(err, queue.ErrRetryable) {
		t.Fatalf("repository failure error = %v", err)
	}
}
