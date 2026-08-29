package postgres_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestRightsDecisionReaderSelectsHighestConflictFreeAndConservativeAllows(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()

	connection := sourceConnection("rights-decision-reader")
	if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	endpointPolicy := insertRightsReaderPolicy(t, runtime, connection.ID, "source_endpoint", "feed-reader", 1, 300, now.Add(-time.Hour))
	secondEndpointPolicy := insertRightsReaderPolicy(t, runtime, connection.ID, "source_endpoint", "feed-reader", 2, 300, now.Add(-time.Hour))
	observationPolicy := insertRightsReaderPolicy(t, runtime, connection.ID, "observation", "blocked-item", 1, 400, now.Add(-time.Hour))

	allowed := sourceapplication.RawEvidenceRightsSubjectDTO{EvidenceKey: rightsReaderDigest("allowed-key"), PayloadSHA256: rightsReaderDigest("allowed-payload")}
	conflicted := sourceapplication.RawEvidenceRightsSubjectDTO{EvidenceKey: rightsReaderDigest("conflicted-key"), PayloadSHA256: rightsReaderDigest("conflicted-payload")}
	blocked := sourceapplication.RawEvidenceRightsSubjectDTO{EvidenceKey: rightsReaderDigest("blocked-key"), PayloadSHA256: rightsReaderDigest("blocked-payload")}

	insertRightsReaderDecision(t, runtime, connection.ID, endpointPolicy, allowed, "store_raw", "allow", nil, now.Add(-time.Minute))
	retainThirtyID := insertRightsReaderDecision(t, runtime, connection.ID, endpointPolicy, allowed, "retain", "allow", rightsReaderIntPointer(30), now.Add(-time.Minute))
	retainTenID := insertRightsReaderDecision(t, runtime, connection.ID, secondEndpointPolicy, allowed, "retain", "allow", rightsReaderIntPointer(10), now.Add(-time.Minute))

	insertRightsReaderDecision(t, runtime, connection.ID, endpointPolicy, conflicted, "store_raw", "allow", nil, now.Add(-time.Minute))
	insertRightsReaderDecision(t, runtime, connection.ID, secondEndpointPolicy, conflicted, "store_raw", "unknown", nil, now.Add(-time.Minute))
	insertRightsReaderDecision(t, runtime, connection.ID, endpointPolicy, conflicted, "retain", "allow", rightsReaderIntPointer(30), now.Add(-time.Minute))

	insertRightsReaderDecision(t, runtime, connection.ID, endpointPolicy, blocked, "store_raw", "allow", nil, now.Add(-time.Minute))
	insertRightsReaderDecision(t, runtime, connection.ID, endpointPolicy, blocked, "retain", "allow", rightsReaderIntPointer(30), now.Add(-time.Minute))
	insertRightsReaderDecision(t, runtime, connection.ID, observationPolicy, blocked, "store_raw", "deny", nil, now.Add(-time.Minute))

	result, err := sourcepostgres.NewRightsDecisionReader(runtime).ResolveCurrent(context.Background(), sourceapplication.CurrentRawEvidenceRightsQuery{
		SourceConnectionID: connection.ID, DecisionAt: now,
		Subjects: []sourceapplication.RawEvidenceRightsSubjectDTO{allowed, conflicted, blocked},
	})
	if err != nil {
		t.Fatalf("ResolveCurrent() error = %v", err)
	}
	if _, found := result.StoreRawDecisions[allowed.EvidenceKey]; !found {
		t.Fatal("conflict-free store_raw allow was not selected")
	}
	if selected, found := result.RetainDecisions[allowed.EvidenceKey]; !found || selected.ID != retainTenID || selected.RetentionDays == nil || *selected.RetentionDays != 10 {
		t.Fatalf("selected conservative retain = %#v, want decision %d / 10 days (not %d)", selected, retainTenID, retainThirtyID)
	}
	if _, found := result.StoreRawDecisions[conflicted.EvidenceKey]; found {
		t.Fatal("same-priority allow/unknown conflict returned store_raw allow")
	}
	if _, found := result.RetainDecisions[conflicted.EvidenceKey]; !found {
		t.Fatal("independent conflict-free retain allow was not selected")
	}
	if _, found := result.StoreRawDecisions[blocked.EvidenceKey]; found {
		t.Fatal("higher-priority deny returned lower-priority store_raw allow")
	}
	if _, found := result.RetainDecisions[blocked.EvidenceKey]; !found {
		t.Fatal("higher-priority store_raw deny incorrectly removed retain allow")
	}
}

