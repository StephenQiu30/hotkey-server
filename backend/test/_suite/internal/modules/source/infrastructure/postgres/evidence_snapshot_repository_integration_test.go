package postgres_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestEvidenceSnapshotRepositoryImplementsApplicationPort(t *testing.T) {
	t.Parallel()
	var repository sourceapplication.EvidenceSnapshotRepository = (*sourcepostgres.EvidenceSnapshotRepository)(nil)
	_ = repository
	for _, value := range []any{
		sourceapplication.ReserveEvidenceSnapshotCommand{},
		sourceapplication.PersistedEvidenceSnapshotDTO{},
		sourceapplication.CommitEvidenceSnapshotCommand{},
		sourceapplication.StoreRawEvidenceResult{},
		sourceapplication.SourceObservationDTO{},
	} {
		typeOf := reflect.TypeOf(value)
		for _, forbidden := range []string{"Payload", "Body", "RawBytes", "RawPayload"} {
			if _, found := typeOf.FieldByName(forbidden); found {
				t.Fatalf("%s exposes forbidden database DTO field %q", typeOf.Name(), forbidden)
			}
		}
	}
}

func TestEvidenceSnapshotRepositoryConcurrentReservePreservesFirstFacts(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	repository := newEvidenceSnapshotRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "concurrent-reserve")
	secondRunID := insertEvidenceCollectionRun(t, runtime.SQL, fixture.SourceID, "second-run")

	first := fixture.Reservation
	second := first
	second.CollectionRunID = secondRunID
	second.MIMEType = "application/xml"
	second.ResponseStatus = 206
	second.RequestedURL = "https://feed.example.test/recaptured.xml"
	second.FinalURL = second.RequestedURL
	second.CapturedAt = first.CapturedAt.Add(time.Minute)
	second.RetentionUntil = second.CapturedAt.Add(30 * 24 * time.Hour)
	second.ResponseHeaders = responseHeaders(t, second.MIMEType, `"recaptured"`)

	start := make(chan struct{})
	results := make(chan sourceapplication.PersistedEvidenceSnapshotDTO, 2)
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for _, command := range []sourceapplication.ReserveEvidenceSnapshotCommand{first, second} {
		command := command
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := repository.Reserve(ctx, command)
			results <- result
			errorsChannel <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent Reserve(): %v", err)
		}
	}
	var returned []sourceapplication.PersistedEvidenceSnapshotDTO
	for result := range results {
		returned = append(returned, result)
	}
	if len(returned) != 2 || returned[0].ID <= 0 || returned[0].ID != returned[1].ID || !samePersistenceDTO(returned[0], returned[1]) {
		t.Fatalf("concurrent reservations diverged: %#v", returned)
	}
	if !persistenceMatchesReservation(returned[0], first) && !persistenceMatchesReservation(returned[0], second) {
		t.Fatalf("persisted facts do not match either first writer: %#v", returned[0])
	}
	var snapshotCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM evidence_snapshots WHERE source_connection_id=$1 AND snapshot_key=$2`, fixture.SourceID, first.EvidenceKey).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if snapshotCount != 1 {
		t.Fatalf("endpoint-scoped reserve created %d rows", snapshotCount)
	}

	later := second
	later.CollectionRunID = insertEvidenceCollectionRun(t, runtime.SQL, fixture.SourceID, "later-run")
	later.CapturedAt = time.Now().UTC().Truncate(time.Microsecond)
	later.RetentionUntil = later.CapturedAt.Add(30 * 24 * time.Hour)
	later.ResponseStatus = 203
	later.RequestedURL = "https://feed.example.test/later.xml"
	later.FinalURL = later.RequestedURL
	later.ResponseHeaders = responseHeaders(t, later.MIMEType, `"later"`)
	recaptured, err := repository.Reserve(ctx, later)
	if err != nil {
		t.Fatalf("recapture Reserve(): %v", err)
	}
	if !samePersistenceDTO(returned[0], recaptured) {
		t.Fatalf("recapture overwrote first capture/retention facts: before=%#v after=%#v", returned[0], recaptured)
	}

	var headersJSON string
	if err := runtime.SQL.QueryRow(`SELECT response_headers::text FROM evidence_snapshots WHERE id=$1`, recaptured.ID).Scan(&headersJSON); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(headersJSON, "Content-Type") || strings.Contains(headersJSON, "ETag") ||
		!strings.Contains(headersJSON, "content-type") || !strings.Contains(headersJSON, "etag") {
		t.Fatalf("response header mapper did not persist lowercase JSONB keys: %s", headersJSON)
	}
	if _, found := recaptured.ResponseHeaders.Values()["Content-Type"]; !found {
		t.Fatalf("response header mapper did not restore the allowlisted DTO: %#v", recaptured.ResponseHeaders.Values())
	}
	var redirectChainJSON string
	if err := runtime.SQL.QueryRow(`SELECT redirect_chain::text FROM evidence_snapshots WHERE id=$1`, recaptured.ID).Scan(&redirectChainJSON); err != nil {
		t.Fatal(err)
	}
	if redirectChainJSON != "[]" {
		t.Fatalf("empty redirect chain persisted as %s, want JSON array", redirectChainJSON)
	}
}

func TestEvidenceSnapshotRepositoryCommitIsAtomicIdempotentAndManyToMany(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	repository := newEvidenceSnapshotRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "commit")
	persisted, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	selectedDigest := digestValue("normalized selected entry")
	first := evidenceObservation(fixture, "entry-one", "First", selectedDigest)
	second := evidenceObservation(fixture, "entry-two", "Second", selectedDigest)
	commit := sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: persisted.ID, StoreResult: storeResult(persisted), Observations: []sourceapplication.SourceObservationDTO{first, second},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	}
	committed, err := repository.Commit(ctx, commit)
	if err != nil {
		t.Fatalf("Commit(): %v", err)
	}
	if committed.Snapshot.LifecycleState != string(domain.EvidenceLifecycleAvailable) {
		t.Fatalf("committed lifecycle = %q", committed.Snapshot.LifecycleState)
	}
	assertEvidenceFactCounts(t, runtime.SQL, fixture.SourceID, 2, 2)
	var legacyBindings int
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM source_observations
WHERE source_connection_id=$1 AND collection_run_item_id IS NOT NULL`, fixture.SourceID).Scan(&legacyBindings); err != nil {
		t.Fatal(err)
	}
	if legacyBindings != 0 {
		t.Fatalf("raw evidence commit unexpectedly required %d legacy collection item bindings", legacyBindings)
	}
	if _, err := repository.Commit(ctx, commit); err != nil {
		t.Fatalf("idempotent Commit(): %v", err)
	}
	assertEvidenceFactCounts(t, runtime.SQL, fixture.SourceID, 2, 2)

	subset := commit
	subset.Observations = append([]sourceapplication.SourceObservationDTO(nil), commit.Observations[:1]...)
	if _, err := repository.Commit(ctx, subset); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("subset Commit() error = %v, want immutable commit-set conflict", err)
	}
	duplicate := commit
	duplicate.Observations = []sourceapplication.SourceObservationDTO{first, first}
	if _, err := repository.Commit(ctx, duplicate); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("duplicate-fact Commit() error = %v, want immutable commit-set conflict", err)
	}
	additional := commit
	additional.Observations = append(append([]sourceapplication.SourceObservationDTO(nil), commit.Observations...),
		evidenceObservation(fixture, "entry-three", "Third", selectedDigest))
	if _, err := repository.Commit(ctx, additional); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("superset Commit() error = %v, want immutable commit-set conflict", err)
	}
	assertEvidenceFactCounts(t, runtime.SQL, fixture.SourceID, 2, 2)

	rawMarker := "raw-body-must-not-enter-evidence-database"
	conflict := commit
	conflict.Observations = append([]sourceapplication.SourceObservationDTO(nil), commit.Observations...)
	conflict.Observations[0].Title = rawMarker
	if _, err := repository.Commit(ctx, conflict); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("different immutable observation Commit() error = %v", err)
	} else if strings.Contains(err.Error(), rawMarker) {
		t.Fatalf("repository error leaked caller content: %q", err)
	}
	assertEvidenceFactCounts(t, runtime.SQL, fixture.SourceID, 2, 2)

	secondFixture := addEvidenceIdentity(t, runtime.SQL, fixture, "second-raw-response")
	secondPersisted, err := repository.Reserve(ctx, secondFixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	linkedAgain := first
	linkedAgain.Evidence.EvidenceKey = secondPersisted.EvidenceKey
	linkedAgain.Evidence.SelectedPayloadSHA256 = digestValue("second evidence selected bytes")
	if _, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: secondPersisted.ID, StoreResult: storeResult(secondPersisted),
		Observations:                  []sourceapplication.SourceObservationDTO{linkedAgain},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("M:N Commit(): %v", err)
	}
	if _, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: secondPersisted.ID, StoreResult: storeResult(secondPersisted),
		Observations:                  []sourceapplication.SourceObservationDTO{linkedAgain},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("idempotent M:N Commit(): %v", err)
	}
	var observationLinks int
	if err := runtime.SQL.QueryRow(`
SELECT count(*)
FROM source_observation_evidences AS link
JOIN source_observations AS observation ON observation.id=link.source_observation_id
WHERE observation.source_connection_id=$1 AND observation.external_id='entry-one'`, fixture.SourceID).Scan(&observationLinks); err != nil {
		t.Fatal(err)
	}
	if observationLinks != 2 {
		t.Fatalf("one observation linked to %d evidence snapshots, want 2", observationLinks)
	}
	var redundantObservationHashColumns int
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM information_schema.columns
WHERE table_schema='public' AND table_name='source_observations'
  AND column_name='selected_payload_sha256'`).Scan(&redundantObservationHashColumns); err != nil {
		t.Fatal(err)
	}
	if redundantObservationHashColumns != 0 {
		t.Fatal("source observation retained an evidence-specific selected payload hash column")
	}
	var locatorCount, distinctLocatorHashes int
	if err := runtime.SQL.QueryRow(`
