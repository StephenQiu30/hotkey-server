package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
)

var ErrInvalidEventHeatContract = errors.New("event heat contract is invalid")

type EventHeatWeightsDTO struct {
	Lineage      float64
	Velocity     float64
	Acceleration float64
	Coverage     float64
	Engagement   float64
	Recency      float64
}

type ReadEventHeatTargetQuery struct {
	MicroEventID  int64
	WindowHours   int
	WindowEndedAt time.Time
}

type EventHeatTargetDTO struct {
	MicroEventID              int64
	MicroEventVersion         int64
	HeatProfileID             int64
	HeatProfileVersion        string
	Weights                   EventHeatWeightsDTO
	WindowStartedAt           time.Time
	WindowEndedAt             time.Time
	IndependentLineageRoots   int
	ReportsInWindow           int
	ReportsInPreviousWindow   int
	ReportsInPriorWindow      int
	PublisherCoverage         int
	SourceTypeCoverage        int
	NormalizedEngagement      *float64
	NormalizationFallback     bool
	TemporalBaselineAvailable bool
	AgeHours                  float64
}

type CommitEventHeatSnapshotCommand struct {
	MicroEventID            int64
	MicroEventVersion       int64
	HeatProfileID           int64
	HeatProfileVersion      string
	WindowStartedAt         time.Time
	WindowEndedAt           time.Time
	IndependentLineageRoots int
	Velocity                float64
	Acceleration            float64
	Coverage                float64
	NormalizedEngagement    *float64
	Recency                 float64
	AvailableWeight         float64
	HeatScore               float64
	WarmingUp               bool
	ReasonCodes             []string
}

type EventHeatSnapshotDTO struct {
	ID                      int64
	MicroEventID            int64
	MicroEventVersion       int64
	HeatProfileID           int64
	HeatProfileVersion      string
	WindowStartedAt         time.Time
	WindowEndedAt           time.Time
	IndependentLineageRoots int
	Velocity                float64
	Acceleration            float64
	Coverage                float64
	NormalizedEngagement    *float64
	Recency                 float64
	AvailableWeight         float64
	HeatScore               float64
	WarmingUp               bool
	ReasonCodes             []string
	Created                 bool
}

type CalculateEventHeatCommand struct {
	MicroEventID  int64
	WindowHours   int
	WindowEndedAt time.Time
}

type CalculateEventHeatResult struct{ Snapshot EventHeatSnapshotDTO }

type EventHeatRepository interface {
	ReadEventHeatTarget(context.Context, ReadEventHeatTargetQuery) (EventHeatTargetDTO, error)
	CommitEventHeatSnapshot(context.Context, CommitEventHeatSnapshotCommand) (EventHeatSnapshotDTO, error)
}

type EventHeatService struct{ repository EventHeatRepository }

func NewEventHeatService(repository EventHeatRepository) (*EventHeatService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidEventHeatContract)
	}
	return &EventHeatService{repository: repository}, nil
}

