package application

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type EventIntelligenceEntity struct {
	Entity      domain.Entity
	EventEntity domain.EventEntity
}

type EventIntelligenceReadResult struct {
	EventID  int64
	Entities []EventIntelligenceEntity
	Claims   []domain.Claim
}

type EventIntelligenceReadRepository interface {
	ReadEventIntelligence(context.Context, int64) (EventIntelligenceReadResult, error)
}

type EventIntelligenceReadService struct {
	repository EventIntelligenceReadRepository
}

func NewEventIntelligenceReadService(repository EventIntelligenceReadRepository) *EventIntelligenceReadService {
	return &EventIntelligenceReadService{repository: repository}
}

func (service *EventIntelligenceReadService) Read(ctx context.Context, eventID int64) (EventIntelligenceReadResult, error) {
	if service == nil || service.repository == nil || eventID <= 0 {
		return EventIntelligenceReadResult{}, fmt.Errorf("event intelligence reader is required")
	}
	return service.repository.ReadEventIntelligence(ctx, eventID)
}