SELECT count(*),count(DISTINCT link.selected_payload_sha256)
FROM source_observation_evidences AS link
JOIN source_observations AS observation ON observation.id=link.source_observation_id
WHERE observation.source_connection_id=$1 AND observation.external_id='entry-one'`, fixture.SourceID).
		Scan(&locatorCount, &distinctLocatorHashes); err != nil {
		t.Fatal(err)
	}
	if locatorCount != 2 || distinctLocatorHashes != 2 {
		t.Fatalf("locator hashes = rows:%d distinct:%d, want 2/2", locatorCount, distinctLocatorHashes)
	}
	var leaked bool
	if err := runtime.SQL.QueryRow(`
SELECT EXISTS (
  SELECT 1 FROM source_observations WHERE to_jsonb(source_observations)::text LIKE '%' || $1 || '%'
  UNION ALL
  SELECT 1 FROM evidence_snapshots WHERE to_jsonb(evidence_snapshots)::text LIKE '%' || $1 || '%'
)`, rawMarker).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked {
		t.Fatal("raw body marker entered evidence persistence tables")
	}
}

func TestEvidenceSnapshotRepositoryCommitConservativelyBindsExistingLegacyItem(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	repository := newEvidenceSnapshotRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "legacy-binding")
	insertEvidenceCollectionItem(t, runtime.SQL, fixture.RunID, fixture.SourceID, "bound-entry")
	persisted, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	observation := evidenceObservation(fixture, "bound-entry", "Bound", digestValue("bound selected bytes"))
	if _, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: persisted.ID, StoreResult: storeResult(persisted),
		Observations:                  []sourceapplication.SourceObservationDTO{observation},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Commit(): %v", err)
	}
	var bound bool
	if err := runtime.SQL.QueryRow(`
