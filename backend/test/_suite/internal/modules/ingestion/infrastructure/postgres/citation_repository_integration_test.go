package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestCitationRepositoryPinsExactVersionAndRechecksCurrentRights(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	sourceID := createDocumentVersionSource(t, runtime, "citation")
	firstObservationID := insertSourceObservation(t, runtime, sourceID, "stable-work", 0)
	secondObservationID := insertSourceObservation(t, runtime, sourceID, "stable-work", 1)
	reader := &integrationDocumentObservationReader{observations: map[int64]ingestionapplication.DocumentObservationDTO{
		firstObservationID: {
			ID: firstObservationID, SourceConnectionID: sourceID, ExternalWorkID: "stable-work",
			BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: "first immutable body", Language: "en", CapturedAt: documentVersionCapturedAt(0),
		},
		secondObservationID: {
			ID: secondObservationID, SourceConnectionID: sourceID, ExternalWorkID: "stable-work",
			BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: "second immutable body", Language: "en", CapturedAt: documentVersionCapturedAt(1),
		},
	}}
	versions := ingestionpostgres.NewDocumentVersionRepository(runtime)
	service, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{Observations: reader, Versions: versions})
	if err != nil {
		t.Fatal(err)
	}

	first := persistCitationVersion(t, service, firstObservationID)
	firstArtifactSHA := strings.Repeat("c", 64)
	publishCitationVersion(t, runtime, service, sourceID, first, 101, firstArtifactSHA)
	firstDisplayID := createDocumentDisplayDecision(t, runtime, sourceID, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, 103, nil, first.DocumentVersion.ID)
	transitionDocumentVersionWithDisplay(t, service, first.DocumentVersion.ID, 3, firstDisplayID)
	attachCitationPartyFacts(t, runtime, sourceID, firstObservationID)

	second := persistCitationVersion(t, service, secondObservationID)
	secondArtifactSHA := strings.Repeat("d", 64)
	publishCitationVersion(t, runtime, service, sourceID, second, 111, secondArtifactSHA)
	secondDisplayID := createDocumentDisplayDecision(t, runtime, sourceID, second.DocumentVersion.ID,
		second.DocumentVersion.ContentSHA256, 113, nil, second.DocumentVersion.ID)
	transitionDocumentVersionWithDisplay(t, service, second.DocumentVersion.ID, 3, secondDisplayID)

	var currentVersionID int64
	if err := runtime.SQL.QueryRow(`SELECT current_document_version_id FROM documents WHERE id=$1`, first.Document.ID).Scan(&currentVersionID); err != nil {
		t.Fatal(err)
	}
	if currentVersionID != second.DocumentVersion.ID {
		t.Fatalf("current document version = %d, want newer %d", currentVersionID, second.DocumentVersion.ID)
	}

	repository := ingestionpostgres.NewCitationRepository(runtime)
	read, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(first) error = %v", err)
	}
	if read.DocumentVersionID != first.DocumentVersion.ID || read.DocumentID != first.Document.ID ||
		read.Title != "revision 0" || read.ContentSHA256 != first.DocumentVersion.ContentSHA256 ||
		read.Artifact == nil || read.Artifact.SHA256 != firstArtifactSHA || !read.DisplayPrivateAllowed ||
		!read.Artifact.StoreDerivedAllowed || !read.Artifact.RetainAllowed || read.Artifact.AnchorMap == nil ||
		len(read.Artifact.AnchorMap.Blocks) != 1 || read.Artifact.AnchorMap.Blocks[0].MarkdownAnchor != "body-0000-000000000000" ||
		read.Publisher == nil || read.Publisher.DisplayName != "Example Newsroom" || read.ContentOrigin == nil ||
		read.ContentOrigin.DisplayName != "Original Desk" || len(read.Distributors) != 1 || read.Distributors[0].DisplayName != "Syndication Desk" ||
		read.RawEvidence.Availability != ingestionapplication.CitationRawEvidenceAvailable || len(read.RawEvidence.PayloadSHA256s) != 1 {
		t.Fatalf("exact first-version projection = %#v", read)
	}
	if read.DocumentVersionID == currentVersionID || read.Artifact.SHA256 == secondArtifactSHA {
		t.Fatalf("exact read drifted to current version: %#v", read)
	}
	assertCitationRawEvidenceLifecycle(t, runtime, repository, first.DocumentVersion.ID, firstObservationID)
	if _, err := runtime.SQL.Exec(`UPDATE source_connections SET enabled=false, deleted_at=now() WHERE id=$1`, sourceID); err != nil {
		t.Fatalf("archive source connection fixture: %v", err)
	}
	archivedSourceRead, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(archived source) error = %v", err)
	}
	if archivedSourceRead.DocumentVersionID != first.DocumentVersion.ID || !archivedSourceRead.DisplayPrivateAllowed ||
		archivedSourceRead.Artifact == nil || !archivedSourceRead.Artifact.StoreDerivedAllowed || !archivedSourceRead.Artifact.RetainAllowed {
		t.Fatalf("operational source archive revoked historical document rights: %#v", archivedSourceRead)
	}

	denyPolicy := createDocumentObservationRightsPolicy(t, runtime, sourceID, firstObservationID, 121, time.Now().UTC().Add(-time.Hour))
	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, "display_private", "deny", nil, nil, first.DocumentVersion.ID)
	revoked, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(revoked) error = %v", err)
	}
	if revoked.DisplayPrivateAllowed {
		t.Fatalf("revoked exact version remained display-allowed: %#v", revoked)
	}
	if revoked.Artifact == nil || !revoked.Artifact.StoreDerivedAllowed || !revoked.Artifact.RetainAllowed {
		t.Fatalf("display revocation incorrectly changed independent artifact rights: %#v", revoked.Artifact)
	}

	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, "store_derived", "deny", nil, nil, first.DocumentVersion.ID)
	retentionDays := 1
	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, "retain", "deny", nil, &retentionDays, first.DocumentVersion.ID)
	fullyRevoked, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(fully revoked) error = %v", err)
	}
	if fullyRevoked.Artifact == nil || fullyRevoked.Artifact.StoreDerivedAllowed || fullyRevoked.Artifact.RetainAllowed ||
		fullyRevoked.Artifact.CurrentRetentionDays != nil {
		t.Fatalf("current artifact rights were not re-evaluated fail-closed: %#v", fullyRevoked.Artifact)
	}
}

