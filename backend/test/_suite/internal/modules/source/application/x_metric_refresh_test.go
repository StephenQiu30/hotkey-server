package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestXMetricRefreshUsesOneStableLookupAndPersistsOnlyReturnedObservations(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 12, 8, 7, 43, 0, time.UTC)
	connection := domain.SourceConnection{
		ID: 7, Version: 3, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN", Enabled: true,
		Config: domain.DefaultSourceConfig(),
	}
	connection.Config.XMetricRefreshEnabled = true
	connection.Config.XMetricRefreshIntervalMinutes = 60
	connection.Config.XMetricRefreshObservationHours = 48
	connection.Config.XMetricRefreshMaxPostsPerRun = 100
	connection.Config.XMetricRefreshDailyRequestBudget = 24

	reader := &xMetricCandidateReaderFake{items: []domain.XMetricRefreshCandidate{
		{ContentID: 12, PostID: "10"},
		{ContentID: 11, PostID: "9"},
		{ContentID: 12, PostID: "10"},
	}}
	zero := int64(0)
	lookup := &xMetricLookupFake{result: domain.XPostMetricLookupResult{
		Observations: []domain.XPostMetricObservation{{PostID: "10", CapturedAt: now, Metrics: domain.SourceMetrics{ViewCount: &zero}}},
		Snapshots:    []domain.EvidenceSnapshot{xMetricSnapshot(t, now)},
		Diagnostics:  []domain.FetchDiagnostic{{Code: "unavailable_post"}},
	}}
	writer := &xMetricObservationWriterFake{}
	archiver := &xMetricContextEvidenceArchiverFake{}
	admission := &xMetricAdmissionFake{}
	service, err := NewXMetricRefreshService(XMetricRefreshDependencies{
		Sources:    &xMetricSourceReaderFake{connection: &connection},
		Admission:  admission,
		Connectors: &xMetricConnectorRegistryFake{lookup: lookup},
		Candidates: reader, Metrics: writer, Evidence: archiver,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewXMetricRefreshService: %v", err)
	}

	result, err := service.Refresh(context.Background(), XMetricRefreshCommand{SourceConnectionID: 7, ExpectedSourceVersion: 3})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if reader.query.SourceConnectionID != 7 || reader.query.Limit != 100 ||
		!reader.query.PublishedAfter.Equal(now.Add(-48*time.Hour)) || !reader.query.SnapshotDueBefore.Equal(now.Add(-time.Hour)) {
		t.Fatalf("candidate query = %#v", reader.query)
	}
	if got := lookup.request.PostIDs; len(got) != 2 || got[0] != "9" || got[1] != "10" {
		t.Fatalf("lookup IDs = %#v, want stable [9 10]", got)
	}
	if archiver.command.SourceConnectionID != 7 || len(archiver.command.Snapshots) != 1 {
		t.Fatalf("archive command = %#v", archiver.command)
	}
	if len(writer.writes) != 1 || writer.writes[0].contentID != 12 || writer.writes[0].metrics.ViewCount == nil || *writer.writes[0].metrics.ViewCount != 0 {
		t.Fatalf("metric writes = %#v", writer.writes)
	}
	if result.CandidateCount != 2 || result.ObservedCount != 1 || result.UnavailableCount != 1 || result.DiagnosticCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	if admission.calls != 1 {
		t.Fatalf("metric lookup admission calls = %d, want 1", admission.calls)
	}
}

