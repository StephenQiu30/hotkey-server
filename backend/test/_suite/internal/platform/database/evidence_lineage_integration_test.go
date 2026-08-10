package database

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEvidenceLineageAndLifecycleConstraints(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%d", now.UnixNano())

	sourceID := insertEvidenceSource(t, runtime, "primary-"+suffix)
	otherSourceID := insertEvidenceSource(t, runtime, "other-"+suffix)
	organizationPolicyHash := strings.Repeat("0", 64)
	organizationActorID := insertEvidenceRightsFixtureActor(t, runtime, organizationPolicyHash)
	organizationIdempotencyKey, organizationCommandFingerprint := evidenceRightsFixtureReceipt("policy", organizationPolicyHash)
	var organizationPolicyID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id, approved_by_user_id, idempotency_key, command_fingerprint,
  scope_type, scope_subject, policy_revision, priority, basis_summary, policy_hash, effective_at
) VALUES ($1, $1, $2, $3, 'organization_default', '', 1, 100, 'organization default fixture', $4, $5)
RETURNING id`, organizationActorID, organizationIdempotencyKey, organizationCommandFingerprint,
		organizationPolicyHash, now.Add(-time.Hour)).Scan(&organizationPolicyID); err != nil {
		t.Fatalf("insert organization policy: %v", err)
	}
	organizationDecision := rightsDecisionFixture{
		SourceID: sourceID, PolicyID: organizationPolicyID, PolicyRevision: 1,
		PolicyScopeType: "organization_default", PriorityRank: 100, PolicyBasis: "organization default fixture",
		SubjectType: "raw_response", SubjectKey: strings.Repeat("0", 64), InputDigest: strings.Repeat("1", 64),
		Action: "fetch", Decision: "deny", EvaluatedAt: now, EffectiveFrom: now.Add(-time.Minute),
	}
	if _, err := insertRightsDecision(runtime, organizationDecision); err != nil {
		t.Fatalf("insert first organization decision: %v", err)
	}
	organizationDecision.SourceID = otherSourceID
	if _, err := insertRightsDecision(runtime, organizationDecision); err != nil {
		t.Fatalf("organization policy could not decide the same subject independently for another source: %v", err)
	}
	policySubject := "source-" + suffix
	if _, err := runtime.SQL.Exec(`
