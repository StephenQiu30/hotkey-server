package application

import (
	"context"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type HeatSnapshotStore interface {
	SaveHeatSnapshot(context.Context, domain.HeatResult) error
	LatestHeatSnapshot(context.Context, int64) (domain.HeatResult, error)
}

type HeatService struct{ snapshots HeatSnapshotStore }

func NewHeatService(snapshots HeatSnapshotStore) *HeatService {
	return &HeatService{snapshots: snapshots}
}

func (service *HeatService) CalculateAndSave(ctx context.Context, input domain.HeatInput, previous float64) (domain.HeatResult, error) {
	if service == nil || service.snapshots == nil {
		return domain.HeatResult{}, fmt.Errorf("heat snapshot store is required")
	}
	result, err := domain.CalculateHeat(input)
	if err != nil {
		return domain.HeatResult{}, err
	}
	trend, err := domain.CalculateTrend(result.HeatScore, previous)
	if err != nil {
		return domain.HeatResult{}, err
	}
	result.TrendScore = trend.Score
	if err := service.snapshots.SaveHeatSnapshot(ctx, result); err != nil {
		return domain.HeatResult{}, err
	}
	return result, nil
}

type HeatSnapshotQuery struct {
	EventID int64
	At      time.Time
}