func attachCitationPartyFacts(t *testing.T, runtime *database.Runtime, sourceID, observationID int64) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	policy := createDocumentRightsPolicy(t, runtime, sourceID, 97, now.Add(-time.Hour))
	snapshotKey := documentRightsFixtureDigest("citation-party-snapshot", fmt.Sprint(observationID))
	payloadSHA := documentRightsFixtureDigest("citation-party-payload", fmt.Sprint(observationID))
	storeDecisionID := insertCitationRawRightsDecision(t, runtime, policy, snapshotKey, payloadSHA, "store_raw", nil)
	retentionDays := 30
	retainDecisionID := insertCitationRawRightsDecision(t, runtime, policy, snapshotKey, payloadSHA, "retain", &retentionDays)
	var snapshotID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO evidence_snapshots (
  source_connection_id,store_raw_rights_decision_id,retain_rights_decision_id,
  snapshot_key,object_key,payload_sha256,collector_profile_version,mime_type,size_bytes,
  response_status,requested_url,final_url,redirect_chain,response_headers,captured_at,retention_until,lifecycle_state,available_at
) VALUES ($1,$2,$3,$4,$5,$6,'citation-party-fixture-v1','application/rss+xml',128,
          200,'https://feed.example.test/citation-party.xml','https://feed.example.test/citation-party.xml',
          '[]'::jsonb,'{}'::jsonb,$7,$8,'raw_available',CURRENT_TIMESTAMP)