SELECT collection_run_item_id IS NOT NULL
FROM source_observations
WHERE source_connection_id=$1 AND external_id=$2`, fixture.SourceID, observation.ExternalID).Scan(&bound); err != nil {
		t.Fatal(err)
	}
	if !bound {
		t.Fatal("existing captured legacy collection item was not conservatively linked")
	}
}

func TestEvidenceSnapshotRepositoryCommitRechecksRevokedRights(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	repository := newEvidenceSnapshotRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "rights-revoked")
	insertEvidenceCollectionItem(t, runtime.SQL, fixture.RunID, fixture.SourceID, "revoked-entry")
	persisted, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	denyPolicy := insertEvidencePolicy(t, runtime.SQL, fixture.SourceID, fixture.PolicySubject, 2, 300, "revoked store raw")
	deny := evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: denyPolicy.ID, PolicyRevision: 2,
		PolicySubject: fixture.PolicySubject, Priority: 300, Basis: denyPolicy.Basis,
		SubjectKey: fixture.Reservation.EvidenceKey, InputDigest: fixture.Reservation.PayloadSHA256,
		Action: "store_raw", Decision: "deny", EffectiveFrom: time.Now().UTC().Add(-time.Minute),
		SupersedesDecisionID: &fixture.StoreDecisionID,
	}
	if _, err := insertEvidenceDecision(runtime.SQL, deny); err != nil {
		t.Fatalf("revoke store_raw: %v", err)
	}
	observation := evidenceObservation(fixture, "revoked-entry", "Revoked", digestValue("revoked selected bytes"))
	_, err = repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: persisted.ID, StoreResult: storeResult(persisted), Observations: []sourceapplication.SourceObservationDTO{observation},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	})
	if !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("Commit() after revocation error = %v, want trigger constraint", err)
	}
	var lifecycle string
	var observationCount int
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state FROM evidence_snapshots WHERE id=$1`, persisted.ID).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_observations WHERE source_connection_id=$1`, fixture.SourceID).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if lifecycle != string(domain.EvidenceLifecyclePending) || observationCount != 0 {
		t.Fatalf("revoked Commit() was not atomic: lifecycle=%q observations=%d", lifecycle, observationCount)
	}
	if err := repository.MarkFailed(ctx, persisted.ID, "RIGHTS_REVOKED"); err != nil {
		t.Fatalf("MarkFailed() after revoked commit: %v", err)
	}
}

func TestEvidenceSnapshotRepositoryFailureRetryUsesCASAndProtectsAvailable(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	repository := newEvidenceSnapshotRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "failure-retry")
	persisted, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkFailed(ctx, persisted.ID, "OBJECT_STORE_FAILED"); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkFailed(ctx, persisted.ID, "OBJECT_STORE_FAILED"); err != nil {
		t.Fatalf("idempotent MarkFailed(): %v", err)
	}
	if err := repository.MarkFailed(ctx, persisted.ID, "DIFFERENT_FAILURE"); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("different MarkFailed() error = %v", err)
	}
	retried, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatalf("retry Reserve(): %v", err)
	}
	if retried.ID != persisted.ID || retried.LifecycleState != string(domain.EvidenceLifecyclePending) {
		t.Fatalf("retry reservation = %#v", retried)
	}
	available, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: retried.ID, StoreResult: storeResult(retried), DocumentGenerationScheduledAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Commit() retry: %v", err)
	}
	if available.Snapshot.LifecycleState != string(domain.EvidenceLifecycleAvailable) {
		t.Fatalf("retry lifecycle = %q", available.Snapshot.LifecycleState)
	}
	if err := repository.MarkFailed(ctx, available.Snapshot.ID, "LATE_FAILURE"); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("MarkFailed() overwrote available snapshot: %v", err)
	}
	var lifecycle string
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state FROM evidence_snapshots WHERE id=$1`, available.Snapshot.ID).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if lifecycle != string(domain.EvidenceLifecycleAvailable) {
		t.Fatalf("available snapshot changed to %q", lifecycle)
	}
}

