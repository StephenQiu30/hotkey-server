package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const ProductEventRefreshWindowProfile = "1h-6h-24h-v1"

var ErrInvalidProductEventRefreshContract = errors.New("product event refresh contract is invalid")

type ProductEventRefreshScheduleTargetQuery struct {
	MicroEventID         int64
	ExpectedEventVersion int64
}

type ProductEventRefreshTargetQuery struct {
	MicroEventID                  int64
	ExpectedEventVersion          int64
	HeatProfileVersion            string
	EvidenceStateAlgorithmVersion string
}

type ProductEventRefreshTargetDTO struct {
	MicroEventID                  int64
	MicroEventVersion             int64
	HeatProfileID                 int64
	HeatProfileVersion            string
	EvidenceStateProfileID        int64
	EvidenceStateAlgorithmVersion string
}

type ScheduleProductEventRefreshCommand struct {
	MicroEventID         int64
	ExpectedEventVersion int64
	WindowEndedAt        time.Time
}

type ScheduleProductEventRefreshResult struct {
	MicroEventID, MicroEventVersion int64
	JobID                           int64
	Created                         bool
	Available                       bool
}

type ProductEventRefreshScheduler interface {
	ScheduleProductEventRefresh(context.Context, ScheduleProductEventRefreshCommand) (ScheduleProductEventRefreshResult, error)
}

type RefreshProductEventCommand struct {
	MicroEventID                  int64
	ExpectedEventVersion          int64
	WindowEndedAt                 time.Time
	WindowProfile                 string
	HeatProfileVersion            string
	EvidenceStateAlgorithmVersion string
}

type CommitProductEventUpdateCommand struct {
	MicroEventID, MicroEventVersion               int64
	WindowEndedAt                                 time.Time
	WindowProfile                                 string
	HeatProfileID                                 int64
	HeatProfileVersion                            string
	EvidenceStateProfileID                        int64
	EvidenceStateAlgorithmVersion                 string
	HeatSnapshot1HourID, HeatSnapshot6HourID      int64
	HeatSnapshot24HourID, EvidenceStateSnapshotID int64
	HeatScore                                     float64
	EvidenceState                                 string
	IndependentOriginCount                        int
	ReasonCodes                                   []string
	RefreshKey                                    string
}

type ProductEventUpdateDTO struct {
	ID, Version, MicroEventID, MicroEventVersion  int64
	WindowEndedAt                                 time.Time
	WindowProfile                                 string
	HeatProfileID                                 int64
	HeatProfileVersion                            string
	EvidenceStateProfileID                        int64
	EvidenceStateAlgorithmVersion                 string
	HeatSnapshot1HourID, HeatSnapshot6HourID      int64
	HeatSnapshot24HourID, EvidenceStateSnapshotID int64
	HeatScore                                     float64
	EvidenceState                                 string
	IndependentOriginCount                        int
	ReasonCodes                                   []string
	RefreshKey                                    string
	CreatedAt                                     time.Time
	Created                                       bool
}

type ProductEventAlertEvaluationResult struct {
	CandidateCount, EligibleCount, NotificationCount, DuplicateCount, SuppressedCount int
}

type ProductEventRefreshResult struct {
	Update          ProductEventUpdateDTO
	HeatSnapshots   []EventHeatSnapshotDTO
	EvidenceState   EvidenceStateSnapshotDTO
	AlertEvaluation ProductEventAlertEvaluationResult
}

type ProductEventRefreshRepository interface {
	ReadProductEventRefreshTarget(context.Context, ProductEventRefreshTargetQuery) (ProductEventRefreshTargetDTO, error)
	CommitProductEventUpdate(context.Context, CommitProductEventUpdateCommand) (ProductEventUpdateDTO, error)
}

type ProductEventRefreshScheduleTargetReader interface {
	ReadProductEventRefreshScheduleTarget(context.Context, ProductEventRefreshScheduleTargetQuery) (ProductEventRefreshTargetDTO, error)
}

type ProductEventAlertEvaluator interface {
	EvaluateProductEventUpdate(context.Context, ProductEventUpdateDTO) (ProductEventAlertEvaluationResult, error)
}

type ProductEventEvidenceStateCalculator interface {
	CalculateState(context.Context, CalculateEvidenceStateCommand) (CalculateEvidenceStateResult, error)
}

type ProductEventRefreshService struct {
	repository ProductEventRefreshRepository
	heat       EventHeatCalculator
	evidence   ProductEventEvidenceStateCalculator
	alerts     ProductEventAlertEvaluator
}