RETURNING id`, sourceID, storeDecisionID, retainDecisionID, snapshotKey,
		fmt.Sprintf("source-raw/v1/%d/%s/%s.raw", sourceID, snapshotKey[:2], snapshotKey), payloadSHA,
		now, now.Add(30*24*time.Hour)).Scan(&snapshotID); err != nil {
		t.Fatalf("insert citation party evidence snapshot: %v", err)
	}
	var referenceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_observation_evidences (
  source_connection_id,source_observation_id,evidence_snapshot_id,locator_type,locator_value,
  selected_payload_sha256,selector_version
) VALUES ($1,$2,$3,'xml_path','/feed/item[1]',$4,'citation-party-selector-v1')
RETURNING id`, sourceID, observationID, snapshotID, payloadSHA).Scan(&referenceID); err != nil {
		t.Fatalf("insert citation party evidence reference: %v", err)
	}
	parties := []struct {
		role, kind, namespace, externalID, displayName, homepageURL string
	}{
		{"publisher", "organization", "publisher-registry", "publisher-42", "Example Newsroom", "https://publisher.example.test/"},
		{"content_origin", "organization", "origin-registry", "origin-9", "Original Desk", ""},
		{"distributor", "account", "platform-account", "distribution-7", "Syndication Desk", "https://distribution.example.test/accounts/7"},
	}
	for _, party := range parties {
		var partyID int64
		if err := runtime.SQL.QueryRow(`
INSERT INTO source_parties (source_connection_id,party_kind,identity_namespace,external_id)
VALUES ($1,$2,$3,$4) RETURNING id`, sourceID, party.kind, party.namespace, party.externalID).Scan(&partyID); err != nil {
			t.Fatalf("insert citation source party: %v", err)
		}
		if _, err := runtime.SQL.Exec(`
INSERT INTO source_observation_parties (
  source_connection_id,source_observation_id,source_party_id,evidence_reference_id,
  role,display_name_snapshot,homepage_url_snapshot
) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''))`, sourceID, observationID, partyID, referenceID,
			party.role, party.displayName, party.homepageURL); err != nil {
			t.Fatalf("insert citation observation party: %v", err)
		}
	}
}

func assertCitationRawEvidenceLifecycle(t *testing.T, runtime *database.Runtime, repository *ingestionpostgres.CitationRepository, documentVersionID, observationID int64) {
	t.Helper()
	ctx := context.Background()
	var snapshotID, approverID int64
	var objectKey, payloadSHA string
	var retentionUntil time.Time
	if err := runtime.SQL.QueryRow(`
SELECT snapshot.id,snapshot.object_key,btrim(snapshot.payload_sha256),snapshot.retention_until,policy.recorded_by_user_id
FROM source_observation_evidences AS reference
JOIN evidence_snapshots AS snapshot ON snapshot.id=reference.evidence_snapshot_id
JOIN source_rights_decisions AS decision ON decision.id=snapshot.retain_rights_decision_id
JOIN source_rights_policies AS policy ON policy.id=decision.policy_id
WHERE reference.source_observation_id=$1`, observationID).Scan(&snapshotID, &objectKey, &payloadSHA, &retentionUntil, &approverID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO evidence_retention_exceptions (evidence_snapshot_id,approved_by_user_id,approval_basis,approved_at)
VALUES ($1,$2,'citation lifecycle fixture',CURRENT_TIMESTAMP)`, snapshotID, approverID); err != nil {
		t.Fatal(err)
	}
	exceptionRead, err := repository.ReadCitation(ctx, documentVersionID)
	if err != nil || exceptionRead.RawEvidence.Availability != ingestionapplication.CitationRawEvidenceExceptionRetained || !exceptionRead.RawEvidence.ExceptionApproved {
		t.Fatalf("approved exception projection = %#v/%v", exceptionRead.RawEvidence, err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_retention_exceptions
SET revoked_by_user_id=$2,revoked_at=CURRENT_TIMESTAMP,revocation_basis='fixture completed'
WHERE evidence_snapshot_id=$1 AND revoked_at IS NULL`, snapshotID, approverID); err != nil {
		t.Fatal(err)
	}
	retention := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)
	at := retentionUntil.Add(time.Hour)
	candidates, err := retention.ClaimExpired(ctx, at, 1)
	if err != nil || len(candidates) != 1 || candidates[0].SnapshotID != snapshotID {
		t.Fatalf("claim citation evidence = %#v/%v", candidates, err)
	}
	if err := retention.CompleteDeletion(ctx, sourceapplication.CompleteRawEvidenceDeletionCommand{
		SnapshotID: snapshotID, AttemptNo: candidates[0].AttemptNo, ObjectKey: objectKey,
		PayloadSHA256: payloadSHA, DeletedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	expiredRead, err := repository.ReadCitation(ctx, documentVersionID)
	if err != nil || expiredRead.RawEvidence.Availability != ingestionapplication.CitationRawEvidenceExpired ||
		!expiredRead.RawEvidence.DeletionAudited || expiredRead.RawEvidence.PayloadSHA256s[0] != payloadSHA {
		t.Fatalf("expired raw evidence projection = %#v/%v", expiredRead.RawEvidence, err)
	}
}

func insertCitationRawRightsDecision(
	t *testing.T,
	runtime *database.Runtime,
	policy documentRightsPolicyFixture,
	snapshotKey, payloadSHA, action string,
	retentionDays *int,
) int64 {
	t.Helper()
	evaluatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	idempotencyKey, commandFingerprint := documentRightsFixtureReceipt(
		"citation-party-decision", fmt.Sprint(policy.ID), snapshotKey, payloadSHA, action,
	)
	var decisionID int64
	if err := runtime.SQL.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  )
  SELECT $1,$2,policy.version,'raw_response',$3,$4,policy.recorded_by_user_id,$11,$12,1
  FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,
  evaluator,evaluated_at,effective_from,retention_days
) SELECT decision_batch.id,$1,$2,$5,$6,$7,$8,$9,'raw_response',$3,$4,$10,'allow',
         'citation-party-fixture',$13,$14,$15
