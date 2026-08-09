package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestRightsManagementRepositoryCreatesPolicyWithDurableIdempotencyAndDistinctRevision(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	ctx := context.Background()

	sourceID := insertRightsManagementSource(t, runtime.SQL, "policy-idempotency")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "policy-idempotency")
	request := rightsManagementPolicyRequest(sourceID, actorID, "policy.repository.idempotency", strings.Repeat("a", 64), 7)

	first, err := repository.CreateRightsPolicy(ctx, request)
	if err != nil {
		t.Fatalf("CreateRightsPolicy(first): %v", err)
	}
	replayed, err := repository.CreateRightsPolicy(ctx, request)
	if err != nil {
		t.Fatalf("CreateRightsPolicy(replay): %v", err)
	}
	if first.IdempotentReplay || !replayed.IdempotentReplay || first.Policy.ID <= 0 || first.Policy.ID != replayed.Policy.ID {
		t.Fatalf("policy receipts = first:%#v replay:%#v", first, replayed)
	}
	if first.Policy.Version != 1 || first.Policy.Revision != 7 || first.Policy.Version == first.Policy.Revision {
		t.Fatalf("row version and policy revision were conflated: %#v", first.Policy)
	}

	loaded, err := repository.FindRightsPolicy(ctx, sourceapplication.FindRightsPolicyQueryDTO{
		PolicyID: first.Policy.ID, ExpectedVersion: first.Policy.Version,
	})
	if err != nil || loaded.ID != first.Policy.ID || loaded.Revision != 7 {
		t.Fatalf("FindRightsPolicy(exact version) = %#v, %v", loaded, err)
	}
	if _, err := repository.FindRightsPolicy(ctx, sourceapplication.FindRightsPolicyQueryDTO{
		PolicyID: first.Policy.ID, ExpectedVersion: first.Policy.Revision,
	}); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("FindRightsPolicy(revision as row version) error = %v", err)
	}

	conflict := request
	conflict.CommandFingerprint = strings.Repeat("b", 64)
	conflict.BasisSummary = "different immutable policy input"
	if _, err := repository.CreateRightsPolicy(ctx, conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("same policy key with different fingerprint error = %v", err)
	}

	var recordedActor int64
	var idempotencyKey, fingerprint string
	if err := runtime.SQL.QueryRow(`
SELECT recorded_by_user_id,idempotency_key,command_fingerprint
FROM source_rights_policies WHERE id=$1`, first.Policy.ID).Scan(&recordedActor, &idempotencyKey, &fingerprint); err != nil {
		t.Fatalf("read policy command receipt: %v", err)
	}
	if recordedActor != actorID || idempotencyKey != request.IdempotencyKey || fingerprint != request.CommandFingerprint {
		t.Fatalf("policy command receipt = actor:%d key:%q fingerprint:%q", recordedActor, idempotencyKey, fingerprint)
	}
}

func TestRightsManagementRepositoryRejectsUnauthorizedPolicyActorsAtDatabaseBoundary(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	ctx := context.Background()

	sourceID := insertRightsManagementSource(t, runtime.SQL, "policy-actor-authority")
	activeAdminID := insertRightsFixtureActor(t, runtime.SQL, "policy-active-admin")
	var editorID, disabledAdminID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role)
VALUES ('rights-editor@example.test','fixture-not-a-credential','Rights editor','editor')
RETURNING id`).Scan(&editorID); err != nil {
		t.Fatalf("insert rights editor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role,status)
VALUES ('rights-disabled-admin@example.test','fixture-not-a-credential','Disabled rights administrator','admin','disabled')
RETURNING id`).Scan(&disabledAdminID); err != nil {
		t.Fatalf("insert disabled rights administrator: %v", err)
	}

	for _, scenario := range []struct {
		name       string
		recorderID int64
		approverID int64
	}{
		{name: "editor recorder", recorderID: editorID, approverID: activeAdminID},
		{name: "disabled administrator recorder", recorderID: disabledAdminID, approverID: activeAdminID},
		{name: "editor approver", recorderID: activeAdminID, approverID: editorID},
		{name: "disabled administrator approver", recorderID: activeAdminID, approverID: disabledAdminID},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			key := "policy.repository.actor." + strings.ReplaceAll(scenario.name, " ", "-")
			request := rightsManagementPolicyRequest(sourceID, scenario.recorderID, key, rightsFixtureDigest("fingerprint", key), 31)
			request.PolicyHash = rightsFixtureDigest("policy", key)
			request.ApprovedByUserID = &scenario.approverID
			if _, err := repository.CreateRightsPolicy(ctx, request); !errors.Is(err, sharedrepository.ErrConstraint) {
				t.Fatalf("CreateRightsPolicy() error = %v, want constraint", err)
			}
		})
	}
}

