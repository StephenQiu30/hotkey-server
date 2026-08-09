package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestRawEvidenceCollectionServiceResolvesExactSubjectsBeforeArchiving(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	snapshot := archiveSnapshot(t, at, []byte("<feed><entry>one</entry></feed>"), "rss-http-feed-go-xml-v1")
	rights := &recordingRawEvidenceRightsReader{
		result: CurrentRawEvidenceRightsResult{
			StoreRawDecisions: map[string]RawEvidenceRightsDecisionDTO{snapshot.Key: rawStoreAllowDecision(snapshot, at)},
			RetainDecisions:   map[string]RawEvidenceRightsDecisionDTO{snapshot.Key: rawRetainAllowDecision(snapshot, at)},
		},
	}
	archive := &recordingRawEvidenceArchiveUseCase{result: ArchiveRawEvidenceResult{Snapshots: []PersistedEvidenceSnapshotDTO{{ID: 9, EvidenceKey: snapshot.Key}}}}
	service, err := NewRawEvidenceCollectionService(RawEvidenceCollectionServiceDependencies{Rights: rights, Archive: archive, Clock: &archiveClockFake{now: at}})
	if err != nil {
		t.Fatalf("NewRawEvidenceCollectionService() error = %v", err)
	}
	result, err := service.ArchiveFetch(context.Background(), ArchiveCollectionEvidenceCommand{
		SourceConnectionID: 42, CollectionRunID: 77,
		Fetch: rawEvidenceFetchDTOFromEntity(domain.FetchResult{Snapshots: []domain.EvidenceSnapshot{snapshot, snapshot}}),
	})
	if err != nil {
		t.Fatalf("ArchiveFetch() error = %v", err)
	}
	if len(rights.query.Subjects) != 1 || rights.query.Subjects[0].EvidenceKey != snapshot.Key || rights.query.Subjects[0].PayloadSHA256 != snapshot.PayloadSHA256 {
		t.Fatalf("rights query subjects = %#v", rights.query.Subjects)
	}
	if !rights.query.DecisionAt.Equal(at) || rights.query.SourceConnectionID != 42 {
		t.Fatalf("rights query = %#v", rights.query)
	}
	if archive.command.CollectionRunID != 77 || archive.command.StoreRawDecisions[snapshot.Key].Action != string(domain.RightsActionStoreRaw) || len(result.Snapshots) != 1 {
		t.Fatalf("archive command/result = %#v / %#v", archive.command, result)
	}
}

func TestRawEvidenceCollectionServiceNeverDerivesRightsFromLegacyConfiguration(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	snapshot := archiveSnapshot(t, at, []byte("<feed><entry>one</entry></feed>"), "rss-http-feed-go-xml-v1")
	rights := &recordingRawEvidenceRightsReader{result: CurrentRawEvidenceRightsResult{
		StoreRawDecisions: map[string]RawEvidenceRightsDecisionDTO{}, RetainDecisions: map[string]RawEvidenceRightsDecisionDTO{},
	}}
	archive := &recordingRawEvidenceArchiveUseCase{err: domain.ErrRawArchiveNotAuthorized}
	service, err := NewRawEvidenceCollectionService(RawEvidenceCollectionServiceDependencies{Rights: rights, Archive: archive, Clock: &archiveClockFake{now: at}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ArchiveFetch(context.Background(), ArchiveCollectionEvidenceCommand{
		SourceConnectionID: 42, CollectionRunID: 77,
		Fetch: rawEvidenceFetchDTOFromEntity(domain.FetchResult{Snapshots: []domain.EvidenceSnapshot{snapshot}}),
	})
	if err == nil {
		t.Fatal("missing explicit current decisions did not fail closed")
	}
	if len(archive.command.StoreRawDecisions) != 0 || len(archive.command.RetainDecisions) != 0 {
		t.Fatal("collection service manufactured rights decisions")
	}
}

type recordingRawEvidenceRightsReader struct {
	query  CurrentRawEvidenceRightsQuery
	result CurrentRawEvidenceRightsResult
	err    error
}

func (reader *recordingRawEvidenceRightsReader) ResolveCurrent(_ context.Context, query CurrentRawEvidenceRightsQuery) (CurrentRawEvidenceRightsResult, error) {
	reader.query = query
	return reader.result, reader.err
}

type recordingRawEvidenceArchiveUseCase struct {
	command ArchiveRawEvidenceCommand
	result  ArchiveRawEvidenceResult
	err     error
}

func (archive *recordingRawEvidenceArchiveUseCase) Archive(_ context.Context, command ArchiveRawEvidenceCommand) (ArchiveRawEvidenceResult, error) {
	archive.command = command
	return archive.result, archive.err
}

func TestCurrentRawEvidenceRightsQueryRejectsOversizedBatches(t *testing.T) {
	t.Parallel()
	subjects := make([]RawEvidenceRightsSubjectDTO, maximumRawEvidenceRightsSubjects+1)
	for index := range subjects {
		subjects[index] = RawEvidenceRightsSubjectDTO{
			EvidenceKey:   strings.Repeat("a", 63) + string(rune('a'+index%6)),
			PayloadSHA256: strings.Repeat("b", 63) + string(rune('a'+index%6)),
		}
	}
	query := CurrentRawEvidenceRightsQuery{SourceConnectionID: 1, DecisionAt: time.Now().UTC(), Subjects: subjects}
	if err := query.Validate(); err == nil {
		t.Fatal("oversized raw evidence rights batch was accepted")
	}
}
