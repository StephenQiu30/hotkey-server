package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestRightsManagementProjectionReadsEndpointHistoryWithBoundCursors(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	ctx := context.Background()

	sourceID := insertRightsManagementSource(t, runtime.SQL, "projection-history")
	otherSourceID := insertRightsManagementSource(t, runtime.SQL, "projection-history-other")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "projection-history")

	firstPolicyRequest := rightsManagementPolicyRequest(sourceID, actorID, "policy.projection.history.first", rightsFixtureDigest("projection-policy-first"), 41)
	firstPolicy, err := repository.CreateRightsPolicy(ctx, firstPolicyRequest)
	if err != nil {
		t.Fatalf("create first projection policy: %v", err)
	}
	secondPolicyRequest := rightsManagementPolicyRequest(sourceID, actorID, "policy.projection.history.second", rightsFixtureDigest("projection-policy-second"), 42)
	secondPolicy, err := repository.CreateRightsPolicy(ctx, secondPolicyRequest)
	if err != nil {
		t.Fatalf("create second projection policy: %v", err)
	}
	otherPolicyRequest := rightsManagementPolicyRequest(otherSourceID, actorID, "policy.projection.history.other", rightsFixtureDigest("projection-policy-other"), 43)
	if _, err := repository.CreateRightsPolicy(ctx, otherPolicyRequest); err != nil {
		t.Fatalf("create other source policy: %v", err)
	}

	firstPage, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{SourceEndpointID: sourceID, Limit: 1})
	if err != nil {
		t.Fatalf("ListRightsPolicies(first): %v", err)
	}
	if len(firstPage.Items) != 1 || firstPage.Items[0].ID != secondPolicy.Policy.ID || firstPage.NextCursor == "" ||
		firstPage.Items[0].RecordedByUserID != actorID || firstPage.Items[0].CreatedAt.IsZero() {
		t.Fatalf("first policy page = %#v", firstPage)
	}
	tamperedCursor := "A" + firstPage.NextCursor[1:]
	if firstPage.NextCursor[0] == 'A' {
		tamperedCursor = "B" + firstPage.NextCursor[1:]
	}
	if _, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{
		SourceEndpointID: sourceID, Cursor: tamperedCursor, Limit: 1,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("tampered policy cursor error = %v", err)
	}
	concurrentPolicyRequest := rightsManagementPolicyRequest(sourceID, actorID, "policy.projection.history.concurrent", rightsFixtureDigest("projection-policy-concurrent"), 44)
	concurrentPolicy, err := repository.CreateRightsPolicy(ctx, concurrentPolicyRequest)
	if err != nil {
		t.Fatalf("create concurrent projection policy: %v", err)
	}
	secondPage, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{
		SourceEndpointID: sourceID, Cursor: firstPage.NextCursor, Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListRightsPolicies(second): %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].ID != firstPolicy.Policy.ID || secondPage.Items[0].ID == concurrentPolicy.Policy.ID {
		t.Fatalf("second policy page = %#v", secondPage)
	}
	freshPage, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{SourceEndpointID: sourceID, Limit: 1})
	if err != nil || len(freshPage.Items) != 1 || freshPage.Items[0].ID != concurrentPolicy.Policy.ID {
		t.Fatalf("fresh policy page after concurrent insert = %#v / %v", freshPage, err)
	}
	if _, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{
		SourceEndpointID: otherSourceID, Cursor: firstPage.NextCursor, Limit: 1,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("cross-endpoint policy cursor error = %v", err)
	}

	decisionRequest := rightsManagementDecisionRequest(sourceID, actorID, firstPolicy.Policy, "decision.projection.history", rightsFixtureDigest("projection-decision"))
	decisionBatch, err := repository.RecordRightsDecisions(ctx, decisionRequest)
	if err != nil {
		t.Fatalf("create projection decision batch: %v", err)
	}
	batchPage, err := repository.ListRightsDecisionBatches(ctx, sourceapplication.ListRightsDecisionBatchesRepositoryDTO{SourceEndpointID: sourceID, Limit: 10})
	if err != nil {
		t.Fatalf("ListRightsDecisionBatches(): %v", err)
	}
	if len(batchPage.Items) != 1 || batchPage.Items[0].ID != decisionBatch.DecisionBatchID ||
		batchPage.Items[0].DecisionCount != len(decisionRequest.Decisions) ||
		len(batchPage.Items[0].Decisions) != len(decisionRequest.Decisions) {
		t.Fatalf("decision batch page = %#v", batchPage)
	}
	loaded, err := repository.FindRightsDecisionRead(ctx, sourceapplication.FindRightsDecisionReadRepositoryDTO{
		SourceEndpointID: sourceID, DecisionID: decisionBatch.Decisions[0].ID,
	})
	if err != nil || loaded.ID != decisionBatch.Decisions[0].ID || loaded.RecordedByUserID != actorID || loaded.CreatedAt.IsZero() {
		t.Fatalf("FindRightsDecisionRead() = %#v, %v", loaded, err)
	}
	if _, err := repository.FindRightsDecisionRead(ctx, sourceapplication.FindRightsDecisionReadRepositoryDTO{
		SourceEndpointID: otherSourceID, DecisionID: decisionBatch.Decisions[0].ID,
	}); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("cross-endpoint decision detail error = %v", err)
	}
	if strings.Contains(loaded.BasisSummary, "object_key") {
		t.Fatalf("decision projection leaked an object address: %#v", loaded)
	}
}

