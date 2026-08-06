package application

import (
	"context"
	"fmt"
)

// ContentEventReader is the Event-owned relation needed after an ingestion
// metric or lifecycle change. Ingestion never reads event_contents directly.
type ContentEventReader interface {
	ListMetricEventIDsForContent(context.Context, int64) ([]int64, error)
}

// ContentMetricRefreshService is the narrow bridge exposed to ingestion. It
// always delegates to the same synchronous Event recomputation use case used
// by governance and evidence changes.
type ContentMetricRefreshService struct {
	events    ContentEventReader
	recompute MetricRecomputer
}

func NewContentMetricRefreshService(events ContentEventReader, recompute MetricRecomputer) (*ContentMetricRefreshService, error) {
	if events == nil || recompute == nil {
		return nil, fmt.Errorf("content metric refresh dependencies are required")
	}
	return &ContentMetricRefreshService{events: events, recompute: recompute}, nil
}

func (service *ContentMetricRefreshService) RecomputeMetricsForContent(ctx context.Context, contentID int64) error {
	if service == nil || service.events == nil || service.recompute == nil || contentID <= 0 {
		return fmt.Errorf("content metric refresh dependencies are required")
	}
	eventIDs, err := service.events.ListMetricEventIDsForContent(ctx, contentID)
	if err != nil {
		return err
	}
	for _, eventID := range eventIDs {
		if err := recomputeCurrentEventMetrics(ctx, service.recompute, eventID); err != nil {
			return err
		}
	}
	return nil
}
