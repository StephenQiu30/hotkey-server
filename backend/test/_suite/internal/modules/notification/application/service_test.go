package application

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type notificationRepositoryStub struct {
	query domain.NotificationQuery
	page  domain.NotificationPage
	err   error
}

func (stub *notificationRepositoryStub) ListAfter(_ context.Context, query domain.NotificationQuery) (domain.NotificationPage, error) {
	stub.query = query
	return stub.page, stub.err
}

func TestServiceNormalizesListAndPreservesRole(t *testing.T) {
	repository := &notificationRepositoryStub{page: domain.NotificationPage{NextAfterID: 9}}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	page, err := service.ListAfter(context.Background(), ListInput{Role: domain.AudienceViewer})
	if err != nil || page.NextAfterID != 9 {
		t.Fatalf("ListAfter() = %#v/%v", page, err)
	}
	if repository.query.Role != domain.AudienceViewer || repository.query.Limit != domain.DefaultListLimit {
		t.Fatalf("repository query = %#v", repository.query)
	}
}

func TestServiceRejectsInvalidQueryBeforeRepository(t *testing.T) {
	repository := &notificationRepositoryStub{}
	service, _ := NewService(repository)
	if _, err := service.ListAfter(context.Background(), ListInput{Role: "owner"}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("ListAfter() error = %v, want invalid input", err)
	}
	if repository.query.Role != "" {
		t.Fatalf("repository unexpectedly called with %#v", repository.query)
	}
}
