package application

import (
	"context"
	"fmt"
	"time"
)

// ContentMicroEventReader resolves only the active v2 relation needed after
// an ingestion metric change. It must never consult legacy event_contents.
type ContentMicroEventReader interface {
	ListMetricMicroEventIDsForContent(context.Context, int64) ([]int64, error)
}

type EventHeatCalculator interface {
	Calculate(context.Context, CalculateEventHeatCommand) (CalculateEventHeatResult, error)
}

// ContentMetricRefreshService is the narrow bridge exposed to ingestion. A
// metric commit schedules one bounded Product Event Refresh per active v2
// Event on a stable minute boundary and never invokes legacy Event heat.
type ContentMetricRefreshService struct {
	events    ContentMicroEventReader
	scheduler ProductEventRefreshScheduler
	now       func() time.Time
}

func NewContentMetricRefreshService(events ContentMicroEventReader, scheduler ProductEventRefreshScheduler) (*ContentMetricRefreshService, error) {
	return NewContentMetricRefreshServiceWithClock(events, scheduler, func() time.Time { return time.Now().UTC() })
}

func NewContentMetricRefreshServiceWithClock(events ContentMicroEventReader, scheduler ProductEventRefreshScheduler, now func() time.Time) (*ContentMetricRefreshService, error) {
	if events == nil || scheduler == nil || now == nil {
		return nil, fmt.Errorf("content metric refresh dependencies are required")
	}
	return &ContentMetricRefreshService{events: events, scheduler: scheduler, now: now}, nil
}

func (service *ContentMetricRefreshService) RecomputeMetricsForContent(ctx context.Context, contentID int64) error {
	if service == nil || service.events == nil || service.scheduler == nil || service.now == nil || contentID <= 0 {
		return fmt.Errorf("content metric refresh dependencies are required")
	}
	microEventIDs, err := service.events.ListMetricMicroEventIDsForContent(ctx, contentID)
	if err != nil {
		return err
	}
	windowEnd := service.now().UTC().Truncate(time.Minute)
	for _, microEventID := range microEventIDs {
		if _, err := service.scheduler.ScheduleProductEventRefresh(ctx, ScheduleProductEventRefreshCommand{
			MicroEventID: microEventID, WindowEndedAt: windowEnd,
		}); err != nil {
			return fmt.Errorf("schedule micro-event %d refresh: %w", microEventID, err)
		}
	}
	return nil
}
