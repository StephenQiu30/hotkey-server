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
// metric commit recomputes all supported Heat v2 windows on one stable minute
// boundary and never invokes legacy Event heat.
type ContentMetricRefreshService struct {
	events     ContentMicroEventReader
	calculator EventHeatCalculator
	now        func() time.Time
}

func NewContentMetricRefreshService(events ContentMicroEventReader, calculator EventHeatCalculator) (*ContentMetricRefreshService, error) {
	return NewContentMetricRefreshServiceWithClock(events, calculator, func() time.Time { return time.Now().UTC() })
}

func NewContentMetricRefreshServiceWithClock(events ContentMicroEventReader, calculator EventHeatCalculator, now func() time.Time) (*ContentMetricRefreshService, error) {
	if events == nil || calculator == nil || now == nil {
		return nil, fmt.Errorf("content metric refresh dependencies are required")
	}
	return &ContentMetricRefreshService{events: events, calculator: calculator, now: now}, nil
}

func (service *ContentMetricRefreshService) RecomputeMetricsForContent(ctx context.Context, contentID int64) error {
	if service == nil || service.events == nil || service.calculator == nil || service.now == nil || contentID <= 0 {
		return fmt.Errorf("content metric refresh dependencies are required")
	}
	microEventIDs, err := service.events.ListMetricMicroEventIDsForContent(ctx, contentID)
	if err != nil {
		return err
	}
	windowEnd := service.now().UTC().Truncate(time.Minute)
	for _, microEventID := range microEventIDs {
		for _, windowHours := range []int{1, 6, 24} {
			if _, err := service.calculator.Calculate(ctx, CalculateEventHeatCommand{
				MicroEventID: microEventID, WindowHours: windowHours, WindowEndedAt: windowEnd,
			}); err != nil {
				return fmt.Errorf("recompute micro-event %d heat window %dh: %w", microEventID, windowHours, err)
			}
		}
	}
	return nil
}