INSERT INTO source_rights_policies (
  recorded_by_user_id, approved_by_user_id, idempotency_key, command_fingerprint,
  source_connection_id, scope_type, scope_subject, policy_revision, priority,
  basis_summary, policy_hash, effective_at
) VALUES ($1, $1, $2, $3, $4, 'source_endpoint', $5, 99, 300, 'invalid hash fixture', $6, $7)`,
		organizationActorID, "fixture.policy.invalid-hash."+suffix, strings.Repeat("f", 64),
		sourceID, policySubject, strings.Repeat("A", 64), now.Add(-time.Hour)); err == nil {
		t.Fatal("rights policy accepted an uppercase hexadecimal policy hash")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	policyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 1, strings.Repeat("a", 64), "authorized feed fixture", now.Add(-time.Hour))
	if _, err := runtime.SQL.Exec(`UPDATE source_rights_policies SET basis_summary = 'mutated' WHERE id = $1`, policyID); err == nil {
		t.Fatal("immutable rights policy accepted an update")
	}
	var unapprovedPolicyID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
  scope_type,scope_subject,policy_revision,priority,basis_summary,policy_hash,effective_at
) VALUES ($1,$2,$3,$4,'source_endpoint',$5,98,300,'unapproved fixture',$6,$7)
RETURNING id`, organizationActorID, "fixture.policy.unapproved."+suffix, strings.Repeat("e", 64),
		sourceID, policySubject, strings.Repeat("4", 64), now.Add(-time.Hour)).Scan(&unapprovedPolicyID); err != nil {
		t.Fatalf("insert unapproved policy: %v", err)
	}
	unapprovedAllow := rightsDecisionFixture{
		SourceID: sourceID, PolicyID: unapprovedPolicyID, PolicyRevision: 98,
		PolicyScopeType: "source_endpoint", PolicySubject: policySubject,
		PriorityRank: 300, PolicyBasis: "unapproved fixture", SubjectType: "raw_response",
		SubjectKey: strings.Repeat("e", 64), InputDigest: strings.Repeat("f", 64),
		Action: "store_raw", Decision: "allow", EvaluatedAt: now, EffectiveFrom: now.Add(-time.Minute),
	}
	if _, err := insertRightsDecision(runtime, unapprovedAllow); err == nil {
		t.Fatal("allow decision accepted an unapproved policy")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}

	snapshotKey := strings.Repeat("c", 64)
	payloadDigest := strings.Repeat("d", 64)
	baseDecision := rightsDecisionFixture{
		SourceID: otherSourceID, PolicyID: policyID, PolicyRevision: 1,
		PolicyScopeType: "source_endpoint", PolicySubject: policySubject,
		PriorityRank: 300, PolicyBasis: "authorized feed fixture",
		SubjectType: "raw_response", SubjectKey: snapshotKey, InputDigest: payloadDigest,
		Action: "store_raw", Decision: "allow", EvaluatedAt: now, EffectiveFrom: now.Add(-time.Minute),
	}
	if _, err := insertRightsDecision(runtime, baseDecision); err == nil {
		t.Fatal("source rights decision accepted a policy owned by another source")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	baseDecision.SourceID = sourceID
	baseDecision.RetentionDays = intPointer(30)
	if _, err := insertRightsDecision(runtime, baseDecision); err == nil {
		t.Fatal("store_raw decision accepted retain-only duration")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	baseDecision.RetentionDays = nil
	storeRawDecisionID, err := insertRightsDecision(runtime, baseDecision)
	if err != nil {
		t.Fatalf("insert store_raw decision: %v", err)
	}
	retainDecision := baseDecision
	retainDecision.Action = "retain"
	retainDecision.RetentionDays = intPointer(30)
	retainDecisionID, err := insertRightsDecision(runtime, retainDecision)
	if err != nil {
		t.Fatalf("insert raw retain decision: %v", err)
	}

	_, err = runtime.SQL.Exec(`
INSERT INTO evidence_snapshots (
  source_connection_id, store_raw_rights_decision_id, retain_rights_decision_id,
  snapshot_key, object_key, payload_sha256, collector_profile_version, mime_type, size_bytes, response_status,
  requested_url, final_url, response_headers, captured_at, retention_until
) VALUES ($1, $2, $3, $4, $5, $6, 'rss-http-v1', 'application/atom+xml', 1, 200,
  'https://feed.example.test/evidence-lineage', 'https://feed.example.test/evidence-lineage',
  '{"set-cookie":"session=forbidden"}'::jsonb, $7, $8)`, sourceID, storeRawDecisionID,
		retainDecisionID, snapshotKey, "source-raw/v1/forbidden-"+suffix, payloadDigest, now, now.Add(30*24*time.Hour))
	if err == nil {
		t.Fatal("evidence snapshot accepted a forbidden response header")
	}
	assertPostgreSQLState(t, err, "23514")

	_, err = runtime.SQL.Exec(`
INSERT INTO evidence_snapshots (
  source_connection_id, store_raw_rights_decision_id, retain_rights_decision_id,
  snapshot_key, object_key, payload_sha256, collector_profile_version, mime_type, size_bytes, response_status,
  requested_url, final_url, captured_at, retention_until
) VALUES ($1, $2, $3, $4, $5, $6, 'RSS-http-v1', 'application/atom+xml', 1, 200,
  'https://feed.example.test/evidence-lineage', 'https://feed.example.test/evidence-lineage', $7, $8)`,
		sourceID, storeRawDecisionID, retainDecisionID, snapshotKey,
		"source-raw/v1/noncanonical-profile-"+suffix, payloadDigest, now, now.Add(30*24*time.Hour))
	if err == nil {
		t.Fatal("evidence snapshot accepted a non-canonical collector profile version")
	}
	assertPostgreSQLState(t, err, "23514")

	expiredSnapshotKey := strings.Repeat("7", 64)
	expiredPayloadDigest := strings.Repeat("6", 64)
	expiredAt := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Microsecond)
	expiredDecision := baseDecision
	expiredDecision.SubjectKey = expiredSnapshotKey
	expiredDecision.InputDigest = expiredPayloadDigest
	expiredDecision.EvaluatedAt = expiredAt.Add(-time.Minute)
	expiredDecision.EffectiveFrom = expiredAt.Add(-time.Minute)
	expiredDecision.ExpiresAt = &expiredAt
	expiredStoreDecisionID, err := insertRightsDecision(runtime, expiredDecision)
	if err != nil {
		t.Fatalf("insert historical store_raw decision: %v", err)
	}
	expiredRetain := expiredDecision
	expiredRetain.Action = "retain"
	expiredRetain.RetentionDays = intPointer(30)
	expiredRetainDecisionID, err := insertRightsDecision(runtime, expiredRetain)
	if err != nil {
		t.Fatalf("insert historical retain decision: %v", err)
	}
	expiredCapturedAt := expiredAt.Add(-time.Minute)
	_, err = runtime.SQL.Exec(`
INSERT INTO evidence_snapshots (
  source_connection_id, store_raw_rights_decision_id, retain_rights_decision_id,
  snapshot_key, object_key, payload_sha256, collector_profile_version, mime_type, size_bytes, response_status,
  requested_url, final_url, captured_at, retention_until
) VALUES ($1, $2, $3, $4, $5, $6, 'rss-http-v1', 'application/atom+xml', 1, 200,
  'https://feed.example.test/evidence-lineage', 'https://feed.example.test/evidence-lineage', $7, $8)`,
		sourceID, expiredStoreDecisionID, expiredRetainDecisionID, expiredSnapshotKey,
		"source-raw/v1/expired-"+suffix, expiredPayloadDigest, expiredCapturedAt, expiredCapturedAt.Add(30*24*time.Hour))
	if err == nil {
		t.Fatal("new snapshot accepted a store_raw decision that was valid only at historical capture time")
	}
	assertPostgreSQLState(t, err, "23514")

	_, err = runtime.SQL.Exec(`
INSERT INTO evidence_snapshots (
  source_connection_id, store_raw_rights_decision_id, retain_rights_decision_id,
  snapshot_key, object_key, payload_sha256, collector_profile_version, mime_type, size_bytes, response_status,
  requested_url, final_url, captured_at, retention_until
) VALUES ($1, $2, $3, $4, $5, $6, 'rss-http-v1', 'application/atom+xml', 1, 200,
  'https://feed.example.test/evidence-lineage', 'https://feed.example.test/evidence-lineage', $7, $8)`,
		sourceID, storeRawDecisionID, retainDecisionID, strings.Repeat("8", 64),
		"source-raw/v1/mismatched-"+suffix, payloadDigest, now, now.Add(30*24*time.Hour))
	if err == nil {
		t.Fatal("evidence snapshot accepted rights decisions for another snapshot subject")
	}
	assertPostgreSQLState(t, err, "23514")
	_, err = runtime.SQL.Exec(`
		INSERT INTO evidence_snapshots (
		  source_connection_id, store_raw_rights_decision_id, retain_rights_decision_id,
		  snapshot_key, object_key, payload_sha256, collector_profile_version, mime_type, size_bytes, response_status,
		  requested_url, final_url, captured_at, retention_until
		) VALUES ($1, $2, $3, $4, $5, $6, 'rss-http-v1', 'application/atom+xml', 1, 200,
	  'https://feed.example.test/evidence-lineage', 'https://feed.example.test/evidence-lineage', $7, $8)`,
		sourceID, storeRawDecisionID, retainDecisionID, snapshotKey,
		"source-raw/v1/over-retained-"+suffix, payloadDigest, now, now.Add(31*24*time.Hour))
	if err == nil {
		t.Fatal("evidence snapshot exceeded its exact retain decision")
	}
	assertPostgreSQLState(t, err, "23514")

	var snapshotID int64
	futureCapturedAt := time.Now().UTC().Add(6 * time.Minute).Truncate(time.Microsecond)
	_, err = runtime.SQL.Exec(`
INSERT INTO evidence_snapshots (
  source_connection_id, store_raw_rights_decision_id, retain_rights_decision_id,
  snapshot_key, object_key, payload_sha256, collector_profile_version, mime_type, size_bytes, response_status,
  requested_url, final_url, captured_at, retention_until
) VALUES ($1, $2, $3, $4, $5, $6, 'rss-http-v1', 'application/atom+xml', 1, 200,
  'https://feed.example.test/evidence-lineage', 'https://feed.example.test/evidence-lineage', $7, $8)`,
		sourceID, storeRawDecisionID, retainDecisionID, snapshotKey,
		"source-raw/v1/future-"+suffix, payloadDigest, futureCapturedAt, futureCapturedAt.Add(30*24*time.Hour))
	if err == nil {
		t.Fatal("evidence snapshot accepted a capture time beyond the collector clock-skew boundary")
	}
	assertPostgreSQLState(t, err, "23514")

	if err := runtime.SQL.QueryRow(`
INSERT INTO evidence_snapshots (
  source_connection_id, store_raw_rights_decision_id, retain_rights_decision_id,
  snapshot_key, object_key, payload_sha256, collector_profile_version, mime_type, size_bytes, response_status,
  requested_url, final_url, captured_at, retention_until
) VALUES ($1, $2, $3, $4, $5, $6, 'rss-http-v1', 'application/atom+xml; charset=utf-8', 128, 200,
  'https://feed.example.test/evidence-lineage', 'https://feed.example.test/evidence-lineage', $7, $8)
RETURNING id`, sourceID, storeRawDecisionID, retainDecisionID, snapshotKey,
		"source-raw/v1/"+suffix, payloadDigest, now, now.Add(30*24*time.Hour)).Scan(&snapshotID); err != nil {
		t.Fatalf("insert evidence snapshot: %v", err)
	}
	shortRetainPolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 2, strings.Repeat("b", 64), "shorter retention fixture", now.Add(-time.Hour))
	shortRetain := retainDecision
	shortRetain.PolicyID = shortRetainPolicyID
	shortRetain.PolicyRevision = 2
	shortRetain.PolicyBasis = "shorter retention fixture"
	shortRetain.RetentionDays = intPointer(10)
	shortRetainDecisionID, err := insertRightsDecision(runtime, shortRetain)
	if err != nil {
		t.Fatalf("insert shorter retain decision: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_snapshots
SET lifecycle_state = 'raw_available', available_at = $2, updated_at = $2
WHERE id = $1`, snapshotID, now.Add(time.Second)); err == nil {
		t.Fatal("evidence snapshot commit exceeded a newer conservative retention allow")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	restoredRetainPolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 3, strings.Repeat("c", 64), "restored retention fixture", now.Add(-time.Hour))
	restoredRetain := retainDecision
	restoredRetain.PolicyID = restoredRetainPolicyID
	restoredRetain.PolicyRevision = 3
	restoredRetain.PolicyBasis = "restored retention fixture"
	restoredRetain.SupersedesDecisionID = &shortRetainDecisionID
	if _, err := insertRightsDecision(runtime, restoredRetain); err != nil {
		t.Fatalf("insert restored retain decision: %v", err)
	}
	rawDenyPolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 4, strings.Repeat("d", 64), "raw storage revoked fixture", now.Add(-time.Hour))
	rawDeny := baseDecision
	rawDeny.PolicyID = rawDenyPolicyID
	rawDeny.PolicyRevision = 4
	rawDeny.PolicyBasis = "raw storage revoked fixture"
	rawDeny.Decision = "deny"
	rawDeny.SupersedesDecisionID = &storeRawDecisionID
	rawDenyDecisionID, err := insertRightsDecision(runtime, rawDeny)
	if err != nil {
		t.Fatalf("insert store_raw denial: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_snapshots
SET lifecycle_state = 'raw_available', available_at = $2, updated_at = $2
WHERE id = $1`, snapshotID, now.Add(time.Second)); err == nil {
		t.Fatal("evidence snapshot committed after store_raw was revoked")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_snapshots
SET lifecycle_state = 'policy_blocked', updated_at = $2
WHERE id = $1`, snapshotID, now.Add(time.Second)); err != nil {
		t.Fatalf("block raw snapshot after revocation: %v", err)
	}
	rawRestorePolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 5, strings.Repeat("e", 64), "raw storage restored fixture", now.Add(-time.Hour))
	rawRestore := baseDecision
	rawRestore.PolicyID = rawRestorePolicyID
	rawRestore.PolicyRevision = 5
	rawRestore.PolicyBasis = "raw storage restored fixture"
	rawRestore.SupersedesDecisionID = &rawDenyDecisionID
	if _, err := insertRightsDecision(runtime, rawRestore); err != nil {
		t.Fatalf("insert restored store_raw allow: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_snapshots
SET lifecycle_state = 'raw_pending', updated_at = $2
WHERE id = $1`, snapshotID, now.Add(2*time.Second)); err != nil {
		t.Fatalf("recover blocked raw snapshot under a new allow: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_snapshots
SET lifecycle_state = 'raw_available', available_at = $2, updated_at = $2
WHERE id = $1`, snapshotID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("advance raw lifecycle: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_snapshots
SET lifecycle_state = 'raw_pending', available_at = NULL, updated_at = $2
WHERE id = $1`, snapshotID, now.Add(4*time.Second)); err == nil {
		t.Fatal("evidence snapshot accepted a backwards lifecycle transition")
	}

	var observationID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_observations (
  source_connection_id, external_id, upstream_identity, source_code, content_type,
  title, language, source_record_url, canonical_url, body_origin, completeness,
  discovered_at, captured_at
) VALUES ($1, $2, $3, 'rss', 'article', 'Archived item', 'en',
  'https://feed.example.test/evidence-lineage#entry', 'https://publisher.example.test/article',
  'feed_content', 'full', $4, $4)
RETURNING id`, sourceID, "entry-"+suffix, strings.Repeat("e", 64), now).Scan(&observationID); err != nil {
		t.Fatalf("insert source observation: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO source_observation_evidences (
  source_connection_id, source_observation_id, evidence_snapshot_id, locator_type,
  locator_value, selected_payload_sha256, selector_version
) VALUES ($1, $2, $3, 'xml_path', '/feed/entry[1]', $4, 'rss-xml-v1')`,
		sourceID, observationID, snapshotID, strings.Repeat("f", 64)); err != nil {
		t.Fatalf("link observation evidence: %v", err)
	}

	var documentID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO documents (source_connection_id, document_key, external_work_id)
VALUES ($1, $2, $3) RETURNING id`, sourceID, strings.Repeat("1", 64), "entry-"+suffix).Scan(&documentID); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	var otherObservationID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_observations (
  source_connection_id, external_id, upstream_identity, source_code, content_type,
  source_record_url, body_origin, completeness, discovered_at, captured_at
) VALUES ($1, $2, $3, 'rss', 'article', 'https://feed.example.test/other',
  'feed_content', 'full', $4, $4) RETURNING id`, otherSourceID, "other-entry-"+suffix,
		strings.Repeat("a", 64), now).Scan(&otherObservationID); err != nil {
		t.Fatalf("insert other-source observation: %v", err)
	}
	_, err = runtime.SQL.Exec(`
INSERT INTO document_versions (
  document_id, source_observation_id, revision_no, version_key, body_origin,
  completeness, content_sha256, extractor_version, extractor_profile_version,
  extractor_profile_sha256, captured_at
) VALUES ($1, $2, 1, $3, 'feed_content', 'full', $4, 'atom-v1', 'atom-profile-v1', $5, $6)`,
		documentID, otherObservationID, strings.Repeat("5", 64), strings.Repeat("6", 64), strings.Repeat("7", 64), now)
	if err == nil {
		t.Fatal("document version accepted a source observation owned by another source")
	}
	assertPostgreSQLState(t, err, "23514")
	contentDigest := strings.Repeat("3", 64)
	var documentVersionID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO document_versions (
  document_id, source_observation_id, revision_no, version_key, body_origin,
  completeness, word_count, language, content_sha256, extractor_version,
  extractor_profile_version, extractor_profile_sha256, quality_score, captured_at
) VALUES ($1, $2, 1, $3, 'feed_content', 'full', 2, 'en', $4, 'atom-v1', 'atom-profile-v1', $5, 87.50, $6)
RETURNING id`, documentID, observationID, strings.Repeat("2", 64), contentDigest, strings.Repeat("4", 64), now).Scan(&documentVersionID); err != nil {
		t.Fatalf("insert document version: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET content_sha256 = $2, version = version + 1, updated_at = $3
WHERE id = $1`, documentVersionID, strings.Repeat("5", 64), now.Add(time.Second)); err == nil {
		t.Fatal("document version accepted an in-place content mutation")
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'derive_pending', version = version + 1, updated_at = $2
WHERE id = $1`, documentVersionID, now.Add(time.Second)); err != nil {
		t.Fatalf("advance document to derive pending: %v", err)
	}

	documentSubject := strconv.FormatInt(documentVersionID, 10)
	derivedDecision := baseDecision
	derivedDecision.SubjectType = "document_version"
	derivedDecision.SubjectKey = documentSubject
	derivedDecision.InputDigest = contentDigest
	derivedDecision.Action = "store_derived"
	wrongDerivedDecision := derivedDecision
	wrongDerivedDecision.SubjectKey = strconv.FormatInt(documentVersionID+1, 10)
	wrongDerivedDecisionID, err := insertRightsDecision(runtime, wrongDerivedDecision)
	if err != nil {
		t.Fatalf("insert mismatched store_derived decision: %v", err)
	}
	storeDerivedDecisionID, err := insertRightsDecision(runtime, derivedDecision)
	if err != nil {
		t.Fatalf("insert store_derived decision: %v", err)
	}
	derivedRetainDecision := derivedDecision
	derivedRetainDecision.Action = "retain"
	derivedRetainDecision.RetentionDays = intPointer(30)
	derivedRetainDecisionID, err := insertRightsDecision(runtime, derivedRetainDecision)
	if err != nil {
		t.Fatalf("insert derived retain decision: %v", err)
	}

	profile := strings.Repeat("6", 64)
	path := fmt.Sprintf("documents/%d/%d/markdown/%s.md", documentID, documentVersionID, profile)
	artifactSHA := strings.Repeat("7", 64)
	anchorMapSHA := strings.Repeat("a", 64)
	_, err = runtime.SQL.Exec(`
INSERT INTO derived_artifacts (
  source_connection_id, document_version_id, store_derived_rights_decision_id,
  retain_rights_decision_id, artifact_type, transformer_profile_sha256,
  vault_relative_path, mime_type, sha256, size_bytes,
  anchor_normalization_version, anchor_map_profile_version, anchor_plaintext_sha256,
  anchor_markdown_sha256, anchor_map_sha256, retention_until
) VALUES ($1, $2, $3, $4, 'markdown', $5, $6, 'text/markdown; charset=utf-8', $7, 64,
  'nfc-lf-collapse-space-v1', 'commonmark-gfm-visible-blocks-v1', $8, $7, $9, $10)`,
		sourceID, documentVersionID, wrongDerivedDecisionID, derivedRetainDecisionID,
		profile, path, artifactSHA, contentDigest, anchorMapSHA, now.Add(30*24*time.Hour))
	if err == nil {
		t.Fatal("derived artifact accepted a store_derived decision for another document version")
	}
	assertPostgreSQLState(t, err, "23514")
	_, err = runtime.SQL.Exec(`
	INSERT INTO derived_artifacts (
	  source_connection_id, document_version_id, store_derived_rights_decision_id,
	  retain_rights_decision_id, artifact_type, transformer_profile_sha256,
	  vault_relative_path, mime_type, sha256, size_bytes,
	  anchor_normalization_version, anchor_map_profile_version, anchor_plaintext_sha256,
	  anchor_markdown_sha256, anchor_map_sha256, retention_until
	) VALUES ($1, $2, $3, $4, 'markdown', $5, '../escape.md',
	  'text/markdown; charset=utf-8', $6, 64,
	  'nfc-lf-collapse-space-v1', 'commonmark-gfm-visible-blocks-v1', $7, $6, $8, $9)`, sourceID, documentVersionID,
		storeDerivedDecisionID, derivedRetainDecisionID, profile, artifactSHA, contentDigest, anchorMapSHA, now.Add(30*24*time.Hour))
	if err == nil {
		t.Fatal("derived artifact accepted a non-deterministic Vault path")
	}
	assertPostgreSQLState(t, err, "23514")
	_, err = runtime.SQL.Exec(`
	INSERT INTO derived_artifacts (
	  source_connection_id, document_version_id, store_derived_rights_decision_id,
	  retain_rights_decision_id, artifact_type, transformer_profile_sha256,
	  vault_relative_path, mime_type, sha256, size_bytes,
	  anchor_normalization_version, anchor_map_profile_version, anchor_plaintext_sha256,
	  anchor_markdown_sha256, anchor_map_sha256, retention_until
	) VALUES ($1, $2, $3, $4, 'markdown', $5, $6,
	  'text/markdown; charset=utf-8', $7, 64,
	  'nfc-lf-collapse-space-v1', 'commonmark-gfm-visible-blocks-v1', $8, $7, $9, $10)`, sourceID, documentVersionID,
		storeDerivedDecisionID, derivedRetainDecisionID, profile, path,
		artifactSHA, contentDigest, anchorMapSHA, now.Add(31*24*time.Hour))
	if err == nil {
		t.Fatal("derived artifact exceeded its exact retain decision")
	}
	assertPostgreSQLState(t, err, "23514")

	var artifactID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO derived_artifacts (
  source_connection_id, document_version_id, store_derived_rights_decision_id,
  retain_rights_decision_id, artifact_type, transformer_profile_sha256,
  vault_relative_path, mime_type, sha256, size_bytes,
  anchor_normalization_version, anchor_map_profile_version, anchor_plaintext_sha256,
  anchor_markdown_sha256, anchor_map_sha256, retention_until
) VALUES ($1, $2, $3, $4, 'markdown', $5, $6, 'text/markdown; charset=utf-8', $7, 64,
  'nfc-lf-collapse-space-v1', 'commonmark-gfm-visible-blocks-v1', $8, $7, $9, $10)
RETURNING id`, sourceID, documentVersionID, storeDerivedDecisionID, derivedRetainDecisionID,
		profile, path, artifactSHA, contentDigest, anchorMapSHA, now.Add(30*24*time.Hour)).Scan(&artifactID); err != nil {
		t.Fatalf("insert derived artifact: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO document_anchor_blocks (
  derived_artifact_id, anchor_map_sha256, block_ordinal,
  plaintext_utf8_byte_start, plaintext_utf8_byte_end,
  markdown_utf8_byte_start, markdown_utf8_byte_end, markdown_anchor
) VALUES ($1, $2, 0, 0, 2, 0, 2, 'body-0000-000000000001')`, artifactID, anchorMapSHA); err != nil {
		t.Fatalf("insert document anchor block: %v", err)
	}
	derivedDenyPolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 6, strings.Repeat("f", 64), "derived storage revoked fixture", now.Add(-time.Hour))
	derivedDeny := derivedDecision
	derivedDeny.PolicyID = derivedDenyPolicyID
	derivedDeny.PolicyRevision = 6
	derivedDeny.PolicyBasis = "derived storage revoked fixture"
	derivedDeny.Decision = "deny"
	derivedDeny.SupersedesDecisionID = &storeDerivedDecisionID
	derivedDenyDecisionID, err := insertRightsDecision(runtime, derivedDeny)
	if err != nil {
		t.Fatalf("insert store_derived denial: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state = 'derived_available', available_at = $2, active = true, updated_at = $2
WHERE id = $1`, artifactID, now.Add(2*time.Second)); err == nil {
		t.Fatal("derived artifact committed after store_derived was revoked")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state = 'derive_failed', failure_code = 'policy_revoked', updated_at = $2
WHERE id = $1`, artifactID, now.Add(2*time.Second)); err != nil {
		t.Fatalf("record derived policy failure: %v", err)
	}
	derivedRestorePolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 7, strings.Repeat("8", 64), "derived storage restored fixture", now.Add(-time.Hour))
	derivedRestore := derivedDecision
	derivedRestore.PolicyID = derivedRestorePolicyID
	derivedRestore.PolicyRevision = 7
	derivedRestore.PolicyBasis = "derived storage restored fixture"
	derivedRestore.SupersedesDecisionID = &derivedDenyDecisionID
	if _, err := insertRightsDecision(runtime, derivedRestore); err != nil {
		t.Fatalf("insert restored store_derived allow: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state = 'derive_pending', failure_code = NULL, updated_at = $2
WHERE id = $1`, artifactID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("retry derived artifact under a new allow: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state = 'derived_available', available_at = $2, active = true, updated_at = $2
WHERE id = $1`, artifactID, now.Add(4*time.Second)); err != nil {
		t.Fatalf("publish derived artifact: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'derived_available', version = version + 1, updated_at = $2
WHERE id = $1`, documentVersionID, now.Add(2*time.Second)); err != nil {
		t.Fatalf("advance document to derived_available: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'readable', version = version + 1, updated_at = $2
WHERE id = $1`, documentVersionID, now.Add(3*time.Second)); err == nil {
		t.Fatal("document became readable without an exact display_private allow")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}

	displayDecision := derivedDecision
	displayDecision.Action = "display_private"
	displayDecisionID, err := insertRightsDecision(runtime, displayDecision)
	if err != nil {
		t.Fatalf("insert display_private allow: %v", err)
	}
	unrelatedDerivedDenyPolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 8, strings.Repeat("9", 64), "same-priority derived denial fixture", now.Add(-time.Hour))
	unrelatedDerivedDeny := derivedDecision
	unrelatedDerivedDeny.PolicyID = unrelatedDerivedDenyPolicyID
	unrelatedDerivedDeny.PolicyRevision = 8
	unrelatedDerivedDeny.PolicyBasis = "same-priority derived denial fixture"
	unrelatedDerivedDeny.Decision = "deny"
	unrelatedDerivedDenyID, err := insertRightsDecision(runtime, unrelatedDerivedDeny)
	if err != nil {
		t.Fatalf("insert unrelated same-priority store_derived denial: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'readable', display_private_rights_decision_id = $2,
    version = version + 1, updated_at = $3
WHERE id = $1`, documentVersionID, displayDecisionID, now.Add(3*time.Second)); err == nil {
		t.Fatal("same-priority store_derived denial left an old artifact readable")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'policy_blocked', version = version + 1, updated_at = $2
WHERE id = $1`, documentVersionID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("block document after current artifact rights denial: %v", err)
	}
	currentDerivedAllowPolicyID := insertRightsPolicy(t, runtime, sourceID, policySubject, 9, strings.Repeat("1", 64), "current derived allow fixture", now.Add(-time.Hour))
	currentDerivedAllow := derivedDecision
	currentDerivedAllow.PolicyID = currentDerivedAllowPolicyID
	currentDerivedAllow.PolicyRevision = 9
	currentDerivedAllow.PolicyBasis = "current derived allow fixture"
	currentDerivedAllow.SupersedesDecisionID = &unrelatedDerivedDenyID
	if _, err := insertRightsDecision(runtime, currentDerivedAllow); err != nil {
		t.Fatalf("insert current store_derived allow: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'readable', display_private_rights_decision_id = $2,
    version = version + 1, updated_at = $3
WHERE id = $1`, documentVersionID, displayDecisionID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("advance document to readable: %v", err)
	}

	policy2ID := insertRightsPolicy(t, runtime, sourceID, policySubject, 10, strings.Repeat("2", 64), "revoked feed fixture", now.Add(-time.Hour))
	denyDisplay := displayDecision
	denyDisplay.PolicyID = policy2ID
	denyDisplay.PolicyRevision = 10
	denyDisplay.PolicyBasis = "revoked feed fixture"
	denyDisplay.Decision = "deny"
	denyDisplay.SupersedesDecisionID = &displayDecisionID
	denyDisplayID, err := insertRightsDecision(runtime, denyDisplay)
	if err != nil {
		t.Fatalf("insert display_private denial: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'policy_blocked', version = version + 1, updated_at = $2
WHERE id = $1`, documentVersionID, now.Add(4*time.Second)); err != nil {
		t.Fatalf("block readable document: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'readable', display_private_rights_decision_id = $2,
    version = version + 1, updated_at = $3
WHERE id = $1`, documentVersionID, displayDecisionID, now.Add(5*time.Second)); err == nil {
		t.Fatal("superseded display_private allow made a document readable")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}

	policy3ID := insertRightsPolicy(t, runtime, sourceID, policySubject, 11, strings.Repeat("3", 64), "restored feed fixture", now.Add(-time.Hour))
	restoredDisplay := displayDecision
	restoredDisplay.PolicyID = policy3ID
	restoredDisplay.PolicyRevision = 11
	restoredDisplay.PolicyBasis = "restored feed fixture"
	restoredDisplay.SupersedesDecisionID = &denyDisplayID
	restoredDisplayID, err := insertRightsDecision(runtime, restoredDisplay)
	if err != nil {
		t.Fatalf("insert restored display_private allow: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions
SET lifecycle_state = 'readable', display_private_rights_decision_id = $2,
    version = version + 1, updated_at = $3
WHERE id = $1`, documentVersionID, restoredDisplayID, now.Add(6*time.Second)); err != nil {
		t.Fatalf("restore document readability: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE documents
SET current_document_version_id = $2, version = version + 1, updated_at = $3
WHERE id = $1`, documentID, documentVersionID, now.Add(7*time.Second)); err != nil {
		t.Fatalf("select current document version: %v", err)
	}

	if _, err := runtime.SQL.Exec(`
	INSERT INTO document_versions (
	  document_id, source_observation_id, revision_no, version_key, body_origin,
	  completeness, content_sha256, extractor_version, extractor_profile_version, extractor_profile_sha256, captured_at
	) VALUES ($1, $2, 2, $3, 'feed_content', 'full', $4, 'atom-v1', 'atom-profile-v1', $5, $6)`,
		documentID, observationID, strings.Repeat("8", 64), contentDigest, strings.Repeat("0", 64), now); err == nil {
		t.Fatal("document version accepted a duplicate business idempotency identity")
	}
	if _, err := runtime.SQL.Exec(`
	INSERT INTO document_versions (
	  document_id, source_observation_id, revision_no, version_key, body_origin,
	  completeness, content_sha256, extractor_version, extractor_profile_version, extractor_profile_sha256, captured_at
	) VALUES ($1, $2, 2, $3, 'feed_content', 'full', $4, 'atom-v2', 'atom-profile-v2', $5, $6)`,
		documentID, observationID, strings.Repeat("8", 64), strings.Repeat("9", 64), strings.Repeat("0", 64), now.Add(time.Minute)); err != nil {
		t.Fatalf("new extractor profile could not append a document version for the same observation: %v", err)
	}
}

func TestEvidenceSnapshotUsesEndpointRightsAndExactDenyStillWins(t *testing.T) {
	runtime := openTestRuntime(t)
	defer func() { _ = runtime.Close() }()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%d", now.UnixNano())
	sourceID := insertEvidenceSource(t, runtime, "endpoint-inheritance-"+suffix)
	policyHash := strings.Repeat("a", 63) + "1"
	policyID := insertRightsPolicy(t, runtime, sourceID, "endpoint-inheritance", 1, policyHash, "approved endpoint contract", now.Add(-time.Hour))
	endpointDecision := rightsDecisionFixture{
		SourceID: sourceID, PolicyID: policyID, PolicyRevision: 1,
		PolicyScopeType: "source_endpoint", PolicySubject: "endpoint-inheritance", PriorityRank: 300,
		PolicyBasis: "approved endpoint contract", SubjectType: "source_endpoint", SubjectKey: strconv.FormatInt(sourceID, 10),
		InputDigest: policyHash, Action: "store_raw", Decision: "allow", EvaluatedAt: now, EffectiveFrom: now.Add(-time.Minute),
	}
	storeDecisionID, err := insertRightsDecision(runtime, endpointDecision)
	if err != nil {
		t.Fatalf("insert endpoint store_raw decision: %v", err)
	}
	endpointDecision.Action = "retain"
	endpointDecision.RetentionDays = intPointer(30)
	retainDecisionID, err := insertRightsDecision(runtime, endpointDecision)
	if err != nil {
		t.Fatalf("insert endpoint retain decision: %v", err)
	}
	snapshotKey := strings.Repeat("c", 63) + "1"
	payloadDigest := strings.Repeat("d", 63) + "1"
	var snapshotID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO evidence_snapshots (
  source_connection_id,store_raw_rights_decision_id,retain_rights_decision_id,
  snapshot_key,object_key,payload_sha256,collector_profile_version,mime_type,size_bytes,response_status,
  requested_url,final_url,captured_at,retention_until
) VALUES ($1,$2,$3,$4,$5,$6,'rss-http-v1','application/atom+xml',1,200,
  'https://feed.example.test/endpoint','https://feed.example.test/endpoint',$7,$8)
RETURNING id`, sourceID, storeDecisionID, retainDecisionID, snapshotKey,
		"source-raw/v1/endpoint-"+suffix, payloadDigest, now, now.Add(30*24*time.Hour)).Scan(&snapshotID); err != nil {
		t.Fatalf("endpoint rights did not authorize exact snapshot: %v", err)
	}

	// An observation-scoped policy has higher priority than an endpoint contract.
	highHash := strings.Repeat("e", 64)
	actorID := insertEvidenceRightsFixtureActor(t, runtime, highHash)
	key, fingerprint := evidenceRightsFixtureReceipt("policy", highHash)
	var exactPolicyID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_rights_policies (
 recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
 scope_type,scope_subject,policy_revision,priority,basis_summary,policy_hash,effective_at
) VALUES ($1,$1,$2,$3,$4,'observation',$5,3,400,'exact item restriction',$6,$7) RETURNING id`,
		actorID, key, fingerprint, sourceID, snapshotKey, highHash, now.Add(-time.Hour)).Scan(&exactPolicyID); err != nil {
		t.Fatalf("insert exact deny policy: %v", err)
	}
	exactDeny := rightsDecisionFixture{
		SourceID: sourceID, PolicyID: exactPolicyID, PolicyRevision: 3,
		PolicyScopeType: "observation", PolicySubject: snapshotKey, PriorityRank: 400,
		PolicyBasis: "exact item restriction", SubjectType: "raw_response", SubjectKey: snapshotKey,
		InputDigest: payloadDigest, Action: "store_raw", Decision: "deny", EvaluatedAt: now, EffectiveFrom: now.Add(-time.Minute),
	}
	if _, err := insertRightsDecision(runtime, exactDeny); err != nil {
		t.Fatalf("insert exact store_raw deny: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE evidence_snapshots SET lifecycle_state='raw_available',available_at=$2,updated_at=$2 WHERE id=$1`, snapshotID, now.Add(time.Second)); err == nil {
		t.Fatal("higher-priority exact deny did not block endpoint-authorized lifecycle commit")
	} else {
		assertPostgreSQLState(t, err, "23514")
	}
}

type rightsDecisionFixture struct {
	SourceID, PolicyID, PolicyRevision int64
	PolicyScopeType                    string
	PolicySubject, PolicyBasis         string
	PriorityRank                       int
	SubjectType, SubjectKey            string
	InputDigest, Action, Decision      string
	EvaluatedAt, EffectiveFrom         time.Time
	ExpiresAt                          *time.Time
	RetentionDays                      *int
	SupersedesDecisionID               *int64
}

func insertEvidenceSource(t *testing.T, runtime *Runtime, name string) int64 {
	t.Helper()
	var sourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', $1, 'https://feed.example.test/evidence-lineage') RETURNING id`, name).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	return sourceID
}

func insertRightsPolicy(t *testing.T, runtime *Runtime, sourceID int64, subject string, revision int64, hash, basis string, effectiveAt time.Time) int64 {
	t.Helper()
	actorID := insertEvidenceRightsFixtureActor(t, runtime, hash)
	idempotencyKey, commandFingerprint := evidenceRightsFixtureReceipt("policy", hash)
	var policyID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id, approved_by_user_id, idempotency_key, command_fingerprint,
  source_connection_id, scope_type, scope_subject, policy_revision, priority,
  basis_summary, policy_hash, effective_at
) VALUES ($1, $1, $2, $3, $4, 'source_endpoint', $5, $6, 300, $7, $8, $9)
RETURNING id`, actorID, idempotencyKey, commandFingerprint, sourceID, subject, revision,
		basis, hash, effectiveAt).Scan(&policyID); err != nil {
		t.Fatalf("insert rights policy revision %d: %v", revision, err)
	}
	return policyID
}

func insertRightsDecision(runtime *Runtime, fixture rightsDecisionFixture) (int64, error) {
	expiresValue := ""
	if fixture.ExpiresAt != nil {
		expiresValue = fixture.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	retentionValue := ""
	if fixture.RetentionDays != nil {
		retentionValue = fmt.Sprint(*fixture.RetentionDays)
	}
	supersedesValue := ""
	if fixture.SupersedesDecisionID != nil {
		supersedesValue = fmt.Sprint(*fixture.SupersedesDecisionID)
	}
	idempotencyKey, commandFingerprint := evidenceRightsFixtureReceipt(
		"decision", fmt.Sprint(fixture.SourceID), fmt.Sprint(fixture.PolicyID), fmt.Sprint(fixture.PolicyRevision),
		fixture.SubjectType, fixture.SubjectKey, fixture.InputDigest, fixture.Action, fixture.Decision,
		fixture.EvaluatedAt.UTC().Format(time.RFC3339Nano), fixture.EffectiveFrom.UTC().Format(time.RFC3339Nano),
		expiresValue, retentionValue, supersedesValue,
	)
	var decisionID int64
	err := runtime.SQL.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id, policy_id, expected_policy_version, subject_type, subject_key, input_digest,
    recorded_by_user_id, idempotency_key, command_fingerprint, decision_count
  )
  SELECT $1, $2, policy.version, $8, $9, $10, policy.recorded_by_user_id, $18, $19, 1
  FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id, source_connection_id, policy_id, policy_revision, policy_scope_type,
	  policy_scope_subject, priority_rank, basis_summary, subject_type, subject_key,
	  input_digest, action, decision, evaluator, evaluated_at, effective_from,
	  expires_at, retention_days, supersedes_decision_id
) SELECT decision_batch.id, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
	  'policy-v1', $13, $14, $15, $16, $17
	FROM decision_batch RETURNING id`, fixture.SourceID, fixture.PolicyID, fixture.PolicyRevision,
		fixture.PolicyScopeType, fixture.PolicySubject, fixture.PriorityRank, fixture.PolicyBasis,
		fixture.SubjectType, fixture.SubjectKey, fixture.InputDigest, fixture.Action, fixture.Decision, fixture.EvaluatedAt,
		fixture.EffectiveFrom, fixture.ExpiresAt, fixture.RetentionDays,
		fixture.SupersedesDecisionID, idempotencyKey, commandFingerprint).Scan(&decisionID)
	return decisionID, err
}

func insertEvidenceRightsFixtureActor(t *testing.T, runtime *Runtime, seed string) int64 {
	t.Helper()
	digest := evidenceRightsFixtureDigest("actor", seed)
	var actorID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role)
VALUES ($1,'fixture-not-a-credential','Evidence rights fixture operator','admin')
RETURNING id`, "evidence-rights-fixture-"+digest[:24]+"@example.test").Scan(&actorID); err != nil {
		t.Fatalf("insert evidence rights fixture actor: %v", err)
	}
	return actorID
}

func evidenceRightsFixtureReceipt(kind string, values ...string) (string, string) {
	fingerprint := evidenceRightsFixtureDigest(append([]string{kind}, values...)...)
	return "fixture." + kind + "." + fingerprint[:32], fingerprint
}

func evidenceRightsFixtureDigest(values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return fmt.Sprintf("%x", digest[:])
}

func intPointer(value int) *int { return &value }