func (service *EventHeatService) Calculate(ctx context.Context, command CalculateEventHeatCommand) (CalculateEventHeatResult, error) {
	if service == nil || service.repository == nil || command.MicroEventID <= 0 ||
		(command.WindowHours != 1 && command.WindowHours != 6 && command.WindowHours != 24) || command.WindowEndedAt.IsZero() {
		return CalculateEventHeatResult{}, ErrInvalidEventHeatContract
	}
	target, err := service.repository.ReadEventHeatTarget(ctx, ReadEventHeatTargetQuery{
		MicroEventID: command.MicroEventID, WindowHours: command.WindowHours, WindowEndedAt: command.WindowEndedAt.UTC(),
	})
	if err != nil {
		return CalculateEventHeatResult{}, fmt.Errorf("read event heat target: %w", err)
	}
	if !validEventHeatTarget(target, command) {
		return CalculateEventHeatResult{}, ErrInvalidEventHeatContract
	}
	calculated, err := eventdomain.CalculateEventHeat(eventdomain.EventHeatInput{
		IndependentLineageRoots: target.IndependentLineageRoots, ReportsInWindow: target.ReportsInWindow,
		ReportsInPreviousWindow: target.ReportsInPreviousWindow, ReportsInPriorWindow: target.ReportsInPriorWindow,
		PublisherCoverage: target.PublisherCoverage, SourceTypeCoverage: target.SourceTypeCoverage,
		NormalizedEngagement: target.NormalizedEngagement, TemporalBaselineAvailable: target.TemporalBaselineAvailable,
		AgeHours: target.AgeHours, ProfileVersion: target.HeatProfileVersion,
		Weights: eventdomain.EventHeatWeights{Lineage: target.Weights.Lineage, Velocity: target.Weights.Velocity,
			Acceleration: target.Weights.Acceleration, Coverage: target.Weights.Coverage,
			Engagement: target.Weights.Engagement, Recency: target.Weights.Recency},
	})
	if err != nil {
		return CalculateEventHeatResult{}, fmt.Errorf("%w: %v", ErrInvalidEventHeatContract, err)
	}
	if target.NormalizationFallback {
		calculated.ReasonCodes = append(calculated.ReasonCodes, "normalization_fallback")
	}
	mutation := CommitEventHeatSnapshotCommand{MicroEventID: target.MicroEventID, MicroEventVersion: target.MicroEventVersion,
		HeatProfileID: target.HeatProfileID, HeatProfileVersion: target.HeatProfileVersion,
		WindowStartedAt: target.WindowStartedAt.UTC(), WindowEndedAt: target.WindowEndedAt.UTC(),
		IndependentLineageRoots: calculated.IndependentLineageRoots, Velocity: roundEventHeat(calculated.Velocity, 7),
		Acceleration: roundEventHeat(calculated.Acceleration, 7), Coverage: roundEventHeat(calculated.Coverage, 7),
		NormalizedEngagement: roundOptionalEventHeat(calculated.NormalizedEngagement, 7), Recency: roundEventHeat(calculated.Recency, 7),
		AvailableWeight: roundEventHeat(calculated.AvailableWeight, 7), HeatScore: roundEventHeat(calculated.Score, 4), WarmingUp: calculated.WarmingUp,
		ReasonCodes: append([]string{}, calculated.ReasonCodes...)}
	snapshot, err := service.repository.CommitEventHeatSnapshot(ctx, mutation)
	if err != nil {
		return CalculateEventHeatResult{}, fmt.Errorf("commit event heat snapshot: %w", err)
	}
	if !eventHeatReceiptMatches(snapshot, mutation) {
		return CalculateEventHeatResult{}, fmt.Errorf("%w: heat receipt changed", ErrInvalidEventHeatContract)
	}
	return CalculateEventHeatResult{Snapshot: snapshot}, nil
}

func validEventHeatTarget(value EventHeatTargetDTO, command CalculateEventHeatCommand) bool {
	return value.MicroEventID == command.MicroEventID && value.MicroEventVersion > 0 && value.HeatProfileID > 0 &&
		value.HeatProfileVersion != "" && value.WindowEndedAt.Equal(command.WindowEndedAt.UTC()) &&
		value.WindowStartedAt.Equal(value.WindowEndedAt.Add(-time.Duration(command.WindowHours)*time.Hour)) &&
		value.IndependentLineageRoots >= 0 && value.ReportsInWindow >= 0 && value.ReportsInPreviousWindow >= 0 &&
		value.ReportsInPriorWindow >= 0 && value.PublisherCoverage >= 0 && value.SourceTypeCoverage >= 0 &&
		!math.IsNaN(value.AgeHours) && !math.IsInf(value.AgeHours, 0) && value.AgeHours >= 0
}

func eventHeatReceiptMatches(value EventHeatSnapshotDTO, command CommitEventHeatSnapshotCommand) bool {
	return value.ID > 0 && value.MicroEventID == command.MicroEventID && value.MicroEventVersion == command.MicroEventVersion &&
		value.HeatProfileID == command.HeatProfileID && value.HeatProfileVersion == command.HeatProfileVersion &&
		value.WindowStartedAt.Equal(command.WindowStartedAt) &&
		value.WindowEndedAt.Equal(command.WindowEndedAt) && value.IndependentLineageRoots == command.IndependentLineageRoots &&
		value.Velocity == command.Velocity && value.Acceleration == command.Acceleration && value.Coverage == command.Coverage &&
		equalOptionalFloat(value.NormalizedEngagement, command.NormalizedEngagement) && value.Recency == command.Recency &&
		value.AvailableWeight == command.AvailableWeight && value.HeatScore == command.HeatScore && value.WarmingUp == command.WarmingUp &&
		slices.Equal(value.ReasonCodes, command.ReasonCodes)
}

func equalOptionalFloat(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func roundEventHeat(value float64, places int) float64 {
	scale := math.Pow10(places)
	return math.Round(value*scale) / scale
}

func roundOptionalEventHeat(value *float64, places int) *float64 {
	if value == nil {
		return nil
	}
	result := roundEventHeat(*value, places)
	return &result
}
