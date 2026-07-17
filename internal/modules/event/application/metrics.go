package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type HeatSnapshotStore interface {
	SaveHeatSnapshot(context.Context, domain.HeatResult) error
	LatestHeatSnapshot(context.Context, int64) (domain.HeatResult, error)
	LoadMetricEvidence(context.Context, int64, time.Time, int) (domain.MetricEvidenceSet, error)
	ListHeatSnapshots(context.Context, int64, int, time.Time, int) ([]domain.HeatResult, error)
	SaveRecomputedHeatSnapshots(context.Context, []domain.HeatResult) error
}

type HeatServiceDependencies struct {
	Snapshots    HeatSnapshotStore
	Capabilities sourcedomain.MetricCapabilityReader
}

type HeatService struct {
	snapshots    HeatSnapshotStore
	capabilities sourcedomain.MetricCapabilityReader
}

// MetricRecomputer is the narrow synchronous boundary used by Event mutations.
// PLAN-013 may later schedule, de-duplicate and retry this exact operation,
// without letting callers invent a second heat-calculation path.
type MetricRecomputer interface {
	RecomputeEventMetrics(context.Context, MetricRecomputeCommand) ([]domain.HeatResult, error)
}

func recomputeCurrentEventMetrics(ctx context.Context, recomputer MetricRecomputer, eventID int64) error {
	if recomputer == nil || eventID <= 0 {
		return nil
	}
	_, err := recomputer.RecomputeEventMetrics(ctx, MetricRecomputeCommand{EventID: eventID, WindowEnd: time.Now().UTC(), HeatVersion: domain.HeatAlgorithmVersionV1})
	return err
}

func NewHeatService(dependencies HeatServiceDependencies) (*HeatService, error) {
	if dependencies.Snapshots == nil || dependencies.Capabilities == nil {
		return nil, fmt.Errorf("heat service dependencies are required")
	}
	return &HeatService{snapshots: dependencies.Snapshots, capabilities: dependencies.Capabilities}, nil
}

type MetricRecomputeCommand struct {
	EventID     int64
	WindowEnd   time.Time
	HeatVersion string
}

func (command MetricRecomputeCommand) Validate() error {
	if command.EventID <= 0 || command.WindowEnd.IsZero() || strings.TrimSpace(command.HeatVersion) == "" {
		return fmt.Errorf("invalid metric recompute command")
	}
	return nil
}

