package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestCollectionServiceArchivesFilteredRawEvidenceBeforeLegacyPersistence(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "raw-evidence-order", 1)
	snapshot := collectionEvidenceSnapshot(t, request.WindowStart)
	connector := &collectionConnectorFake{result: domain.FetchResult{
		Snapshots: []domain.EvidenceSnapshot{snapshot},
		Items: []domain.SourceItem{
			{SourceCode: "rss", ExternalID: "accepted", ContentType: "article", Title: "Climate accepted", Body: "body", ObservedAt: request.WindowStart},
			{SourceCode: "rss", ExternalID: "excluded", ContentType: "article", Title: "Climate blocked", Body: "body", ObservedAt: request.WindowStart},
		},
	}}
	request.Targets[0].Terms = append(request.Targets[0].Terms, domain.CollectionTerm{Value: "blocked", Excluded: true})
	archiver := &collectionEvidenceArchiverFake{runtime: runtime}
	service, err := newCollectionServiceForTest(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: connector}, Evidence: archiver, Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Collect(context.Background(), request)
	if err != nil || run.Status != domain.CollectionRunSucceeded {
		t.Fatalf("Collect() run/error = %#v / %v", run, err)
	}
	if archiver.calls != 1 || !archiver.beforeLegacyPersistence || archiver.command.CollectionRunID != run.ID {
		t.Fatalf("archive order/calls/command = %t / %d / %#v", archiver.beforeLegacyPersistence, archiver.calls, archiver.command)
	}
	if len(archiver.command.Fetch.Items) != 1 || archiver.command.Fetch.Items[0].ExternalID != "accepted" {
		t.Fatalf("archive received unfiltered observations: %#v", archiver.command.Fetch.Items)
	}
}

func TestCollectionServiceTreatsMissingRawStorageRightsAsPolicySkip(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "raw-evidence-policy-skip", 1)
	snapshot := collectionEvidenceSnapshot(t, request.WindowStart)
	archiver := &collectionEvidenceArchiverFake{runtime: runtime, err: domain.ErrRawArchiveNotAuthorized}
	service, err := newCollectionServiceForTest(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{result: domain.FetchResult{
			Snapshots: []domain.EvidenceSnapshot{snapshot},
			Items:     []domain.SourceItem{{SourceCode: "rss", ExternalID: "policy-skip", ContentType: "article", Title: "Climate", ObservedAt: request.WindowStart}},
		}}}, Evidence: archiver, Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Collect(context.Background(), request)
	if err != nil || run.Status != domain.CollectionRunSucceeded {
		t.Fatalf("policy-blocked raw archive changed fetch outcome: %#v / %v", run, err)
	}
}

func TestCollectionServiceFailsRetryablyWhenAuthorizedRawArchiveStorageFails(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "raw-evidence-store-failure", 1)
	snapshot := collectionEvidenceSnapshot(t, request.WindowStart)
	archiver := &collectionEvidenceArchiverFake{runtime: runtime, err: errors.New("object store failed")}
	service, err := newCollectionServiceForTest(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionConnectorRegistryFake{connector: &collectionConnectorFake{result: domain.FetchResult{
			Snapshots: []domain.EvidenceSnapshot{snapshot},
			Items:     []domain.SourceItem{{SourceCode: "rss", ExternalID: "store-failure", ContentType: "article", Title: "Climate", ObservedAt: request.WindowStart}},
		}}}, Evidence: archiver, Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Collect(context.Background(), request)
	if err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorTemporary || run.Status != domain.CollectionRunFailed {
		t.Fatalf("raw archive store failure = %#v / %v, want retryable failed run", run, err)
	}
	var legacyItems int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id=$1`, run.ID).Scan(&legacyItems); err != nil {
		t.Fatal(err)
	}
	if legacyItems != 0 {
		t.Fatalf("legacy items persisted before raw archive succeeded: %d", legacyItems)
	}
}

type collectionEvidenceArchiverFake struct {
	runtime                 *database.Runtime
	command                 sourceapplication.ArchiveCollectionEvidenceCommand
	err                     error
	calls                   int
	beforeLegacyPersistence bool
}

func (archiver *collectionEvidenceArchiverFake) ArchiveFetch(_ context.Context, command sourceapplication.ArchiveCollectionEvidenceCommand) (sourceapplication.ArchiveCollectionEvidenceResult, error) {
	archiver.calls++
	archiver.command = command
	var count int
	if archiver.runtime != nil && archiver.runtime.SQL.QueryRow(`SELECT count(*) FROM collection_run_items WHERE run_id=$1`, command.CollectionRunID).Scan(&count) == nil {
		archiver.beforeLegacyPersistence = count == 0
	}
	return sourceapplication.ArchiveCollectionEvidenceResult{}, archiver.err
}

func collectionEvidenceSnapshot(t *testing.T, capturedAt time.Time) domain.EvidenceSnapshot {
	t.Helper()
	profile, err := domain.NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Payload: []byte("<feed><entry>Climate</entry></feed>"), CollectorProfileVersion: profile,
		MIMEType: "application/rss+xml", StatusCode: 200,
		RequestedURL: "https://feed.example.test/rss", FinalURL: "https://feed.example.test/rss", CapturedAt: capturedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