type evidenceRepositoryFixture struct {
	SourceID         int64
	RunID            int64
	PolicySubject    string
	StoreDecisionID  int64
	RetainDecisionID int64
	Reservation      sourceapplication.ReserveEvidenceSnapshotCommand
}

type evidenceDocumentGenerationSchedulerFake struct{}

func (evidenceDocumentGenerationSchedulerFake) Schedule(ctx context.Context, command sourceapplication.ScheduleSourceDocumentGenerationCommand) (sourceapplication.ScheduleSourceDocumentGenerationResult, error) {
	if _, inTransaction := database.TransactionFromContext(ctx); !inTransaction {
		return sourceapplication.ScheduleSourceDocumentGenerationResult{}, errors.New("document generation scheduler escaped evidence transaction")
	}
	if err := command.Validate(); err != nil {
		return sourceapplication.ScheduleSourceDocumentGenerationResult{}, err
	}
	receipts := make([]sourceapplication.SourceDocumentGenerationScheduleReceiptDTO, len(command.EvidenceReferences))
	for index, reference := range command.EvidenceReferences {
		receipts[index] = sourceapplication.SourceDocumentGenerationScheduleReceiptDTO{
			EvidenceReferenceID: reference.EvidenceReferenceID,
			JobID:               900000 + reference.EvidenceReferenceID,
			Created:             true,
		}
	}
	return sourceapplication.ScheduleSourceDocumentGenerationResult{Receipts: receipts}, nil
}