// RecomputeEventMetrics synchronously appends the 1/6/24-hour snapshots and
// asks the Event repository to atomically update the current Event and Monitor
// projections. Scheduling, deduplication and retries remain PLAN-013 work.
func (service *HeatService) RecomputeEventMetrics(ctx context.Context, command MetricRecomputeCommand) ([]domain.HeatResult, error) {
	if service == nil || service.snapshots == nil || service.capabilities == nil {
		return nil, fmt.Errorf("%w: metric recompute dependencies are required", sharedrepository.ErrUnavailable)
	}
	if err := command.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	windowEnd := command.WindowEnd.UTC().Truncate(time.Hour)
	resultsByWindow := make(map[int]domain.HeatResult, len(domain.MetricWindowsV1()))
	setsByWindow := make(map[int]domain.MetricEvidenceSet, len(domain.MetricWindowsV1()))
	for _, windowHours := range domain.MetricWindowsV1() {
		set, err := service.snapshots.LoadMetricEvidence(ctx, command.EventID, windowEnd, windowHours)
		if err != nil {
			return nil, err
		}
		result, err := service.calculateWindow(ctx, set, windowEnd, windowHours, command.HeatVersion)
		if err != nil {
			return nil, err
		}
		setsByWindow[windowHours], resultsByWindow[windowHours] = set, result
	}
	trend, err := service.calculateTrend(ctx, command.EventID, windowEnd, resultsByWindow[1], resultsByWindow[6], setsByWindow[1])
	if err != nil {
		return nil, err
	}
	results := make([]domain.HeatResult, 0, len(resultsByWindow))
	for _, windowHours := range domain.MetricWindowsV1() {
		result := resultsByWindow[windowHours]
		result.TrendScore, result.TrendStatus = trend.Score, trend.Status
		results = append(results, result)
	}
	if err := service.snapshots.SaveRecomputedHeatSnapshots(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

func (service *HeatService) calculateWindow(ctx context.Context, set domain.MetricEvidenceSet, windowEnd time.Time, windowHours int, heatVersion string) (domain.HeatResult, error) {
	sourceIDs := make([]int64, 0, len(set.Evidence))
	for _, evidence := range set.Evidence {
		sourceIDs = append(sourceIDs, evidence.SourceConnectionID)
	}
	resolved, err := service.capabilities.ResolveMetricSourceCapabilities(ctx, sourceIDs)
	if err != nil {
		return domain.HeatResult{}, err
	}
	capabilities := make(map[int64]domain.MetricCapability, len(resolved))
	capabilityList := make([]domain.MetricCapability, 0, len(resolved))
	for _, item := range resolved {
		capability := metricCapability(item)
		capabilities[capability.SourceConnectionID] = capability
		capabilityList = append(capabilityList, capability)
	}
	populations := make(map[domain.MetricPopulationKey]domain.MetricPopulation, len(set.Populations))
	for _, population := range set.Populations {
		populations[population.MetricPopulationKey] = population
	}
	normalized := make([]domain.NormalizedEvidence, 0, len(set.Evidence))
	selectedCapabilities := make(map[int64]domain.MetricCapability, len(capabilities))
	for _, evidence := range set.Evidence {
		capability, found := capabilities[evidence.SourceConnectionID]
		if !found {
			continue
		}
		population := populations[domain.MetricPopulationKey{SourceConnectionID: evidence.SourceConnectionID, ContentType: evidence.ContentType}]
		engagement, fallback, err := domain.NormalizeEngagement(evidence, capability, population)
		if err != nil {
			return domain.HeatResult{}, err
		}
		independenceKey := fmt.Sprintf("source:%d", evidence.SourceConnectionID)
		if capability.IndependenceStrategy == "author" && evidence.AuthorID != nil {
			independenceKey = fmt.Sprintf("author:%d", *evidence.AuthorID)
		}
		normalized = append(normalized, domain.NormalizedEvidence{ContentID: evidence.ContentID, SourceConnectionID: evidence.SourceConnectionID, IndependenceKey: independenceKey, SourceType: capability.SourceType, PublishedAt: evidence.PublishedAt, EngagementScore: engagement, CredibilityWeight: capability.CredibilityWeight, UsedFallback: fallback})
		selectedCapabilities[capability.SourceConnectionID] = capability
	}
	capabilityList = capabilityList[:0]
	for _, capability := range selectedCapabilities {
		capabilityList = append(capabilityList, capability)
	}
	sort.Slice(capabilityList, func(left, right int) bool {
		return capabilityList[left].SourceConnectionID < capabilityList[right].SourceConnectionID
	})
	eventAge := windowEnd.Sub(set.FirstSeenAt).Hours()
	if eventAge < 0 {
		eventAge = 0
	}
	return domain.CalculateRecomputedHeat(domain.RecomputeHeatInput{EventID: set.EventID, WindowEnd: windowEnd, WindowHours: windowHours, HeatVersion: heatVersion, EvidenceSetHash: domain.EvidenceSetHash(set.Evidence), CapabilityProfileSetHash: domain.CapabilityProfileSetHash(capabilityList), EventAgeHours: eventAge, ActiveEvidenceCount: len(set.Evidence), Evidences: normalized})
}

func (service *HeatService) calculateTrend(ctx context.Context, eventID int64, windowEnd time.Time, short, long domain.HeatResult, set domain.MetricEvidenceSet) (domain.TrendResult, error) {
	shortHistory, err := service.snapshots.ListHeatSnapshots(ctx, eventID, 1, windowEnd, 5)
	if err != nil {
		return domain.TrendResult{}, err
	}
	longHistory, err := service.snapshots.ListHeatSnapshots(ctx, eventID, 6, windowEnd, 5)
	if err != nil {
		return domain.TrendResult{}, err
	}
	shortSeries := append(heatScores(shortHistory), short.HeatScore)
	longSeries := append(heatScores(longHistory), long.HeatScore)
	latestEvidenceAge := windowEnd.Sub(set.LastSeenAt).Hours()
	if latestEvidenceAge < 0 {
		latestEvidenceAge = 0
	}
	eventAge := windowEnd.Sub(set.FirstSeenAt).Hours()
	if eventAge < 0 {
		eventAge = 0
	}
	return domain.CalculateEMATrend(domain.TrendInput{ShortSeries: shortSeries, LongSeries: longSeries, EventAgeHours: eventAge, LatestEvidenceAgeHours: latestEvidenceAge})
}

func metricCapability(item sourcedomain.MetricSourceCapability) domain.MetricCapability {
	profile := item.Profile
	return domain.MetricCapability{SourceConnectionID: item.SourceConnectionID, SourceType: string(item.SourceType), ProfileVersion: profile.ProfileVersion, ProfileID: profile.ID, ProfileRecordVer: profile.Version, SupportsViews: profile.SupportsViews, SupportsLikes: profile.SupportsLikes, SupportsComments: profile.SupportsComments, SupportsShares: profile.SupportsShares, IndependenceStrategy: string(profile.IndependenceStrategy), CredibilityWeight: profile.CredibilityWeight, MaxSingleItemContribution: profile.MaxSingleItemContribution}
}

func heatScores(results []domain.HeatResult) []float64 {
	scores := make([]float64, 0, len(results))
	for _, result := range results {
		scores = append(scores, result.HeatScore)
	}
	return scores
}

func (service *HeatService) Latest(ctx context.Context, eventID int64) (domain.HeatResult, error) {
	if service == nil || service.snapshots == nil || eventID <= 0 {
		return domain.HeatResult{}, fmt.Errorf("%w: invalid heat query", sharedrepository.ErrInvalidInput)
	}
	return service.snapshots.LatestHeatSnapshot(ctx, eventID)
}
