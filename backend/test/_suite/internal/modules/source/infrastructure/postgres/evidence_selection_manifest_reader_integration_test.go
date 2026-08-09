package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestEvidenceSelectionManifestReaderReadsExactManyToManyFactsAndCurrentRights(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()

	var port sourceapplication.EvidenceSelectionManifestReader = sourcepostgres.NewEvidenceSelectionManifestReader(runtime)
	repository := newEvidenceSnapshotRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "selection-reader")
	firstSnapshot, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatalf("Reserve(first): %v", err)
	}
	firstObservation := evidenceObservation(fixture, "selection-one", "First selection", digestValue("first selected bytes"))
	secondObservation := evidenceObservation(fixture, "selection-two", "Second selection", digestValue("second selected bytes"))
	if _, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: firstSnapshot.ID, StoreResult: storeResult(firstSnapshot),
		Observations:                  []sourceapplication.SourceObservationDTO{firstObservation, secondObservation},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Commit(first): %v", err)
	}

	firstReferenceID := evidenceReferenceID(t, runtime.SQL, fixture.SourceID, firstSnapshot.ID, firstObservation.ExternalID)
	secondReferenceID := evidenceReferenceID(t, runtime.SQL, fixture.SourceID, firstSnapshot.ID, secondObservation.ExternalID)
	firstManifest, err := port.ReadEvidenceSelectionManifest(ctx, firstReferenceID)
	if err != nil {
		t.Fatalf("ReadEvidenceSelectionManifest(first): %v", err)
	}
	secondManifest, err := port.ReadEvidenceSelectionManifest(ctx, secondReferenceID)
	if err != nil {
		t.Fatalf("ReadEvidenceSelectionManifest(second): %v", err)
	}
	if firstManifest.EvidenceReferenceID != firstReferenceID || secondManifest.EvidenceReferenceID != secondReferenceID ||
		firstManifest.SourceObservationID == secondManifest.SourceObservationID ||
		firstManifest.EvidenceSnapshotID != firstSnapshot.ID || secondManifest.EvidenceSnapshotID != firstSnapshot.ID {
		t.Fatalf("one snapshot/two observations mapping is not exact: first=%#v second=%#v", firstManifest, secondManifest)
	}
	if firstManifest.EvidenceReference.SelectedPayloadSHA256 == secondManifest.EvidenceReference.SelectedPayloadSHA256 ||
		firstManifest.ExternalID != firstObservation.ExternalID || secondManifest.ExternalID != secondObservation.ExternalID {
		t.Fatalf("locator-specific facts collapsed: first=%#v second=%#v", firstManifest.EvidenceReference, secondManifest.EvidenceReference)
	}
	assertReadableEvidenceSelectionManifest(t, firstManifest, fixture, firstSnapshot)

	secondFixture := addEvidenceIdentity(t, runtime.SQL, fixture, "selection-reader-second-raw-response")
	secondSnapshot, err := repository.Reserve(ctx, secondFixture.Reservation)
	if err != nil {
		t.Fatalf("Reserve(second): %v", err)
	}
	linkedAgain := firstObservation
	linkedAgain.Evidence.EvidenceKey = secondSnapshot.EvidenceKey
	linkedAgain.Evidence.SelectedPayloadSHA256 = digestValue("same observation second snapshot bytes")
	if _, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: secondSnapshot.ID, StoreResult: storeResult(secondSnapshot),
		Observations:                  []sourceapplication.SourceObservationDTO{linkedAgain},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Commit(second): %v", err)
	}
	thirdReferenceID := evidenceReferenceID(t, runtime.SQL, fixture.SourceID, secondSnapshot.ID, linkedAgain.ExternalID)
	thirdManifest, err := port.ReadEvidenceSelectionManifest(ctx, thirdReferenceID)
	if err != nil {
		t.Fatalf("ReadEvidenceSelectionManifest(third): %v", err)
	}
	if thirdManifest.SourceObservationID != firstManifest.SourceObservationID ||
		thirdManifest.EvidenceSnapshotID == firstManifest.EvidenceSnapshotID ||
		thirdManifest.EvidenceKey == firstManifest.EvidenceKey {
		t.Fatalf("one observation/two snapshots mapping collapsed: first=%#v third=%#v", firstManifest, thirdManifest)
	}

	policy := insertEvidencePolicy(t, runtime.SQL, fixture.SourceID, fixture.PolicySubject, 2, 300, "selection read revoked")
	if _, err := insertEvidenceDecision(runtime.SQL, evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: policy.ID, PolicyRevision: 2, PolicySubject: fixture.PolicySubject,
		Priority: 300, Basis: policy.Basis, SubjectKey: firstSnapshot.EvidenceKey, InputDigest: firstSnapshot.PayloadSHA256,
		Action: "store_raw", Decision: "deny", EffectiveFrom: time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("insert current store_raw deny: %v", err)
	}
	shortRetentionDays := 7
	if _, err := insertEvidenceDecision(runtime.SQL, evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: policy.ID, PolicyRevision: 2, PolicySubject: fixture.PolicySubject,
		Priority: 300, Basis: policy.Basis, SubjectKey: firstSnapshot.EvidenceKey, InputDigest: firstSnapshot.PayloadSHA256,
		Action: "retain", Decision: "allow", RetentionDays: &shortRetentionDays, EffectiveFrom: time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("insert current shorter retain allow: %v", err)
	}
	revoked, err := port.ReadEvidenceSelectionManifest(ctx, firstReferenceID)
	if err != nil {
		t.Fatalf("ReadEvidenceSelectionManifest(revoked): %v", err)
	}
	if revoked.StoreRawAllowed || !revoked.RetainAllowed || revoked.CurrentRetentionDays == nil || *revoked.CurrentRetentionDays != shortRetentionDays {
		t.Fatalf("current rights were not evaluated from one exact manifest read: %#v", revoked)
	}
	if !revoked.RetentionUntil.Equal(firstSnapshot.RetentionUntil) {
		t.Fatalf("immutable retention receipt drifted: got %s want %s", revoked.RetentionUntil, firstSnapshot.RetentionUntil)
	}
}