func TestRightsDecisionReaderAppliesEndpointPolicyAndExactDenyPrecedence(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()

	connection := sourceConnection("endpoint-rights-decision-reader")
	if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	endpointPolicy := insertRightsReaderPolicy(t, runtime, connection.ID, "source_endpoint", "feed-reader", 1, 300, now.Add(-time.Hour))
	observationPolicy := insertRightsReaderPolicy(t, runtime, connection.ID, "observation", "blocked-entry", 1, 400, now.Add(-time.Hour))
	retentionDays := 30
	storeID := insertRightsReaderEndpointDecision(t, runtime, connection.ID, endpointPolicy, "store_raw", "allow", nil, now.Add(-time.Minute))
	insertRightsReaderEndpointDecision(t, runtime, connection.ID, endpointPolicy, "retain", "allow", &retentionDays, now.Add(-time.Minute))

	allowed := sourceapplication.RawEvidenceRightsSubjectDTO{EvidenceKey: rightsReaderDigest("endpoint-allowed"), PayloadSHA256: rightsReaderDigest("endpoint-allowed-payload")}
	blocked := sourceapplication.RawEvidenceRightsSubjectDTO{EvidenceKey: rightsReaderDigest("endpoint-blocked"), PayloadSHA256: rightsReaderDigest("endpoint-blocked-payload")}
	insertRightsReaderDecision(t, runtime, connection.ID, observationPolicy, blocked, "store_raw", "deny", nil, now.Add(-time.Minute))

	result, err := sourcepostgres.NewRightsDecisionReader(runtime).ResolveCurrent(context.Background(), sourceapplication.CurrentRawEvidenceRightsQuery{
		SourceConnectionID: connection.ID,
		DecisionAt:         now,
		Subjects:           []sourceapplication.RawEvidenceRightsSubjectDTO{allowed, blocked},
	})
	if err != nil {
		t.Fatalf("ResolveCurrent() error = %v", err)
	}
	selected, found := result.StoreRawDecisions[allowed.EvidenceKey]
	if !found || selected.ID != storeID || selected.SubjectType != "source_endpoint" ||
		selected.AuthorizedEvidenceKey != allowed.EvidenceKey || selected.AuthorizedPayloadSHA256 != allowed.PayloadSHA256 {
		t.Fatalf("endpoint authorization = %#v", selected)
	}
	if _, found := result.RetainDecisions[allowed.EvidenceKey]; !found {
		t.Fatal("endpoint retain allow was not inherited")
	}
	if _, found := result.StoreRawDecisions[blocked.EvidenceKey]; found {
		t.Fatal("higher-priority exact deny did not override endpoint allow")
	}
	if _, found := result.RetainDecisions[blocked.EvidenceKey]; !found {
		t.Fatal("independent endpoint retain allow was removed")
	}
}