func TestRightsManagementRepositoryRecordsAtomicDecisionBatchAndStableReplay(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	ctx := context.Background()

	sourceID := insertRightsManagementSource(t, runtime.SQL, "decision-batch")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "decision-batch")
	policyResult, err := repository.CreateRightsPolicy(ctx, rightsManagementPolicyRequest(
		sourceID, actorID, "policy.repository.batch", strings.Repeat("c", 64), 11,
	))
	if err != nil {
		t.Fatalf("create decision policy: %v", err)
	}
	request := rightsManagementDecisionRequest(sourceID, actorID, policyResult.Policy, "decision.repository.batch", strings.Repeat("d", 64))

	first, err := repository.RecordRightsDecisions(ctx, request)
	if err != nil {
		t.Fatalf("RecordRightsDecisions(first): %v", err)
	}
	replayed, err := repository.RecordRightsDecisions(ctx, request)
	if err != nil {
		t.Fatalf("RecordRightsDecisions(replay): %v", err)
	}
	if first.IdempotentReplay || !replayed.IdempotentReplay || first.DecisionBatchID <= 0 ||
		first.DecisionBatchID != replayed.DecisionBatchID || !reflect.DeepEqual(rightsManagementDecisionIDs(first.Decisions), rightsManagementDecisionIDs(replayed.Decisions)) {
		t.Fatalf("decision receipts = first:%#v replay:%#v", first, replayed)
	}
	if len(first.Decisions) != len(request.Decisions) {
		t.Fatalf("decision count = %d, want %d", len(first.Decisions), len(request.Decisions))
	}
	for _, decision := range first.Decisions {
		if decision.DecisionBatchID != first.DecisionBatchID || decision.PolicyRevision != policyResult.Policy.Revision ||
			decision.PolicyRevision == policyResult.Policy.Version || decision.SourceConnectionID != sourceID ||
			decision.PolicyScopeType != policyResult.Policy.ScopeType || decision.PolicyScopeSubject != policyResult.Policy.ScopeSubject ||
			decision.Priority != policyResult.Policy.Priority || decision.BasisSummary != policyResult.Policy.BasisSummary ||
			decision.TermsURL != policyResult.Policy.TermsURL || decision.LicenseURI != policyResult.Policy.LicenseURI {
			t.Fatalf("decision batch/policy snapshot = %#v", decision)
		}
	}
	loadedDecision, err := repository.FindRightsDecision(ctx, first.Decisions[0].ID)
	if err != nil || loadedDecision.ID != first.Decisions[0].ID || loadedDecision.DecisionBatchID != first.DecisionBatchID {
		t.Fatalf("FindRightsDecision() = %#v, %v", loadedDecision, err)
	}

	var recordedActor, persistedExpectedVersion int64
	var declaredCount, persistedCount int
	if err := runtime.SQL.QueryRow(`
SELECT batch.recorded_by_user_id,batch.expected_policy_version,batch.decision_count,count(decision.id)
FROM source_rights_decision_batches AS batch
LEFT JOIN source_rights_decisions AS decision ON decision.decision_batch_id=batch.id
WHERE batch.id=$1
GROUP BY batch.id`, first.DecisionBatchID).Scan(&recordedActor, &persistedExpectedVersion, &declaredCount, &persistedCount); err != nil {
		t.Fatalf("read decision batch receipt: %v", err)
	}
	if recordedActor != actorID || persistedExpectedVersion != policyResult.Policy.Version || declaredCount != 2 || persistedCount != 2 {
		t.Fatalf("batch receipt = actor:%d expectedVersion:%d declared:%d persisted:%d", recordedActor, persistedExpectedVersion, declaredCount, persistedCount)
	}

	conflict := request
	conflict.CommandFingerprint = strings.Repeat("e", 64)
	conflict.Decisions = append([]sourceapplication.RightsActionDecisionDTO(nil), request.Decisions...)
	conflict.Decisions[0].Evaluator = "different-evaluator"
	if _, err := repository.RecordRightsDecisions(ctx, conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("same decision key with different fingerprint error = %v", err)
	}

	atomic := request
	atomic.IdempotencyKey = "decision.repository.atomic-failure"
	atomic.CommandFingerprint = strings.Repeat("f", 64)
	atomic.SubjectKey = rightsFixtureDigest("atomic-subject")
	atomic.InputDigest = rightsFixtureDigest("atomic-input")
	days := 9
	atomic.Decisions = []sourceapplication.RightsActionDecisionDTO{
		{Action: "fetch", Decision: "allow", ReasonCodes: []string{"approved"}, Evaluator: "rights-repository-test", EvaluatedAt: request.Decisions[0].EvaluatedAt, EffectiveFrom: request.Decisions[0].EffectiveFrom},
		{Action: "quote", Decision: "allow", ReasonCodes: []string{"invalid_retention"}, Evaluator: "rights-repository-test", EvaluatedAt: request.Decisions[0].EvaluatedAt, EffectiveFrom: request.Decisions[0].EffectiveFrom, RetentionDays: &days},
	}
	if _, err := repository.RecordRightsDecisions(ctx, atomic); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("invalid second decision error = %v", err)
	}
	var batchCount, decisionCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_rights_decision_batches WHERE idempotency_key=$1`, atomic.IdempotencyKey).Scan(&batchCount); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_rights_decisions WHERE subject_key=$1`, atomic.SubjectKey).Scan(&decisionCount); err != nil {
		t.Fatal(err)
	}
	if batchCount != 0 || decisionCount != 0 {
		t.Fatalf("failed atomic batch leaked facts: batches=%d decisions=%d", batchCount, decisionCount)
	}
}