func TestRightsManagementProjectionListCursorsAreSignedBoundExpiringAndStable(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	codec, err := pagination.NewCodec(strings.Repeat("rights-history-cursor-secret-", 2), time.Minute)
	if err != nil {
		t.Fatalf("pagination.NewCodec(): %v", err)
	}
	repository := sourcepostgres.NewRightsManagementRepositoryWithCursorCodec(runtime, codec)
	ctx := context.Background()
	sourceID := insertRightsManagementSource(t, runtime.SQL, "projection-cursor-matrix")
	otherSourceID := insertRightsManagementSource(t, runtime.SQL, "projection-cursor-matrix-other")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "projection-cursor-matrix")

	policies := make([]sourceapplication.RightsPolicyDTO, 0, 3)
	for index := 1; index <= 3; index++ {
		request := rightsManagementPolicyRequest(
			sourceID, actorID, fmt.Sprintf("policy.projection.cursor.%d", index), rightsFixtureDigest("projection-cursor-policy", fmt.Sprint(index)), int64(60+index),
		)
		stored, err := repository.CreateRightsPolicy(ctx, request)
		if err != nil {
			t.Fatalf("create cursor policy %d: %v", index, err)
		}
		policies = append(policies, stored.Policy)
	}
	policyFirst, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{SourceEndpointID: sourceID, Limit: 2})
	if err != nil || len(policyFirst.Items) != 2 || policyFirst.Items[0].ID != policies[2].ID ||
		policyFirst.Items[1].ID != policies[1].ID || policyFirst.NextCursor == "" || !strings.Contains(policyFirst.NextCursor, ".") {
		t.Fatalf("first rights policy cursor page = %#v / %v", policyFirst, err)
	}
	policyTampered := "A" + policyFirst.NextCursor[1:]
	if policyFirst.NextCursor[0] == 'A' {
		policyTampered = "B" + policyFirst.NextCursor[1:]
	}
	for name, query := range map[string]sourceapplication.ListRightsPoliciesRepositoryDTO{
		"tampered":       {SourceEndpointID: sourceID, Cursor: policyTampered, Limit: 2},
		"cross endpoint": {SourceEndpointID: otherSourceID, Cursor: policyFirst.NextCursor, Limit: 2},
	} {
		if _, err := repository.ListRightsPolicies(ctx, query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("%s rights policy cursor error = %v", name, err)
		}
	}
	concurrentPolicyRequest := rightsManagementPolicyRequest(
		sourceID, actorID, "policy.projection.cursor.concurrent", rightsFixtureDigest("projection-cursor-policy-concurrent"), 64,
	)
	concurrentPolicy, err := repository.CreateRightsPolicy(ctx, concurrentPolicyRequest)
	if err != nil {
		t.Fatalf("create concurrent cursor policy: %v", err)
	}
	policySecond, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{
		SourceEndpointID: sourceID, Cursor: policyFirst.NextCursor, Limit: 2,
	})
	if err != nil || len(policySecond.Items) != 1 || policySecond.Items[0].ID != policies[0].ID || policySecond.NextCursor != "" {
		t.Fatalf("second rights policy cursor page = %#v / %v", policySecond, err)
	}
	policyFresh, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{SourceEndpointID: sourceID, Limit: 1})
	if err != nil || len(policyFresh.Items) != 1 || policyFresh.Items[0].ID != concurrentPolicy.Policy.ID {
		t.Fatalf("fresh rights policy cursor page = %#v / %v", policyFresh, err)
	}

	batches := make([]sourceapplication.RecordRightsDecisionRepositoryResultDTO, 0, len(policies))
	for index, policy := range policies {
		request := rightsManagementDecisionRequest(
			sourceID, actorID, policy, fmt.Sprintf("decision.projection.cursor.%d", index+1), rightsFixtureDigest("projection-cursor-decision", fmt.Sprint(index+1)),
		)
		stored, err := repository.RecordRightsDecisions(ctx, request)
		if err != nil {
			t.Fatalf("create cursor decision batch %d: %v", index+1, err)
		}
		batches = append(batches, stored)
	}
	batchFirst, err := repository.ListRightsDecisionBatches(ctx, sourceapplication.ListRightsDecisionBatchesRepositoryDTO{SourceEndpointID: sourceID, Limit: 2})
	if err != nil || len(batchFirst.Items) != 2 || batchFirst.Items[0].ID != batches[2].DecisionBatchID ||
		batchFirst.Items[1].ID != batches[1].DecisionBatchID || batchFirst.NextCursor == "" || !strings.Contains(batchFirst.NextCursor, ".") {
		t.Fatalf("first rights decision batch cursor page = %#v / %v", batchFirst, err)
	}
	batchTampered := "A" + batchFirst.NextCursor[1:]
	if batchFirst.NextCursor[0] == 'A' {
		batchTampered = "B" + batchFirst.NextCursor[1:]
	}
	for name, query := range map[string]sourceapplication.ListRightsDecisionBatchesRepositoryDTO{
		"tampered":       {SourceEndpointID: sourceID, Cursor: batchTampered, Limit: 2},
		"cross endpoint": {SourceEndpointID: otherSourceID, Cursor: batchFirst.NextCursor, Limit: 2},
		"policy cursor":  {SourceEndpointID: sourceID, Cursor: policyFirst.NextCursor, Limit: 2},
	} {
		if _, err := repository.ListRightsDecisionBatches(ctx, query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("%s rights decision batch cursor error = %v", name, err)
		}
	}
	if _, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{
		SourceEndpointID: sourceID, Cursor: batchFirst.NextCursor, Limit: 2,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("decision batch cursor crossed into policy list: %v", err)
	}
	concurrentBatchRequest := rightsManagementDecisionRequest(
		sourceID, actorID, concurrentPolicy.Policy, "decision.projection.cursor.concurrent", rightsFixtureDigest("projection-cursor-decision-concurrent"),
	)
	concurrentBatch, err := repository.RecordRightsDecisions(ctx, concurrentBatchRequest)
	if err != nil {
		t.Fatalf("create concurrent cursor decision batch: %v", err)
	}
	batchSecond, err := repository.ListRightsDecisionBatches(ctx, sourceapplication.ListRightsDecisionBatchesRepositoryDTO{
		SourceEndpointID: sourceID, Cursor: batchFirst.NextCursor, Limit: 2,
	})
	if err != nil || len(batchSecond.Items) != 1 || batchSecond.Items[0].ID != batches[0].DecisionBatchID || batchSecond.NextCursor != "" {
		t.Fatalf("second rights decision batch cursor page = %#v / %v", batchSecond, err)
	}
	batchFresh, err := repository.ListRightsDecisionBatches(ctx, sourceapplication.ListRightsDecisionBatchesRepositoryDTO{SourceEndpointID: sourceID, Limit: 1})
	if err != nil || len(batchFresh.Items) != 1 || batchFresh.Items[0].ID != concurrentBatch.DecisionBatchID {
		t.Fatalf("fresh rights decision batch cursor page = %#v / %v", batchFresh, err)
	}
	if _, err := repository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{
		SourceEndpointID: sourceID, Limit: 101,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("oversized rights policy page error = %v", err)
	}
	if _, err := repository.ListRightsDecisionBatches(ctx, sourceapplication.ListRightsDecisionBatchesRepositoryDTO{
		SourceEndpointID: sourceID, Limit: 101,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("oversized rights decision batch page error = %v", err)
	}

	shortCodec, err := pagination.NewCodec(strings.Repeat("short-rights-history-secret-", 2), time.Millisecond)
	if err != nil {
		t.Fatalf("pagination.NewCodec(short): %v", err)
	}
	shortRepository := sourcepostgres.NewRightsManagementRepositoryWithCursorCodec(runtime, shortCodec)
	expiringPolicy, err := shortRepository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{SourceEndpointID: sourceID, Limit: 1})
	if err != nil || expiringPolicy.NextCursor == "" {
		t.Fatalf("expiring rights policy page = %#v / %v", expiringPolicy, err)
	}
	expiringBatch, err := shortRepository.ListRightsDecisionBatches(ctx, sourceapplication.ListRightsDecisionBatchesRepositoryDTO{SourceEndpointID: sourceID, Limit: 1})
	if err != nil || expiringBatch.NextCursor == "" {
		t.Fatalf("expiring rights decision batch page = %#v / %v", expiringBatch, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := shortRepository.ListRightsPolicies(ctx, sourceapplication.ListRightsPoliciesRepositoryDTO{
		SourceEndpointID: sourceID, Cursor: expiringPolicy.NextCursor, Limit: 1,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired rights policy cursor error = %v", err)
	}
	if _, err := shortRepository.ListRightsDecisionBatches(ctx, sourceapplication.ListRightsDecisionBatchesRepositoryDTO{
		SourceEndpointID: sourceID, Cursor: expiringBatch.NextCursor, Limit: 1,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired rights decision batch cursor error = %v", err)
	}
}

func TestRightsManagementProjectionEvaluatesExactCurrentMatrixAndFailsClosed(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	ctx := context.Background()

	sourceID := insertRightsManagementSource(t, runtime.SQL, "projection-matrix")
	actorID := insertRightsFixtureActor(t, runtime.SQL, "projection-matrix")
	at := time.Now().UTC().Truncate(time.Microsecond)
	subjectKey := rightsFixtureDigest("projection-matrix-subject")
	inputDigest := rightsFixtureDigest("projection-matrix-input")

	firstPolicyRequest := rightsManagementPolicyRequest(sourceID, actorID, "policy.projection.matrix.first", rightsFixtureDigest("matrix-policy-first"), 51)
	firstPolicyRequest.EffectiveFrom = at.Add(-time.Hour)
	firstPolicy, err := repository.CreateRightsPolicy(ctx, firstPolicyRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondPolicyRequest := rightsManagementPolicyRequest(sourceID, actorID, "policy.projection.matrix.second", rightsFixtureDigest("matrix-policy-second"), 52)
	secondPolicyRequest.EffectiveFrom = at.Add(-time.Hour)
	secondPolicy, err := repository.CreateRightsPolicy(ctx, secondPolicyRequest)
	if err != nil {
		t.Fatal(err)
	}

	firstRetention, secondRetention := 30, 10
	firstDecisions := sourceapplication.RecordRightsDecisionRepositoryDTO{
		ActorID: actorID, IdempotencyKey: "decision.projection.matrix.first", CommandFingerprint: rightsFixtureDigest("matrix-decision-first"),
		SourceConnectionID: sourceID, ExpectedPolicyVersion: firstPolicy.Policy.Version, Policy: firstPolicy.Policy,
		SubjectType: "raw_response", SubjectKey: subjectKey, InputDigest: inputDigest,
		Decisions: []sourceapplication.RightsActionDecisionDTO{
			{Action: "fetch", Decision: "allow", ReasonCodes: []string{"endpoint_terms"}, Evaluator: "projection-test", EvaluatedAt: at, EffectiveFrom: at.Add(-time.Minute)},
			{Action: "display_private", Decision: "deny", ReasonCodes: []string{"display_restricted"}, Evaluator: "projection-test", EvaluatedAt: at, EffectiveFrom: at.Add(-time.Minute)},
			{Action: "retain", Decision: "allow", ReasonCodes: []string{"retention_approved"}, Evaluator: "projection-test", EvaluatedAt: at, EffectiveFrom: at.Add(-time.Minute), RetentionDays: &firstRetention},
		},
	}
	firstBatch, err := repository.RecordRightsDecisions(ctx, firstDecisions)
	if err != nil {
		t.Fatal(err)
	}
	secondDecisions := sourceapplication.RecordRightsDecisionRepositoryDTO{
		ActorID: actorID, IdempotencyKey: "decision.projection.matrix.second", CommandFingerprint: rightsFixtureDigest("matrix-decision-second"),
		SourceConnectionID: sourceID, ExpectedPolicyVersion: secondPolicy.Policy.Version, Policy: secondPolicy.Policy,
		SubjectType: "raw_response", SubjectKey: subjectKey, InputDigest: inputDigest,
		Decisions: []sourceapplication.RightsActionDecisionDTO{
			{Action: "fetch", Decision: "unknown", ReasonCodes: []string{"terms_ambiguous"}, Evaluator: "projection-test", EvaluatedAt: at, EffectiveFrom: at.Add(-time.Minute)},
			{Action: "retain", Decision: "allow", ReasonCodes: []string{"short_retention"}, Evaluator: "projection-test", EvaluatedAt: at, EffectiveFrom: at.Add(-time.Minute), RetentionDays: &secondRetention},
		},
	}
	secondBatch, err := repository.RecordRightsDecisions(ctx, secondDecisions)
	if err != nil {
		t.Fatal(err)
	}

	matrix, err := repository.EvaluateRightsActionMatrix(ctx, sourceapplication.EvaluateRightsActionMatrixRepositoryDTO{
		SourceEndpointID: sourceID, SubjectType: "raw_response", SubjectKey: subjectKey, InputDigest: inputDigest, At: at,
	})
	if err != nil {
		t.Fatalf("EvaluateRightsActionMatrix(): %v", err)
	}
	if matrix.SourceEndpointID != sourceID || !matrix.EvaluatedAt.Equal(at) || len(matrix.Actions) != 9 {
		t.Fatalf("matrix identity = %#v", matrix)
	}
	actions := rightsActionMatrixByName(matrix.Actions)
	if actions["fetch"].Decision != "deny" || len(actions["fetch"].DecisionIDs) != 2 {
		t.Fatalf("same-priority allow/unknown did not fail closed: %#v", actions["fetch"])
	}
	if actions["display_private"].Decision != "deny" || len(actions["display_private"].DecisionIDs) != 1 {
		t.Fatalf("explicit deny projection = %#v", actions["display_private"])
	}
	if actions["retain"].Decision != "allow" || actions["retain"].RetentionDays == nil || *actions["retain"].RetentionDays != 10 ||
		len(actions["retain"].DecisionIDs) != 2 {
		t.Fatalf("conservative retain projection = %#v", actions["retain"])
	}
	if actions["store_raw"].Decision != "unknown" || len(actions["store_raw"].DecisionIDs) != 0 || actions["store_raw"].Priority != nil {
		t.Fatalf("missing action was not unknown: %#v", actions["store_raw"])
	}
	for _, decisionID := range []int64{firstBatch.Decisions[0].ID, secondBatch.Decisions[0].ID} {
		found := false
		for _, selected := range actions["fetch"].DecisionIDs {
			found = found || selected == decisionID
		}
		if !found {
			t.Fatalf("fetch receipt omitted decision %d: %#v", decisionID, actions["fetch"])
		}
	}

	otherDigest := rightsFixtureDigest("projection-matrix-other-input")
	unknown, err := repository.EvaluateRightsActionMatrix(ctx, sourceapplication.EvaluateRightsActionMatrixRepositoryDTO{
		SourceEndpointID: sourceID, SubjectType: "raw_response", SubjectKey: subjectKey, InputDigest: otherDigest, At: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range unknown.Actions {
		if action.Decision != "unknown" || len(action.DecisionIDs) != 0 {
			t.Fatalf("a source-level decision leaked across exact digest: %#v", action)
		}
	}
	for _, invalid := range []sourceapplication.EvaluateRightsActionMatrixRepositoryDTO{
		{SourceEndpointID: sourceID, SubjectType: "unsupported", SubjectKey: subjectKey, InputDigest: inputDigest, At: at},
		{SourceEndpointID: sourceID, SubjectType: "raw_response", SubjectKey: subjectKey, InputDigest: strings.ToUpper(inputDigest), At: at},
		{SourceEndpointID: sourceID, SubjectType: "raw_response", SubjectKey: subjectKey, InputDigest: inputDigest},
	} {
		if _, err := repository.EvaluateRightsActionMatrix(ctx, invalid); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("invalid exact repository evaluation error = %v", err)
		}
	}
}

func TestRightsManagementProjectionReturnsCredentialFreeConnectorFacts(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	sourceID := insertRightsManagementSource(t, runtime.SQL, "projection-capability")
	if _, err := runtime.SQL.Exec(`
UPDATE source_connections
SET enabled=true,health_status='healthy',auth_type='api_key',credential_ref='env:RIGHTS_PROJECTION_SECRET',
    config=jsonb_build_object('allow_body_storage',true)
WHERE id=$1`, sourceID); err != nil {
		t.Fatalf("seed source private facts: %v", err)
	}
	facts, err := repository.FindSourceEndpointCapabilityFacts(context.Background(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if facts.SourceEndpointID != sourceID || facts.SourceType != "rss" || !facts.Enabled || facts.HealthStatus != "healthy" {
		t.Fatalf("capability facts = %#v", facts)
	}
	if _, err := repository.FindSourceEndpointCapabilityFacts(context.Background(), sourceID+999999); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("missing endpoint error = %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE source_connections SET deleted_at=now() WHERE id=$1`, sourceID); err != nil {
		t.Fatalf("archive capability source: %v", err)
	}
	if _, err := repository.FindSourceEndpointCapabilityFacts(context.Background(), sourceID); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("archived public capability error = %v", err)
	}
}

func rightsActionMatrixByName(values []sourceapplication.RightsActionCapabilityDTO) map[string]sourceapplication.RightsActionCapabilityDTO {
	result := make(map[string]sourceapplication.RightsActionCapabilityDTO, len(values))
	for _, value := range values {
		result[value.Action] = value
	}
	return result
}