func TestRightsDecisionReaderResolvesCurrentEndpointFetchConservatively(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()

	connection := sourceConnection("endpoint-fetch-rights-reader")
	if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	firstPolicy := insertRightsReaderPolicy(t, runtime, connection.ID, "source_endpoint", "feed-reader", 1, 300, now.Add(-time.Hour))
	secondPolicy := insertRightsReaderPolicy(t, runtime, connection.ID, "source_endpoint", "feed-reader", 2, 300, now.Add(-time.Hour))
	allowID := insertRightsReaderEndpointDecision(t, runtime, connection.ID, firstPolicy, "fetch", "allow", nil, now.Add(-3*time.Minute))
	unknownID := insertRightsReaderEndpointDecision(t, runtime, connection.ID, secondPolicy, "fetch", "unknown", nil, now.Add(-time.Minute))

	reader := sourcepostgres.NewRightsDecisionReader(runtime)
	result, err := reader.ResolveCurrentFetch(context.Background(), sourceapplication.CurrentCollectionFetchRightsQuery{
		SourceConnectionID: connection.ID,
		DecisionAt:         now,
	})
	if err != nil {
		t.Fatalf("ResolveCurrentFetch(): %v", err)
	}
	if result.Decision != "deny" || !result.EvaluatedAt.Equal(now) || len(result.DecisionIDs) != 2 || len(result.PolicyIDs) != 2 {
		t.Fatalf("conflicting endpoint fetch result = %#v", result)
	}
	if result.DecisionIDs[0] != allowID || result.DecisionIDs[1] != unknownID {
		t.Fatalf("decision receipts = %v", result.DecisionIDs)
	}

	allowedAt := now.Add(-2 * time.Minute)
	result, err = reader.ResolveCurrentFetch(context.Background(), sourceapplication.CurrentCollectionFetchRightsQuery{
		SourceConnectionID: connection.ID,
		DecisionAt:         allowedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != "allow" || len(result.DecisionIDs) != 1 || result.DecisionIDs[0] != allowID {
		t.Fatalf("explicit allow result = %#v", result)
	}

	missing, err := reader.ResolveCurrentFetch(context.Background(), sourceapplication.CurrentCollectionFetchRightsQuery{
		SourceConnectionID: connection.ID,
		DecisionAt:         now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Decision != "unknown" || len(missing.DecisionIDs) != 0 || len(missing.PolicyIDs) != 0 {
		t.Fatalf("missing endpoint fetch result = %#v", missing)
	}
}

type rightsReaderPolicyFixture struct {
	ID           int64
	Revision     int64
	ScopeType    string
	ScopeSubject string
	Priority     int
	Basis        string
	PolicyHash   string
}

func insertRightsReaderPolicy(t *testing.T, runtime *database.Runtime, sourceID int64, scopeType, scopeSubject string, revision int64, priority int, effectiveAt time.Time) rightsReaderPolicyFixture {
	t.Helper()
	fixture := rightsReaderPolicyFixture{Revision: revision, ScopeType: scopeType, ScopeSubject: scopeSubject, Priority: priority, Basis: "rights reader fixture"}
	policyHash := rightsReaderDigest(fmt.Sprintf("%d:%s:%s:%d", sourceID, scopeType, scopeSubject, revision))
	fixture.PolicyHash = policyHash
	actorID := insertRightsFixtureActor(t, runtime.SQL, policyHash)
	idempotencyKey, commandFingerprint := rightsFixtureReceipt("policy", policyHash)
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
  scope_type,scope_subject,policy_revision,priority,basis_summary,policy_hash,effective_at
) VALUES ($1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING id`, actorID, idempotencyKey, commandFingerprint, sourceID, scopeType, scopeSubject,
		revision, priority, fixture.Basis, policyHash, effectiveAt).Scan(&fixture.ID); err != nil {
		t.Fatalf("insert rights reader policy: %v", err)
	}
	return fixture
}

func insertRightsReaderEndpointDecision(t *testing.T, runtime *database.Runtime, sourceID int64, policy rightsReaderPolicyFixture, action, decision string, retentionDays *int, effectiveFrom time.Time) int64 {
	t.Helper()
	idempotencyKey, commandFingerprint := rightsFixtureReceipt(
		"endpoint-decision", sourceID, policy.ID, policy.Revision, action, decision, effectiveFrom.UTC().Format(time.RFC3339Nano),
	)
	var id int64
	if err := runtime.SQL.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  ) SELECT $1::bigint,$2,policy.version,'source_endpoint',($1::bigint)::text,policy.policy_hash,
           policy.recorded_by_user_id,$10,$11,1
    FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,
  reason_codes,evaluator,evaluated_at,effective_from,retention_days
) SELECT decision_batch.id,$1::bigint,$2,$3,$4,$5,$6,$7,'source_endpoint',($1::bigint)::text,$8,$9,$12,
         ARRAY['approved_endpoint_policy'],'rights-reader-test',$13,$13,$14
  FROM decision_batch RETURNING id`, sourceID, policy.ID, policy.Revision, policy.ScopeType, policy.ScopeSubject,
		policy.Priority, policy.Basis, policy.PolicyHash, action, idempotencyKey, commandFingerprint, decision,
		effectiveFrom, retentionDays).Scan(&id); err != nil {
		t.Fatalf("insert endpoint rights reader decision %s/%s: %v", action, decision, err)
	}
	return id
}

func insertRightsReaderDecision(t *testing.T, runtime *database.Runtime, sourceID int64, policy rightsReaderPolicyFixture, subject sourceapplication.RawEvidenceRightsSubjectDTO, action, decision string, retentionDays *int, effectiveFrom time.Time) int64 {
	t.Helper()
	retentionValue := ""
	if retentionDays != nil {
		retentionValue = fmt.Sprint(*retentionDays)
	}
	idempotencyKey, commandFingerprint := rightsFixtureReceipt(
		"decision", sourceID, policy.ID, policy.Revision, subject.EvidenceKey, subject.PayloadSHA256,
		action, decision, retentionValue, effectiveFrom.UTC().Format(time.RFC3339Nano),
	)
	var id int64
	if err := runtime.SQL.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  )
  SELECT $1,$2,policy.version,'raw_response',$8,$9,policy.recorded_by_user_id,$16,$17,1
  FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,
  reason_codes,evaluator,evaluated_at,effective_from,retention_days
) SELECT decision_batch.id,$1,$2,$3,$4,$5,$6,$7,'raw_response',$8,$9,$10,$11,
          ARRAY['contract_fixture'],$12,$13,$14,$15
FROM decision_batch RETURNING id`, sourceID, policy.ID, policy.Revision, policy.ScopeType, policy.ScopeSubject,
		policy.Priority, policy.Basis, subject.EvidenceKey, subject.PayloadSHA256, action, decision,
		"rights-reader-test", effectiveFrom, effectiveFrom, retentionDays, idempotencyKey, commandFingerprint).Scan(&id); err != nil {
		t.Fatalf("insert rights reader decision %s/%s: %v", action, decision, err)
	}
	return id
}

func rightsReaderDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:])
}

func rightsReaderIntPointer(value int) *int { return &value }
