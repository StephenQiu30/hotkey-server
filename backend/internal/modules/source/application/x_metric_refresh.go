package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

type XMetricSourceReader interface {
	FindByID(context.Context, int64) (*domain.SourceConnection, error)
}

type XMetricObservationWriter interface {
	AppendXMetricObservation(context.Context, int64, time.Time, domain.SourceMetrics) error
}

type XMetricRefreshDependencies struct {
	Sources    XMetricSourceReader
	Admission  CollectionAdmissionAuthorizer
	Connectors domain.CollectionConnectorRegistry
	Candidates domain.XMetricRefreshCandidateReader
	Metrics    XMetricObservationWriter
	Evidence   ContextEvidenceArchiver
	Now        func() time.Time
}

type XMetricRefreshService struct {
	sources    XMetricSourceReader
	admission  CollectionAdmissionAuthorizer
	connectors domain.CollectionConnectorRegistry
	candidates domain.XMetricRefreshCandidateReader
	metrics    XMetricObservationWriter
	evidence   ContextEvidenceArchiver
	now        func() time.Time
}

func NewXMetricRefreshService(dependencies XMetricRefreshDependencies) (*XMetricRefreshService, error) {
	if dependencies.Sources == nil || dependencies.Admission == nil || dependencies.Connectors == nil || dependencies.Candidates == nil || dependencies.Metrics == nil || dependencies.Evidence == nil {
		return nil, errors.New("x metric refresh dependencies are required")
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	return &XMetricRefreshService{
		sources: dependencies.Sources, admission: dependencies.Admission, connectors: dependencies.Connectors, candidates: dependencies.Candidates,
		metrics: dependencies.Metrics, evidence: dependencies.Evidence, now: dependencies.Now,
	}, nil
}

type XMetricRefreshCommand struct {
	SourceConnectionID    int64
	ExpectedSourceVersion int64
}

type XMetricRefreshResult struct {
	CandidateCount   int
	ObservedCount    int
	UnavailableCount int
	DiagnosticCount  int
}

func (service *XMetricRefreshService) Refresh(ctx context.Context, command XMetricRefreshCommand) (XMetricRefreshResult, error) {
	if service == nil || service.sources == nil || service.admission == nil || service.connectors == nil || service.candidates == nil || service.metrics == nil || service.evidence == nil || service.now == nil {
		return XMetricRefreshResult{}, errors.New("x metric refresh service is not initialized")
	}
	if command.SourceConnectionID <= 0 || command.ExpectedSourceVersion <= 0 {
		return XMetricRefreshResult{}, errors.New("x metric refresh source identity is required")
	}
	connection, err := service.sources.FindByID(ctx, command.SourceConnectionID)
	if err != nil {
		return XMetricRefreshResult{}, err
	}
	if connection == nil || connection.ID != command.SourceConnectionID || connection.Version != command.ExpectedSourceVersion {
		return XMetricRefreshResult{}, errors.New("x metric refresh source version changed")
	}
	if connection.SourceType != domain.SourceTypeX || !connection.Enabled || connection.Deleted || !connection.Config.XMetricRefreshEnabled {
		return XMetricRefreshResult{}, nil
	}
	now := service.now().UTC()
	if now.IsZero() {
		return XMetricRefreshResult{}, errors.New("x metric refresh clock returned zero time")
	}
	query := domain.XMetricRefreshCandidateQuery{
		SourceConnectionID: connection.ID,
		PublishedAfter:     now.Add(-time.Duration(connection.Config.XMetricRefreshObservationHours) * time.Hour),
		SnapshotDueBefore:  now.Add(-time.Duration(connection.Config.XMetricRefreshIntervalMinutes) * time.Minute),
		Limit:              connection.Config.XMetricRefreshMaxPostsPerRun,
	}
	if err := query.Validate(); err != nil {
		return XMetricRefreshResult{}, err
	}
	candidates, err := service.candidates.ListXMetricRefreshCandidates(ctx, query)
	if err != nil {
		return XMetricRefreshResult{}, err
	}
	byPostID := make(map[string]domain.XMetricRefreshCandidate, len(candidates))
	postIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return XMetricRefreshResult{}, err
		}
		if _, duplicate := byPostID[candidate.PostID]; duplicate {
			continue
		}
		byPostID[candidate.PostID] = candidate
		postIDs = append(postIDs, candidate.PostID)
	}
	sort.Slice(postIDs, func(left, right int) bool {
		if len(postIDs[left]) != len(postIDs[right]) {
			return len(postIDs[left]) < len(postIDs[right])
		}
		return postIDs[left] < postIDs[right]
	})
	if len(postIDs) == 0 {
		return XMetricRefreshResult{}, nil
	}
	if err := service.admission.AuthorizeCollection(ctx, *connection); err != nil {
		return XMetricRefreshResult{}, err
	}
	connector, err := service.connectors.Resolve(ctx, *connection)
	if err != nil {
		return XMetricRefreshResult{}, err
	}
	if err := connector.Validate(ctx, *connection); err != nil {
		return XMetricRefreshResult{}, err
	}
	lookup, ok := connector.(domain.XPostMetricLookup)
	if !ok {
		return XMetricRefreshResult{}, errors.New("resolved X connector does not support metric lookup")
	}
	lookupResult, err := lookup.LookupPostMetrics(ctx, domain.XPostMetricLookupRequest{SourceConnectionID: connection.ID, PostIDs: postIDs})
	if err != nil {
		return XMetricRefreshResult{}, err
	}
	if len(lookupResult.Snapshots) > 0 {
		snapshots := make([]RawEvidenceSnapshotDTO, len(lookupResult.Snapshots))
		for index, snapshot := range lookupResult.Snapshots {
			snapshots[index] = rawEvidenceSnapshotDTOFromEntity(snapshot)
		}
		if err := service.evidence.ArchiveContext(ctx, ArchiveContextEvidenceCommand{SourceConnectionID: connection.ID, Snapshots: snapshots}); err != nil && !errors.Is(err, domain.ErrRawArchiveNotAuthorized) {
			return XMetricRefreshResult{}, fmt.Errorf("archive X metric lookup evidence: %w", err)
		}
	}
	observed := make(map[string]struct{}, len(lookupResult.Observations))
	for _, observation := range lookupResult.Observations {
		candidate, found := byPostID[observation.PostID]
		if !found || observation.CapturedAt.IsZero() {
			return XMetricRefreshResult{}, errors.New("x metric lookup returned an unknown or invalid observation")
		}
		if _, duplicate := observed[observation.PostID]; duplicate {
			return XMetricRefreshResult{}, errors.New("x metric lookup returned duplicate observations")
		}
		observed[observation.PostID] = struct{}{}
		if err := service.metrics.AppendXMetricObservation(ctx, candidate.ContentID, observation.CapturedAt.UTC(), observation.Metrics); err != nil {
			return XMetricRefreshResult{}, err
		}
	}
	return XMetricRefreshResult{
		CandidateCount: len(postIDs), ObservedCount: len(observed), UnavailableCount: len(postIDs) - len(observed),
		DiagnosticCount: len(lookupResult.Diagnostics),
	}, nil
}