func TestRightsManagementRepositoryRejectsDecisionFromInactiveRecorderAtDatabaseBoundary(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	ctx := context.Background()

	sourceID := insertRightsManagementSource(t, runtime.SQL, "decision-actor-authority")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "decision-actor-authority")
	policyResult, err := repository.CreateRightsPolicy(ctx, rightsManagementPolicyRequest(
		sourceID, actorID, "policy.repository.decision-actor", rightsFixtureDigest("fingerprint", "decision-actor-policy"), 13,
	))
	if err != nil {
		t.Fatalf("create approved rights policy: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE users SET status='disabled' WHERE id=$1`, actorID); err != nil {
		t.Fatalf("disable rights decision recorder: %v", err)
	}
	request := rightsManagementDecisionRequest(sourceID, actorID, policyResult.Policy, "decision.repository.inactive-actor", rightsFixtureDigest("fingerprint", "inactive-decision-actor"))
	if _, err := repository.RecordRightsDecisions(ctx, request); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("RecordRightsDecisions() error = %v, want constraint", err)
	}
}

func TestRightsManagementRepositorySerializesConcurrentIdempotentDecisionWriters(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	ctx := context.Background()

	sourceID := insertRightsManagementSource(t, runtime.SQL, "decision-concurrency")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "decision-concurrency")
	policyResult, err := repository.CreateRightsPolicy(ctx, rightsManagementPolicyRequest(
		sourceID, actorID, "policy.repository.concurrency", strings.Repeat("1", 64), 17,
	))
	if err != nil {
		t.Fatal(err)
	}
	request := rightsManagementDecisionRequest(sourceID, actorID, policyResult.Policy, "decision.repository.concurrency", strings.Repeat("2", 64))

	const writers = 8
	results := make(chan sourceapplication.RecordRightsDecisionRepositoryResultDTO, writers)
	errorsChannel := make(chan error, writers)
	var group sync.WaitGroup
	group.Add(writers)
	for index := 0; index < writers; index++ {
		go func() {
			defer group.Done()
			result, err := repository.RecordRightsDecisions(ctx, request)
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("concurrent RecordRightsDecisions(): %v", err)
	}

	var batchID int64
	var expectedIDs []int64
	firstWrites := 0
	for result := range results {
		if !result.IdempotentReplay {
			firstWrites++
		}
		ids := rightsManagementDecisionIDs(result.Decisions)
		if batchID == 0 {
			batchID, expectedIDs = result.DecisionBatchID, ids
			continue
		}
		if result.DecisionBatchID != batchID || !reflect.DeepEqual(ids, expectedIDs) {
			t.Fatalf("concurrent receipt drifted: batch=%d ids=%v, want batch=%d ids=%v", result.DecisionBatchID, ids, batchID, expectedIDs)
		}
	}
	if firstWrites != 1 {
		t.Fatalf("first-write results = %d, want 1", firstWrites)
	}
	var batches, decisions int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_rights_decision_batches WHERE idempotency_key=$1`, request.IdempotencyKey).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_rights_decisions WHERE decision_batch_id=$1`, batchID).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if batches != 1 || decisions != len(request.Decisions) {
		t.Fatalf("concurrent persisted facts = batches:%d decisions:%d", batches, decisions)
	}
}