func NewProductEventRefreshService(repository ProductEventRefreshRepository, heat EventHeatCalculator,
	evidence ProductEventEvidenceStateCalculator, alerts ProductEventAlertEvaluator) (*ProductEventRefreshService, error) {
	if repository == nil || heat == nil || evidence == nil || alerts == nil {
		return nil, fmt.Errorf("%w: dependencies are required", ErrInvalidProductEventRefreshContract)
	}
	return &ProductEventRefreshService{repository: repository, heat: heat, evidence: evidence, alerts: alerts}, nil
}

func (service *ProductEventRefreshService) Refresh(ctx context.Context, command RefreshProductEventCommand) (ProductEventRefreshResult, error) {
	command.HeatProfileVersion = strings.TrimSpace(command.HeatProfileVersion)
	command.EvidenceStateAlgorithmVersion = strings.TrimSpace(command.EvidenceStateAlgorithmVersion)
	command.WindowProfile = strings.TrimSpace(command.WindowProfile)
	command.WindowEndedAt = command.WindowEndedAt.UTC().Truncate(time.Minute)
	if service == nil || service.repository == nil || service.heat == nil || service.evidence == nil || service.alerts == nil ||
		command.MicroEventID <= 0 || command.ExpectedEventVersion <= 0 || command.WindowEndedAt.IsZero() ||
		command.WindowProfile != ProductEventRefreshWindowProfile || command.HeatProfileVersion == "" ||
		command.EvidenceStateAlgorithmVersion != CanonicalEvidenceStateAlgorithmVersion {
		return ProductEventRefreshResult{}, ErrInvalidProductEventRefreshContract
	}
	target, err := service.repository.ReadProductEventRefreshTarget(ctx, ProductEventRefreshTargetQuery{
		MicroEventID: command.MicroEventID, ExpectedEventVersion: command.ExpectedEventVersion,
		HeatProfileVersion:            command.HeatProfileVersion,
		EvidenceStateAlgorithmVersion: command.EvidenceStateAlgorithmVersion,
	})
	if err != nil {
		return ProductEventRefreshResult{}, fmt.Errorf("read product event refresh target: %w", err)
	}
	if !productEventRefreshTargetMatches(target, command) {
		return ProductEventRefreshResult{}, ErrInvalidProductEventRefreshContract
	}
	heatSnapshots := make([]EventHeatSnapshotDTO, 0, 3)
	for _, hours := range []int{1, 6, 24} {
		calculated, calculateErr := service.heat.Calculate(ctx, CalculateEventHeatCommand{
			MicroEventID: command.MicroEventID, WindowHours: hours, WindowEndedAt: command.WindowEndedAt,
		})
		if calculateErr != nil {
			return ProductEventRefreshResult{}, fmt.Errorf("calculate product event heat %dh: %w", hours, calculateErr)
		}
		if calculated.Snapshot.MicroEventVersion != command.ExpectedEventVersion ||
			calculated.Snapshot.HeatProfileID != target.HeatProfileID ||
			calculated.Snapshot.HeatProfileVersion != target.HeatProfileVersion {
			return ProductEventRefreshResult{}, ErrInvalidProductEventRefreshContract
		}
		heatSnapshots = append(heatSnapshots, calculated.Snapshot)
	}
	evidence, err := service.evidence.CalculateState(ctx, CalculateEvidenceStateCommand{
		MicroEventID: command.MicroEventID, ExpectedEventVersion: command.ExpectedEventVersion,
		AlgorithmVersion: command.EvidenceStateAlgorithmVersion, CalculatedAt: command.WindowEndedAt,
	})
	if err != nil {
		return ProductEventRefreshResult{}, fmt.Errorf("calculate product event evidence state: %w", err)
	}
	if evidence.Snapshot.ProfileID != target.EvidenceStateProfileID ||
		evidence.Snapshot.AlgorithmVersion != target.EvidenceStateAlgorithmVersion {
		return ProductEventRefreshResult{}, ErrInvalidProductEventRefreshContract
	}
	reasons := []string{"product_event_refreshed", "evidence_state_" + evidence.Snapshot.State}
	if heatSnapshots[2].WarmingUp {
		reasons = append(reasons, "heat_warming_up")
	}
	refreshKey := productEventRefreshKey(command, target)
	mutation := CommitProductEventUpdateCommand{
		MicroEventID: command.MicroEventID, MicroEventVersion: command.ExpectedEventVersion,
		WindowEndedAt: command.WindowEndedAt, WindowProfile: command.WindowProfile,
		HeatProfileID: target.HeatProfileID, HeatProfileVersion: target.HeatProfileVersion,
		EvidenceStateProfileID:        target.EvidenceStateProfileID,
		EvidenceStateAlgorithmVersion: target.EvidenceStateAlgorithmVersion,
		HeatSnapshot1HourID:           heatSnapshots[0].ID, HeatSnapshot6HourID: heatSnapshots[1].ID,
		HeatSnapshot24HourID: heatSnapshots[2].ID, EvidenceStateSnapshotID: evidence.Snapshot.ID,
		HeatScore: heatSnapshots[2].HeatScore, EvidenceState: evidence.Snapshot.State,
		IndependentOriginCount: evidence.Snapshot.IndependentOriginCount,
		ReasonCodes:            reasons, RefreshKey: refreshKey,
	}
	update, err := service.repository.CommitProductEventUpdate(ctx, mutation)
	if err != nil {
		return ProductEventRefreshResult{}, fmt.Errorf("commit product event update: %w", err)
	}
	if !productEventUpdateReceiptMatches(update, mutation) {
		return ProductEventRefreshResult{}, ErrInvalidProductEventRefreshContract
	}
	evaluated, err := service.alerts.EvaluateProductEventUpdate(ctx, update)
	if err != nil {
		return ProductEventRefreshResult{}, fmt.Errorf("evaluate product event alerts: %w", err)
	}
	if evaluated.CandidateCount < 0 || evaluated.EligibleCount < 0 || evaluated.NotificationCount < 0 || evaluated.DuplicateCount < 0 || evaluated.SuppressedCount < 0 ||
		evaluated.EligibleCount > evaluated.CandidateCount || evaluated.NotificationCount+evaluated.DuplicateCount+evaluated.SuppressedCount > evaluated.EligibleCount {
		return ProductEventRefreshResult{}, ErrInvalidProductEventRefreshContract
	}
	return ProductEventRefreshResult{Update: update, HeatSnapshots: heatSnapshots,
		EvidenceState: evidence.Snapshot, AlertEvaluation: evaluated}, nil
}