func TestEvidenceSelectionManifestReaderRejectsInvalidAndMissingReferences(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	reader := sourcepostgres.NewEvidenceSelectionManifestReader(runtime)

	if _, err := reader.ReadEvidenceSelectionManifest(context.Background(), 0); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("invalid reference error = %v", err)
	}
	if _, err := reader.ReadEvidenceSelectionManifest(context.Background(), 9223372036854775807); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("missing reference error = %v", err)
	}
}

func evidenceReferenceID(t *testing.T, query interface {
	QueryRow(string, ...any) *sql.Row
}, sourceID, snapshotID int64, externalID string) int64 {
	t.Helper()
	var referenceID int64
	if err := query.QueryRow(`
SELECT reference.id
FROM source_observation_evidences AS reference
JOIN source_observations AS observation ON observation.id=reference.source_observation_id
WHERE reference.source_connection_id=$1 AND reference.evidence_snapshot_id=$2 AND observation.external_id=$3`,
		sourceID, snapshotID, externalID).Scan(&referenceID); err != nil {
		t.Fatalf("read evidence reference id: %v", err)
	}
	return referenceID
}

func assertReadableEvidenceSelectionManifest(t *testing.T, manifest sourceapplication.EvidenceSelectionManifestDTO, fixture evidenceRepositoryFixture, snapshot sourceapplication.PersistedEvidenceSnapshotDTO) {
	t.Helper()
	if manifest.SourceConnectionID != fixture.SourceID || manifest.EvidenceKey != snapshot.EvidenceKey ||
		manifest.ObjectKey != snapshot.ObjectKey || manifest.PayloadSHA256 != snapshot.PayloadSHA256 ||
		manifest.CollectorProfileVersion != snapshot.CollectorProfileVersion || manifest.MIMEType != snapshot.MIMEType ||
		manifest.SizeBytes != snapshot.SizeBytes || manifest.ResponseStatus != snapshot.ResponseStatus ||
		!manifest.CapturedAt.Equal(snapshot.CapturedAt) || !manifest.RetentionUntil.Equal(snapshot.RetentionUntil) {
		t.Fatalf("snapshot identity/retention facts were not mapped exactly: %#v", manifest)
	}
	if !manifest.StoreRawAllowed || !manifest.RetainAllowed || manifest.CurrentRetentionDays == nil || *manifest.CurrentRetentionDays != 30 || manifest.RightsEvaluatedAt.IsZero() {
		t.Fatalf("current rights projection is not readable: %#v", manifest)
	}
	if manifest.EvidenceReference.EvidenceKey != snapshot.EvidenceKey || manifest.EvidenceReference.LocatorValue == "" ||
		manifest.EvidenceReference.SelectedPayloadSHA256 == "" || manifest.EvidenceReference.SelectorVersion == "" {
		t.Fatalf("M:N locator was not mapped exactly: %#v", manifest.EvidenceReference)
	}
	if values := manifest.ResponseHeaders.Values()["Content-Type"]; len(values) != 1 || values[0] != snapshot.MIMEType {
		t.Fatalf("response headers = %#v", manifest.ResponseHeaders.Values())
	}
}