func TestRightsManagementTransactionAdapterReusesRuntimeTransaction(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	transactions := sourcepostgres.NewRightsManagementTransactionAdapter(runtime)
	sourceID := insertRightsManagementSource(t, runtime.SQL, "transaction-reuse")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "transaction-reuse")
	request := rightsManagementPolicyRequest(sourceID, actorID, "policy.repository.rollback", strings.Repeat("3", 64), 23)
	rollback := errors.New("rollback rights transaction")

	err := transactions.WithinRightsManagementTransaction(context.Background(), func(ctx context.Context) error {
		if _, err := repository.CreateRightsPolicy(ctx, request); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v", err)
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_rights_policies WHERE idempotency_key=$1`, request.IdempotencyKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled back policy count = %d", count)
	}
}

func insertRightsManagementSource(t *testing.T, runtime rightsFixtureQueryRower, label string) int64 {
	t.Helper()
	var sourceID int64
	if err := runtime.QueryRow(`
INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss',$1,$2) RETURNING id`, "rights-management-"+label, "https://rights.example.test/"+label).Scan(&sourceID); err != nil {
		t.Fatalf("insert rights management source: %v", err)
	}
	return sourceID
}

func rightsManagementPolicyRequest(sourceID, actorID int64, idempotencyKey, fingerprint string, revision int64) sourceapplication.CreateRightsPolicyRepositoryDTO {
	effectiveAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	return sourceapplication.CreateRightsPolicyRepositoryDTO{
		ActorID: actorID, IdempotencyKey: idempotencyKey, CommandFingerprint: fingerprint,
		SourceConnectionID: &sourceID, ScopeType: "source_endpoint", ScopeSubject: "feed-contract-" + idempotencyKey,
		Revision: revision, Priority: 300, BasisSummary: "approved repository policy",
		TermsURL: "https://publisher.example.test/terms", LicenseURI: "urn:license:rights-management",
		PolicyHash: rightsFixtureDigest("policy", idempotencyKey), EffectiveFrom: effectiveAt, ApprovedByUserID: &actorID,
	}
}

func rightsManagementDecisionRequest(sourceID, actorID int64, policy sourceapplication.RightsPolicyDTO, idempotencyKey, fingerprint string) sourceapplication.RecordRightsDecisionRepositoryDTO {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	days := 30
	return sourceapplication.RecordRightsDecisionRepositoryDTO{
		ActorID: actorID, IdempotencyKey: idempotencyKey, CommandFingerprint: fingerprint,
		SourceConnectionID: sourceID, ExpectedPolicyVersion: policy.Version, Policy: policy,
		SubjectType: "raw_response", SubjectKey: rightsFixtureDigest("subject", idempotencyKey), InputDigest: rightsFixtureDigest("input", idempotencyKey),
		Decisions: []sourceapplication.RightsActionDecisionDTO{
			{Action: "store_raw", Decision: "allow", ReasonCodes: []string{"terms_confirmed"}, Evaluator: "rights-repository-test", EvaluatedAt: now, EffectiveFrom: now},
			{Action: "retain", Decision: "allow", ReasonCodes: []string{"retention_approved"}, Evaluator: "rights-repository-test", EvaluatedAt: now, EffectiveFrom: now, RetentionDays: &days},
		},
	}
}

func rightsManagementDecisionIDs(decisions []sourceapplication.RightsDecisionDTO) []int64 {
	ids := make([]int64, len(decisions))
	for index, decision := range decisions {
		ids[index] = decision.ID
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}