func newEvidenceSnapshotRepository(t *testing.T, runtime *database.Runtime) *sourcepostgres.EvidenceSnapshotRepository {
	t.Helper()
	repository, err := sourcepostgres.NewEvidenceSnapshotRepository(runtime, evidenceDocumentGenerationSchedulerFake{})
	if err != nil {
		t.Fatalf("NewEvidenceSnapshotRepository(): %v", err)
	}
	return repository
}

type evidencePolicyFixture struct {
	ID    int64
	Basis string
}

type evidenceDecisionFixture struct {
	SourceID             int64
	PolicyID             int64
	PolicyRevision       int64
	PolicySubject        string
	Priority             int
	Basis                string
	SubjectKey           string
	InputDigest          string
	Action               string
	Decision             string
	RetentionDays        *int
	EffectiveFrom        time.Time
	SupersedesDecisionID *int64
}

func newEvidenceRepositoryFixture(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
}, label string) evidenceRepositoryFixture {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%s-%d", label, time.Now().UnixNano())
	var sourceID int64
	if err := runtime.QueryRow(`
INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss',$1,$2) RETURNING id`, "evidence-repository-"+suffix, "https://feed.example.test/"+suffix).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	runID := insertEvidenceCollectionRun(t, runtime, sourceID, suffix)
	reservation := evidenceReservation(t, sourceID, runID, "raw-response-"+suffix, now)
	policySubject := "source-endpoint-" + suffix
	policy := insertEvidencePolicy(t, runtime, sourceID, policySubject, 1, 300, "raw archive fixture "+suffix)
	storeDecisionID, err := insertEvidenceDecision(runtime, evidenceDecisionFixture{
		SourceID: sourceID, PolicyID: policy.ID, PolicyRevision: 1, PolicySubject: policySubject,
		Priority: 300, Basis: policy.Basis, SubjectKey: reservation.EvidenceKey, InputDigest: reservation.PayloadSHA256,
		Action: "store_raw", Decision: "allow", EffectiveFrom: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("insert store decision: %v", err)
	}
	retentionDays := 30
	retainDecisionID, err := insertEvidenceDecision(runtime, evidenceDecisionFixture{
		SourceID: sourceID, PolicyID: policy.ID, PolicyRevision: 1, PolicySubject: policySubject,
		Priority: 300, Basis: policy.Basis, SubjectKey: reservation.EvidenceKey, InputDigest: reservation.PayloadSHA256,
		Action: "retain", Decision: "allow", RetentionDays: &retentionDays, EffectiveFrom: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("insert retain decision: %v", err)
	}
	reservation.StoreRawRightsDecisionID = storeDecisionID
	reservation.RetainRightsDecisionID = retainDecisionID
	return evidenceRepositoryFixture{
		SourceID: sourceID, RunID: runID, PolicySubject: policySubject,
		StoreDecisionID: storeDecisionID, RetainDecisionID: retainDecisionID, Reservation: reservation,
	}
}

func addEvidenceIdentity(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
}, fixture evidenceRepositoryFixture, payload string) evidenceRepositoryFixture {
	t.Helper()
	result := fixture
	result.Reservation = evidenceReservation(t, fixture.SourceID, fixture.RunID, payload, fixture.Reservation.CapturedAt)
	policy := insertEvidencePolicy(t, runtime, fixture.SourceID, fixture.PolicySubject, 3, 300, "second evidence identity")
	storeDecisionID, err := insertEvidenceDecision(runtime, evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: policy.ID, PolicyRevision: 3, PolicySubject: fixture.PolicySubject,
		Priority: 300, Basis: policy.Basis, SubjectKey: result.Reservation.EvidenceKey, InputDigest: result.Reservation.PayloadSHA256,
		Action: "store_raw", Decision: "allow", EffectiveFrom: time.Now().UTC().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	days := 30
	retainDecisionID, err := insertEvidenceDecision(runtime, evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: policy.ID, PolicyRevision: 3, PolicySubject: fixture.PolicySubject,
		Priority: 300, Basis: policy.Basis, SubjectKey: result.Reservation.EvidenceKey, InputDigest: result.Reservation.PayloadSHA256,
		Action: "retain", Decision: "allow", RetentionDays: &days, EffectiveFrom: time.Now().UTC().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	result.StoreDecisionID, result.RetainDecisionID = storeDecisionID, retainDecisionID
	result.Reservation.StoreRawRightsDecisionID, result.Reservation.RetainRightsDecisionID = storeDecisionID, retainDecisionID
	return result
}

func evidenceReservation(t *testing.T, sourceID, runID int64, payload string, capturedAt time.Time) sourceapplication.ReserveEvidenceSnapshotCommand {
	t.Helper()
	payloadDigest := digestValue(payload)
	profile, err := domain.NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		t.Fatal(err)
	}
	evidenceKey, err := domain.EvidenceSnapshotIdentity(payloadDigest, profile)
	if err != nil {
		t.Fatal(err)
	}
	mimeType := "application/atom+xml; charset=utf-8"
	return sourceapplication.ReserveEvidenceSnapshotCommand{
		SourceConnectionID: sourceID, CollectionRunID: runID, EvidenceKey: evidenceKey,
		ObjectKey: sourceapplication.RawEvidenceObjectKey(sourceID, evidenceKey), PayloadSHA256: payloadDigest,
		CollectorProfileVersion: profile.String(), MIMEType: mimeType, SizeBytes: int64(len(payload)), ResponseStatus: 200,
		RequestedURL: "https://feed.example.test/archive.xml", FinalURL: "https://feed.example.test/archive.xml",
		ResponseHeaders: responseHeaders(t, mimeType, `"archive"`), CapturedAt: capturedAt,
		RetentionUntil: capturedAt.Add(30 * 24 * time.Hour),
	}
}

func responseHeaders(t *testing.T, mimeType, etag string) sourceapplication.RawResponseHeadersDTO {
	t.Helper()
	headers, err := sourceapplication.NewRawResponseHeadersDTO(map[string][]string{"Content-Type": {mimeType}, "ETag": {etag}})
	if err != nil {
		t.Fatal(err)
	}
	return headers
}

func insertEvidenceCollectionRun(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
}, sourceID int64, label string) int64 {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var runID int64
	if err := runtime.QueryRow(`
INSERT INTO collection_runs (
  source_connection_id,query_signature,window_start,window_end,trigger_type,scheduled_at
) VALUES ($1,$2,$3,$4,'manual',$3) RETURNING id`, sourceID, digestValue(label+fmt.Sprint(now.UnixNano())), now.Add(-time.Hour), now).Scan(&runID); err != nil {
		t.Fatalf("insert collection run: %v", err)
	}
	return runID
}

func insertEvidenceCollectionItem(t *testing.T, runtime interface {
	Exec(string, ...any) (sql.Result, error)
}, runID, sourceID int64, externalID string) {
	t.Helper()
	_, err := runtime.Exec(`
INSERT INTO collection_run_items (
  run_id,source_connection_id,source_code,external_id,content_type,captured_item_version,
  captured_item,payload_hash,raw_payload_disposition,outcome,observed_at
) VALUES ($1,$2,'rss',$3,'article','v2','{}'::jsonb,$4,'discarded','captured',$5)`,
		runID, sourceID, externalID, digestValue("collection-item-"+externalID), time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("insert collection item: %v", err)
	}
}

func insertEvidencePolicy(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
}, sourceID int64, subject string, revision int64, priority int, basis string) evidencePolicyFixture {
	t.Helper()
	policyHash := digestValue(fmt.Sprintf("policy-%d-%s-%d", sourceID, subject, revision))
	actorID := insertRightsFixtureActor(t, runtime, policyHash)
	idempotencyKey, commandFingerprint := rightsFixtureReceipt("policy", policyHash)
	var policyID int64
	if err := runtime.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
  scope_type,scope_subject,policy_revision,priority,basis_summary,policy_hash,effective_at
) VALUES ($1,$1,$2,$3,$4,'source_endpoint',$5,$6,$7,$8,$9,$10) RETURNING id`,
		actorID, idempotencyKey, commandFingerprint, sourceID, subject, revision, priority, basis,
		policyHash, time.Now().UTC().Add(-2*time.Hour)).Scan(&policyID); err != nil {
		t.Fatalf("insert evidence policy: %v", err)
	}
	return evidencePolicyFixture{ID: policyID, Basis: basis}
}

func insertEvidenceDecision(runtime interface {
	QueryRow(string, ...any) *sql.Row
}, fixture evidenceDecisionFixture) (int64, error) {
	retentionDays := ""
	if fixture.RetentionDays != nil {
		retentionDays = fmt.Sprint(*fixture.RetentionDays)
	}
	supersedesDecisionID := ""
	if fixture.SupersedesDecisionID != nil {
		supersedesDecisionID = fmt.Sprint(*fixture.SupersedesDecisionID)
	}
	idempotencyKey, commandFingerprint := rightsFixtureReceipt(
		"decision", fixture.SourceID, fixture.PolicyID, fixture.PolicyRevision, fixture.SubjectKey,
		fixture.InputDigest, fixture.Action, fixture.Decision, fixture.EffectiveFrom.UTC().Format(time.RFC3339Nano),
		retentionDays, supersedesDecisionID,
	)
	var decisionID int64
	err := runtime.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  )
  SELECT $1,$2,policy.version,'raw_response',$7,$8,policy.recorded_by_user_id,$14,$15,1
  FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,
  evaluator,evaluated_at,effective_from,retention_days,supersedes_decision_id
) SELECT decision_batch.id,$1,$2,$3,'source_endpoint',$4,$5,$6,'raw_response',$7,$8,$9,$10,
  'repository-fixture',CURRENT_TIMESTAMP,$11,$12,$13 FROM decision_batch RETURNING id`,
		fixture.SourceID, fixture.PolicyID, fixture.PolicyRevision, fixture.PolicySubject, fixture.Priority, fixture.Basis,
		fixture.SubjectKey, fixture.InputDigest, fixture.Action, fixture.Decision, fixture.EffectiveFrom.UTC(),
		fixture.RetentionDays, fixture.SupersedesDecisionID, idempotencyKey, commandFingerprint).Scan(&decisionID)
	return decisionID, err
}