func productEventRefreshTargetMatches(target ProductEventRefreshTargetDTO, command RefreshProductEventCommand) bool {
	return target.MicroEventID == command.MicroEventID && target.MicroEventVersion == command.ExpectedEventVersion &&
		target.HeatProfileID > 0 && target.HeatProfileVersion == command.HeatProfileVersion &&
		target.EvidenceStateProfileID > 0 && target.EvidenceStateAlgorithmVersion == command.EvidenceStateAlgorithmVersion
}

func productEventRefreshKey(command RefreshProductEventCommand, target ProductEventRefreshTargetDTO) string {
	payload, _ := json.Marshal(struct {
		MicroEventID, EventVersion, HeatProfileID, EvidenceProfileID int64
		WindowEndedAt                                                time.Time
		WindowProfile, HeatProfileVersion, EvidenceAlgorithm         string
	}{command.MicroEventID, command.ExpectedEventVersion, target.HeatProfileID, target.EvidenceStateProfileID,
		command.WindowEndedAt, command.WindowProfile, target.HeatProfileVersion, target.EvidenceStateAlgorithmVersion})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func productEventUpdateReceiptMatches(value ProductEventUpdateDTO, command CommitProductEventUpdateCommand) bool {
	return value.ID > 0 && value.Version == 1 && value.MicroEventID == command.MicroEventID &&
		value.MicroEventVersion == command.MicroEventVersion && value.WindowEndedAt.Equal(command.WindowEndedAt) &&
		value.WindowProfile == command.WindowProfile && value.HeatProfileID == command.HeatProfileID &&
		value.HeatProfileVersion == command.HeatProfileVersion && value.EvidenceStateProfileID == command.EvidenceStateProfileID &&
		value.EvidenceStateAlgorithmVersion == command.EvidenceStateAlgorithmVersion &&
		value.HeatSnapshot1HourID == command.HeatSnapshot1HourID && value.HeatSnapshot6HourID == command.HeatSnapshot6HourID &&
		value.HeatSnapshot24HourID == command.HeatSnapshot24HourID && value.EvidenceStateSnapshotID == command.EvidenceStateSnapshotID &&
		value.HeatScore == command.HeatScore && value.EvidenceState == command.EvidenceState &&
		value.IndependentOriginCount == command.IndependentOriginCount && value.RefreshKey == command.RefreshKey &&
		slices.Equal(value.ReasonCodes, command.ReasonCodes)
}
