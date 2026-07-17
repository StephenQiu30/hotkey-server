package application

import (
	"context"
	"testing"
)

type contentEventReaderFake struct {
	eventIDs []int64
}

func (fake contentEventReaderFake) ListMetricEventIDsForContent(context.Context, int64) ([]int64, error) {
	return append([]int64(nil), fake.eventIDs...), nil
}

func TestContentMetricRefreshDelegatesToTheMetricRecomputeUseCase(t *testing.T) {
	recompute := &metricRecomputeFake{}
	service, err := NewContentMetricRefreshService(contentEventReaderFake{eventIDs: []int64{3, 7}}, recompute)
	if err != nil {
		t.Fatalf("NewContentMetricRefreshService: %v", err)
	}
	if err := service.RecomputeMetricsForContent(context.Background(), 11); err != nil {
		t.Fatalf("RecomputeMetricsForContent: %v", err)
	}
	if len(recompute.commands) != 2 || recompute.commands[0].EventID != 3 || recompute.commands[1].EventID != 7 {
		t.Fatalf("metric recompute commands = %#v", recompute.commands)
	}
}