FROM decision_batch RETURNING id`, policy.SourceID, policy.ID, snapshotKey, payloadSHA,
		policy.Revision, policy.ScopeType, policy.Subject, policy.Priority, policy.Basis, action,
		idempotencyKey, commandFingerprint, evaluatedAt, policy.EffectiveAt, retentionDays).Scan(&decisionID); err != nil {
		t.Fatalf("insert citation raw %s decision: %v", action, err)
	}
	return decisionID
}

func persistCitationVersion(t *testing.T, service *ingestionapplication.DocumentVersionService, observationID int64) ingestionapplication.PersistDocumentVersionResult {
	t.Helper()
	result, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
		SourceObservationID: observationID, ExtractorVersion: "rss-entry-v2",
		ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("PersistSourceObservation(%d) error = %v", observationID, err)
	}
	return result
}

func publishCitationVersion(t *testing.T, runtime *database.Runtime, service *ingestionapplication.DocumentVersionService, sourceID int64, persisted ingestionapplication.PersistDocumentVersionResult, policyRevision int64, artifactSHA string) {
	t.Helper()
	documentVersionID := persisted.DocumentVersion.ID
	transitionDocumentVersion(t, service, documentVersionID, 1, ingestionapplication.DocumentDerivedPending)
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	policy := createDocumentRightsPolicy(t, runtime, sourceID, policyRevision, now.Add(-time.Hour))
	storeDecisionID := insertDocumentRightsDecision(t, runtime, policy, documentVersionID,
		persisted.DocumentVersion.ContentSHA256, "store_derived", nil, nil, documentVersionID)
	retentionDays := 30
	retainDecisionID := insertDocumentRightsDecision(t, runtime, policy, documentVersionID,
		persisted.DocumentVersion.ContentSHA256, "retain", nil, &retentionDays, documentVersionID)
	profileSHA := strings.Repeat("b", 64)
	var artifactID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO derived_artifacts (
  source_connection_id,document_version_id,store_derived_rights_decision_id,
  retain_rights_decision_id,artifact_type,transformer_profile_sha256,
	  vault_relative_path,mime_type,sha256,size_bytes,
	  anchor_normalization_version,anchor_map_profile_version,anchor_plaintext_sha256,anchor_markdown_sha256,anchor_map_sha256,
	  retention_until
	) VALUES ($1,$2,$3,$4,'markdown',$5,$6,'text/markdown; charset=utf-8',$7,12,
	          $8,$9,$10,$7,$11,$12)
RETURNING id`, sourceID, documentVersionID, storeDecisionID, retainDecisionID, profileSHA,
		fmt.Sprintf("documents/%d/%d/markdown/%s.md", persisted.Document.ID, documentVersionID, profileSHA),
		artifactSHA, ingestionapplication.CanonicalDocumentTextNormalizationVersion,
		ingestionapplication.CanonicalDocumentAnchorMapProfileVersion, persisted.DocumentVersion.ContentSHA256,
		strings.Repeat("e", 64), persisted.DocumentVersion.CapturedAt.Add(30*24*time.Hour)).Scan(&artifactID); err != nil {
		t.Fatalf("insert citation artifact: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO document_anchor_blocks (
  derived_artifact_id,anchor_map_sha256,block_ordinal,
  plaintext_utf8_byte_start,plaintext_utf8_byte_end,
  markdown_utf8_byte_start,markdown_utf8_byte_end,markdown_anchor
) VALUES ($1,$2,0,0,1,0,12,'body-0000-000000000000')`, artifactID, strings.Repeat("e", 64)); err != nil {
		t.Fatalf("insert citation anchor block: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state='derived_available', available_at=now(), active=true, updated_at=now()
WHERE id=$1`, artifactID); err != nil {
		t.Fatalf("publish citation artifact: %v", err)
	}
	transitionDocumentVersion(t, service, documentVersionID, 2, ingestionapplication.DocumentDerivedAvailable)
}