func evidenceObservation(fixture evidenceRepositoryFixture, externalID, title, selectedDigest string) sourceapplication.SourceObservationDTO {
	start, end := int64(0), int64(16)
	publishedAt := fixture.Reservation.CapturedAt.Add(-time.Hour).Add(321 * time.Nanosecond)
	return sourceapplication.SourceObservationDTO{
		SourceConnectionID: fixture.SourceID, CollectionRunID: fixture.RunID,
		ExternalID: externalID, UpstreamIdentity: digestValue("observation-" + externalID), SourceCode: "rss", ContentType: "article",
		Title: title, Language: "en", Author: "Publisher", SourceRecordURL: fixture.Reservation.FinalURL,
		CanonicalURL: "https://publisher.example.test/" + externalID, BodyOrigin: "feed_content", Completeness: "full",
		PublishedAt: &publishedAt, DiscoveredAt: fixture.Reservation.CapturedAt, CapturedAt: fixture.Reservation.CapturedAt,
		Evidence: sourceapplication.RawEvidenceReferenceDTO{
			EvidenceKey: fixture.Reservation.EvidenceKey, LocatorType: string(domain.EvidenceLocatorByteRange),
			LocatorValue: "bytes[0:16]", ByteStart: &start, ByteEnd: &end,
			SelectedPayloadSHA256: selectedDigest, SelectorVersion: domain.ByteRangeSelectorVersion,
		},
	}
}

