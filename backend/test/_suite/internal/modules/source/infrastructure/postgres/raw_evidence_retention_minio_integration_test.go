//go:build integration

package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourceminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/minio"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestRawEvidenceRightsRevocationDeletesExactMinIOObjectAndKeepsImmutableLineage(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	store, client, bucket, cleanup := openRawEvidenceRevocationMinIO(t, ctx)
	defer cleanup()

	snapshots := newEvidenceSnapshotRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "retention-minio-rights-revoked")
	payload := fmt.Sprintf("approved synthetic revocation object %d", time.Now().UnixNano())
	fixture = addEvidenceIdentity(t, runtime.SQL, fixture, payload)
	reserved, err := snapshots.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.PutIfAbsent(ctx, sourceapplication.StoreRawEvidenceCommand{
		SourceConnectionID: reserved.SourceConnectionID, EvidenceKey: reserved.EvidenceKey,
		ObjectKey: reserved.ObjectKey, Payload: []byte(payload), PayloadSHA256: reserved.PayloadSHA256,
		CollectorProfileVersion: reserved.CollectorProfileVersion, MIMEType: reserved.MIMEType,
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := snapshots.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: reserved.ID, StoreResult: stored, Observations: []sourceapplication.SourceObservationDTO{},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := committed.Snapshot
	var approverID int64
	if err := runtime.SQL.QueryRow(`SELECT recorded_by_user_id FROM source_rights_policies WHERE id=(SELECT policy_id FROM source_rights_decisions WHERE id=$1)`, fixture.RetainDecisionID).Scan(&approverID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO evidence_retention_exceptions
(evidence_snapshot_id,approved_by_user_id,approval_basis,approved_at)
VALUES ($1,$2,'synthetic exception must not override rights revocation',CURRENT_TIMESTAMP)`, snapshot.ID, approverID); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	denyPolicy := insertEvidencePolicy(t, runtime.SQL, fixture.SourceID, fixture.PolicySubject, 4, 300, "raw object rights revoked")
	if _, err := insertEvidenceDecision(runtime.SQL, evidenceDecisionFixture{
		SourceID: fixture.SourceID, PolicyID: denyPolicy.ID, PolicyRevision: 4, PolicySubject: fixture.PolicySubject,
		Priority: 300, Basis: denyPolicy.Basis, SubjectKey: fixture.Reservation.EvidenceKey,
		InputDigest: fixture.Reservation.PayloadSHA256, Action: "store_raw", Decision: "deny",
		EffectiveFrom: at.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	service, err := sourceapplication.NewRawEvidenceRetentionService(sourceapplication.RawEvidenceRetentionDependencies{
		Repository: sourcepostgres.NewRawEvidenceRetentionRepository(runtime), Deleter: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(ctx, sourceapplication.RunRawEvidenceRetentionCommand{At: at, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Claimed != 1 || result.Deleted != 1 || result.Failed != 0 || !snapshot.RetentionUntil.After(at) {
		t.Fatalf("revocation deletion result = %#v, retention_until=%s at=%s", result, snapshot.RetentionUntil, at)
	}
	if _, err := client.StatObject(ctx, bucket, snapshot.ObjectKey, miniosdk.StatObjectOptions{}); err == nil || miniosdk.ToErrorResponse(err).StatusCode != 404 {
		t.Fatalf("revoked MinIO object remained or returned an unstable error: %v", err)
	}
	replayed, err := service.Run(ctx, sourceapplication.RunRawEvidenceRetentionCommand{At: at.Add(time.Minute), Limit: 10})
	if err != nil || replayed.Claimed != 0 || replayed.Deleted != 0 {
		t.Fatalf("idempotent revocation replay = %#v/%v", replayed, err)
	}
	var lifecycle, objectKey, payloadSHA, claimedReason, succeededReason string
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state,object_key,btrim(payload_sha256)
FROM evidence_snapshots WHERE id=$1`, snapshot.ID).Scan(&lifecycle, &objectKey, &payloadSHA); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT
max(reason_code) FILTER (WHERE event_type='delete_claimed'),
max(reason_code) FILTER (WHERE event_type='delete_succeeded')
FROM evidence_deletion_audits WHERE evidence_snapshot_id=$1`, snapshot.ID).Scan(&claimedReason, &succeededReason); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "tombstoned" || objectKey != snapshot.ObjectKey || payloadSHA != snapshot.PayloadSHA256 ||
		claimedReason != sourceapplication.RawEvidenceDeleteRightsRevoked || succeededReason != claimedReason {
		t.Fatalf("immutable revocation lineage = lifecycle:%s object:%t digest:%t reasons:%s/%s",
			lifecycle, objectKey == snapshot.ObjectKey, payloadSHA == snapshot.PayloadSHA256, claimedReason, succeededReason)
	}
}

func openRawEvidenceRevocationMinIO(
	t *testing.T,
	ctx context.Context,
) (*sourceminio.RawEvidenceStore, *miniosdk.Client, string, func()) {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_SECRET_KEY"))
	baseBucket := strings.Trim(strings.ToLower(strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_BUCKET"))), "-")
	useSSL := strings.EqualFold(strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_USE_SSL")), "true")
	if endpoint == "" || accessKey == "" || secretKey == "" || baseBucket == "" {
		t.Fatal("isolated MinIO test configuration is required")
	}
	randomBytes := make([]byte, 6)
	if _, err := rand.Read(randomBytes); err != nil {
		t.Fatal(err)
	}
	if len(baseBucket) > 30 {
		baseBucket = baseBucket[:30]
	}
	bucket := fmt.Sprintf("%s-revoke-%s", baseBucket, hex.EncodeToString(randomBytes))
	client, err := miniosdk.New(endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: useSSL,
		Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.MakeBucket(ctx, bucket, miniosdk.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for object := range client.ListObjects(cleanupCtx, bucket, miniosdk.ListObjectsOptions{Recursive: true}) {
			if object.Err == nil {
				_ = client.RemoveObject(cleanupCtx, bucket, object.Key, miniosdk.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(cleanupCtx, bucket)
	}
	store, err := sourceminio.NewRawEvidenceStore(config.MinIOConfig{
		Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey, Bucket: bucket, UseSSL: useSSL,
	})
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	return store, client, bucket, cleanup
}