func TestXMetricRefreshIsDisabledByDefaultAndMakesNoLookup(t *testing.T) {
	t.Parallel()
	connection := domain.SourceConnection{
		ID: 7, Version: 3, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: "env:X_BEARER_TOKEN", Enabled: true, Config: domain.DefaultSourceConfig(),
	}
	lookup := &xMetricLookupFake{}
	service, err := NewXMetricRefreshService(XMetricRefreshDependencies{
		Sources: &xMetricSourceReaderFake{connection: &connection}, Admission: &xMetricAdmissionFake{}, Connectors: &xMetricConnectorRegistryFake{lookup: lookup},
		Candidates: &xMetricCandidateReaderFake{}, Metrics: &xMetricObservationWriterFake{}, Evidence: &xMetricContextEvidenceArchiverFake{},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Refresh(context.Background(), XMetricRefreshCommand{SourceConnectionID: 7, ExpectedSourceVersion: 3})
	if err != nil || result != (XMetricRefreshResult{}) || lookup.calls != 0 {
		t.Fatalf("disabled refresh = %#v, %v, lookup calls %d", result, err, lookup.calls)
	}
}

func TestXMetricRefreshAdmissionDenialStopsBeforeConnectorResolution(t *testing.T) {
	now := time.Date(2026, time.August, 29, 13, 0, 0, 0, time.UTC)
	connection := domain.SourceConnection{
		ID: 7, Version: 3, SourceType: domain.SourceTypeX, Name: "X", Endpoint: domain.XRecentSearchEndpoint,
		AuthType: domain.AuthTypeBearer, CredentialRef: domain.ManagedCredentialReference, Enabled: true, HealthStatus: domain.HealthStatusHealthy,
		Config: domain.DefaultSourceConfig(),
	}
	connection.Config.XMetricRefreshEnabled = true
	registry := &xMetricConnectorRegistryFake{}
	service, err := NewXMetricRefreshService(XMetricRefreshDependencies{
		Sources:    &xMetricSourceReaderFake{connection: &connection},
		Admission:  &xMetricAdmissionFake{err: domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("fixture denial"))},
		Connectors: registry, Candidates: &xMetricCandidateReaderFake{items: []domain.XMetricRefreshCandidate{{ContentID: 12, PostID: "10"}}},
		Metrics: &xMetricObservationWriterFake{}, Evidence: &xMetricContextEvidenceArchiverFake{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(context.Background(), XMetricRefreshCommand{SourceConnectionID: 7, ExpectedSourceVersion: 3}); domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited {
		t.Fatalf("metric admission denial error = %v", err)
	}
	if registry.calls != 0 {
		t.Fatalf("connector resolutions after admission denial = %d", registry.calls)
	}
}

type xMetricSourceReaderFake struct{ connection *domain.SourceConnection }

func (fake *xMetricSourceReaderFake) FindByID(context.Context, int64) (*domain.SourceConnection, error) {
	return fake.connection, nil
}

type xMetricConnectorRegistryFake struct {
	lookup domain.XPostMetricLookup
	calls  int
}

func (fake *xMetricConnectorRegistryFake) Resolve(context.Context, domain.SourceConnection) (domain.Connector, error) {
	fake.calls++
	return xMetricConnectorAdapter{XPostMetricLookup: fake.lookup}, nil
}

type xMetricAdmissionFake struct {
	calls int
	err   error
}

func (fake *xMetricAdmissionFake) AuthorizeCollection(context.Context, domain.SourceConnection) error {
	fake.calls++
	return fake.err
}

type xMetricConnectorAdapter struct{ domain.XPostMetricLookup }

func (xMetricConnectorAdapter) Validate(context.Context, domain.SourceConnection) error { return nil }
func (xMetricConnectorAdapter) Fetch(context.Context, domain.FetchRequest) (domain.FetchResult, error) {
	return domain.FetchResult{}, nil
}
func (xMetricConnectorAdapter) Health(context.Context, domain.SourceConnection) domain.HealthResult {
	return domain.HealthResult{Healthy: true}
}

type xMetricCandidateReaderFake struct {
	query domain.XMetricRefreshCandidateQuery
	items []domain.XMetricRefreshCandidate
}

func (fake *xMetricCandidateReaderFake) ListXMetricRefreshCandidates(_ context.Context, query domain.XMetricRefreshCandidateQuery) ([]domain.XMetricRefreshCandidate, error) {
	fake.query = query
	return fake.items, nil
}

type xMetricLookupFake struct {
	request domain.XPostMetricLookupRequest
	result  domain.XPostMetricLookupResult
	calls   int
}

func (fake *xMetricLookupFake) LookupPostMetrics(_ context.Context, request domain.XPostMetricLookupRequest) (domain.XPostMetricLookupResult, error) {
	fake.calls++
	fake.request = request
	return fake.result, nil
}

type xMetricWrite struct {
	contentID  int64
	capturedAt time.Time
	metrics    domain.SourceMetrics
}

type xMetricObservationWriterFake struct{ writes []xMetricWrite }

func (fake *xMetricObservationWriterFake) AppendXMetricObservation(_ context.Context, contentID int64, capturedAt time.Time, metrics domain.SourceMetrics) error {
	fake.writes = append(fake.writes, xMetricWrite{contentID: contentID, capturedAt: capturedAt, metrics: metrics})
	return nil
}

type xMetricContextEvidenceArchiverFake struct{ command ArchiveContextEvidenceCommand }

func (fake *xMetricContextEvidenceArchiverFake) ArchiveContext(_ context.Context, command ArchiveContextEvidenceCommand) error {
	fake.command = command
	return nil
}

func xMetricSnapshot(t *testing.T, capturedAt time.Time) domain.EvidenceSnapshot {
	t.Helper()
	profile, err := domain.NewCollectorProfileVersion("x-post-lookup-json-v1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Payload: []byte(`{"data":[]}`), CollectorProfileVersion: profile, MIMEType: "application/json",
		StatusCode: 200, RequestedURL: "https://api.x.com/2/tweets?ids=9%2C10", FinalURL: "https://api.x.com/2/tweets?ids=9%2C10", CapturedAt: capturedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