func storeResult(snapshot sourceapplication.PersistedEvidenceSnapshotDTO) sourceapplication.StoreRawEvidenceResult {
	return sourceapplication.StoreRawEvidenceResult{
		SourceConnectionID: snapshot.SourceConnectionID, EvidenceKey: snapshot.EvidenceKey, ObjectKey: snapshot.ObjectKey,
		PayloadSHA256: snapshot.PayloadSHA256, CollectorProfileVersion: snapshot.CollectorProfileVersion,
		MIMEType: snapshot.MIMEType, SizeBytes: snapshot.SizeBytes,
	}
}

func persistenceMatchesReservation(stored sourceapplication.PersistedEvidenceSnapshotDTO, command sourceapplication.ReserveEvidenceSnapshotCommand) bool {
	return stored.SourceConnectionID == command.SourceConnectionID && stored.CollectionRunID == command.CollectionRunID &&
		stored.StoreRawRightsDecisionID == command.StoreRawRightsDecisionID && stored.RetainRightsDecisionID == command.RetainRightsDecisionID &&
		stored.EvidenceKey == command.EvidenceKey && stored.ObjectKey == command.ObjectKey && stored.PayloadSHA256 == command.PayloadSHA256 &&
		stored.CollectorProfileVersion == command.CollectorProfileVersion && stored.MIMEType == command.MIMEType && stored.SizeBytes == command.SizeBytes &&
		stored.ResponseStatus == command.ResponseStatus && stored.RequestedURL == command.RequestedURL && stored.FinalURL == command.FinalURL &&
		stored.ResponseHeaders.Equal(command.ResponseHeaders) && stored.CapturedAt.Equal(command.CapturedAt) && stored.RetentionUntil.Equal(command.RetentionUntil)
}

