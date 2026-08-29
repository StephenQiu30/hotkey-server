//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
)

func TestRawEvidenceRetentionRepositoryHonorsBoundaryExceptionAndDeletionAudit(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	snapshots := newEvidenceSnapshotRepository(t, runtime)
	retention := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)

	expiredFixture := newEvidenceRepositoryFixture(t, runtime.SQL, "retention-expired")
	expiredFixture = withEvidenceRetentionDays(t, runtime.SQL, expiredFixture, 10)
	expired := commitRetentionSnapshot(t, ctx, snapshots, expiredFixture)

	retainedFixture := newEvidenceRepositoryFixture(t, runtime.SQL, "retention-live")
	retained := commitRetentionSnapshot(t, ctx, snapshots, retainedFixture)

	exceptionFixture := newEvidenceRepositoryFixture(t, runtime.SQL, "retention-exception")
	exceptionFixture = withEvidenceRetentionDays(t, runtime.SQL, exceptionFixture, 10)
	exception := commitRetentionSnapshot(t, ctx, snapshots, exceptionFixture)
	var approverID int64
	if err := runtime.SQL.QueryRow(`SELECT recorded_by_user_id FROM source_rights_policies WHERE id=(SELECT policy_id FROM source_rights_decisions WHERE id=$1)`, exceptionFixture.RetainDecisionID).Scan(&approverID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO evidence_retention_exceptions (evidence_snapshot_id,approved_by_user_id,approval_basis,approved_at)
VALUES ($1,$2,'approved litigation preservation',CURRENT_TIMESTAMP)`, exception.ID, approverID); err != nil {
		t.Fatal(err)
	}

	at := expired.RetentionUntil.Add(24 * time.Hour)
	candidates, err := retention.ClaimExpired(ctx, at, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].SnapshotID != expired.ID || candidates[0].AttemptNo != 1 || candidates[0].RetentionPolicyID <= 0 || candidates[0].RetentionPolicyVersion <= 0 {
		t.Fatalf("unexpected candidates: %#v (live=%d exception=%d)", candidates, retained.ID, exception.ID)
	}
	if err := retention.CompleteDeletion(ctx, sourceapplication.CompleteRawEvidenceDeletionCommand{
		SnapshotID: expired.ID, AttemptNo: candidates[0].AttemptNo, ObjectKey: expired.ObjectKey,
		PayloadSHA256: expired.PayloadSHA256, DeletedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	var lifecycle, objectKey, payloadSHA string
	var retentionUntil time.Time
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state,object_key,btrim(payload_sha256),retention_until FROM evidence_snapshots WHERE id=$1`, expired.ID).Scan(&lifecycle, &objectKey, &payloadSHA, &retentionUntil); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "tombstoned" || objectKey != expired.ObjectKey || payloadSHA != expired.PayloadSHA256 || !retentionUntil.Equal(expired.RetentionUntil) {
		t.Fatalf("tombstone lost immutable metadata: lifecycle=%s object=%s hash=%s retention=%s", lifecycle, objectKey, payloadSHA, retentionUntil)
	}
	var claimed, succeeded int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FILTER (WHERE event_type='delete_claimed'),count(*) FILTER (WHERE event_type='delete_succeeded') FROM evidence_deletion_audits WHERE evidence_snapshot_id=$1`, expired.ID).Scan(&claimed, &succeeded); err != nil {
		t.Fatal(err)
	}
	if claimed != 1 || succeeded != 1 {
		t.Fatalf("deletion audit events = claimed:%d succeeded:%d", claimed, succeeded)
	}
}

func TestRawEvidenceRetentionRepositoryRetriesFailedDeletionWithNextAttempt(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	snapshots := newEvidenceSnapshotRepository(t, runtime)
	retention := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "retention-retry")
	fixture = withEvidenceRetentionDays(t, runtime.SQL, fixture, 10)
	snapshot := commitRetentionSnapshot(t, ctx, snapshots, fixture)
	at := snapshot.RetentionUntil.Add(time.Hour)

	first, err := retention.ClaimExpired(ctx, at, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim = %#v/%v", first, err)
	}
	if err := retention.FailDeletion(ctx, sourceapplication.FailRawEvidenceDeletionCommand{
		SnapshotID: snapshot.ID, AttemptNo: 1, ObjectKey: snapshot.ObjectKey,
		PayloadSHA256: snapshot.PayloadSHA256, FailureCode: sourceapplication.RawEvidenceDeleteObjectFailed, FailedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	second, err := retention.ClaimExpired(ctx, at.Add(time.Minute), 1)
	if err != nil || len(second) != 1 || second[0].AttemptNo != 2 || second[0].SnapshotID != snapshot.ID {
		t.Fatalf("retry claim = %#v/%v", second, err)
	}
}

func TestRawEvidenceRetentionRepositoryRevocationOverridesLiveRetentionAndApprovedException(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	snapshots := newEvidenceSnapshotRepository(t, runtime)
	retention := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "retention-rights-revoked")
	snapshot := commitRetentionSnapshot(t, ctx, snapshots, fixture)
	var approverID int64
	if err := runtime.SQL.QueryRow(`SELECT recorded_by_user_id FROM source_rights_policies WHERE id=(SELECT policy_id FROM source_rights_decisions WHERE id=$1)`, fixture.RetainDecisionID).Scan(&approverID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO evidence_retention_exceptions
(evidence_snapshot_id,approved_by_user_id,approval_basis,approved_at)
VALUES ($1,$2,'approved retention cannot override current rights denial',CURRENT_TIMESTAMP)`, snapshot.ID, approverID); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	denyPolicy := insertEvidencePolicy(t, runtime.SQL, fixture.SourceID, fixture.PolicySubject, 4, 300, "rights revocation fixture")
	if _, err := insertEvidenceDecision(runtime.SQL, evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: denyPolicy.ID, PolicyRevision: 4, PolicySubject: fixture.PolicySubject,
		Priority: 300, Basis: denyPolicy.Basis, SubjectKey: fixture.Reservation.EvidenceKey,
		InputDigest: fixture.Reservation.PayloadSHA256, Action: "store_raw", Decision: "deny",
		EffectiveFrom: at.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	candidates, err := retention.ClaimExpired(ctx, at, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].SnapshotID != snapshot.ID ||
		candidates[0].ReasonCode != sourceapplication.RawEvidenceDeleteRightsRevoked ||
		!snapshot.RetentionUntil.After(at) {
		t.Fatalf("revoked candidate = %#v, retention_until=%s at=%s", candidates, snapshot.RetentionUntil, at)
	}
	if err := retention.CompleteDeletion(ctx, sourceapplication.CompleteRawEvidenceDeletionCommand{
		SnapshotID: snapshot.ID, AttemptNo: candidates[0].AttemptNo, ObjectKey: snapshot.ObjectKey,
		PayloadSHA256: snapshot.PayloadSHA256, DeletedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	var lifecycle string
	var claimedReason, succeededReason string
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state FROM evidence_snapshots WHERE id=$1`, snapshot.ID).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT
max(reason_code) FILTER (WHERE event_type='delete_claimed'),
max(reason_code) FILTER (WHERE event_type='delete_succeeded')
FROM evidence_deletion_audits WHERE evidence_snapshot_id=$1`, snapshot.ID).Scan(&claimedReason, &succeededReason); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "tombstoned" || claimedReason != sourceapplication.RawEvidenceDeleteRightsRevoked || succeededReason != claimedReason {
		t.Fatalf("revoked deletion facts = lifecycle:%s claimed:%s succeeded:%s", lifecycle, claimedReason, succeededReason)
	}
}

func commitRetentionSnapshot(t *testing.T, ctx context.Context, repository *sourcepostgres.EvidenceSnapshotRepository, fixture evidenceRepositoryFixture) sourceapplication.PersistedEvidenceSnapshotDTO {
	t.Helper()
	reserved, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: reserved.ID, StoreResult: storeResult(reserved), Observations: []sourceapplication.SourceObservationDTO{},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return committed.Snapshot
}

func withEvidenceRetentionDays(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
}, fixture evidenceRepositoryFixture, days int) evidenceRepositoryFixture {
	t.Helper()
	policy := insertEvidencePolicy(t, runtime, fixture.SourceID, fixture.PolicySubject, 2, 300, "retention boundary fixture")
	decisionID, err := insertEvidenceDecision(runtime, evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: policy.ID, PolicyRevision: 2, PolicySubject: fixture.PolicySubject,
		Priority: 300, Basis: policy.Basis, SubjectKey: fixture.Reservation.EvidenceKey,
		InputDigest: fixture.Reservation.PayloadSHA256, Action: "retain", Decision: "allow",
		RetentionDays: &days, EffectiveFrom: time.Now().UTC().Add(-30 * time.Minute),
		SupersedesDecisionID: &fixture.RetainDecisionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.RetainDecisionID = decisionID
	fixture.Reservation.RetainRightsDecisionID = decisionID
	fixture.Reservation.RetentionUntil = fixture.Reservation.CapturedAt.Add(time.Duration(days) * 24 * time.Hour)
	return fixture
}