func samePersistenceDTO(left, right sourceapplication.PersistedEvidenceSnapshotDTO) bool {
	return left.ID == right.ID && left.LifecycleState == right.LifecycleState &&
		persistenceMatchesReservation(left, sourceapplication.ReserveEvidenceSnapshotCommand{
			SourceConnectionID: right.SourceConnectionID, CollectionRunID: right.CollectionRunID,
			StoreRawRightsDecisionID: right.StoreRawRightsDecisionID, RetainRightsDecisionID: right.RetainRightsDecisionID,
			EvidenceKey: right.EvidenceKey, ObjectKey: right.ObjectKey, PayloadSHA256: right.PayloadSHA256,
			CollectorProfileVersion: right.CollectorProfileVersion, MIMEType: right.MIMEType, SizeBytes: right.SizeBytes,
			ResponseStatus: right.ResponseStatus, RequestedURL: right.RequestedURL, FinalURL: right.FinalURL,
			RedirectChain: right.RedirectChain, ResponseHeaders: right.ResponseHeaders,
			CapturedAt: right.CapturedAt, RetentionUntil: right.RetentionUntil,
		})
}

func assertEvidenceFactCounts(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
}, sourceID int64, observations, locators int) {
	t.Helper()
	var observationCount, locatorCount int
	if err := runtime.QueryRow(`SELECT count(*) FROM source_observations WHERE source_connection_id=$1`, sourceID).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if err := runtime.QueryRow(`SELECT count(*) FROM source_observation_evidences WHERE source_connection_id=$1`, sourceID).Scan(&locatorCount); err != nil {
		t.Fatal(err)
	}
	if observationCount != observations || locatorCount != locators {
		t.Fatalf("evidence fact counts = observations:%d locators:%d, want %d/%d", observationCount, locatorCount, observations, locators)
	}
}

func digestValue(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
