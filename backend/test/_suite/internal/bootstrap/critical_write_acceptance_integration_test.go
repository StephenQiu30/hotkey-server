package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	identitypostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/postgres"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestCriticalRightsWriteMatrixPreservesFactsAndAppendsSanitizedAudit(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	defer func() { _ = runtime.Close() }()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}

	adminID, viewerID, sourceID := seedCriticalRightsWriteActors(t, runtime)
	repository := sourcepostgres.NewRightsManagementRepository(runtime)
	auditWriter := operationspostgres.NewAuditWriter(runtime)
	service, err := newRightsManagementService(
		repository,
		newRightsActorAuthorizer(identitypostgres.NewUserRepository(runtime)),
		newRightsManagementAuditWriter(auditWriter),
		sourcepostgres.NewRightsManagementTransactionAdapter(runtime),
	)
	if err != nil {
		t.Fatalf("newRightsManagementService(): %v", err)
	}

	basePolicy := criticalRightsPolicyCommand(adminID, sourceID, "critical.policy.once")
	invalid := basePolicy
	invalid.IdempotencyKey = "critical.policy.invalid"
	invalid.Priority = 999
	before := readCriticalWriteFacts(t, runtime)
	if _, err := service.CreatePolicy(ctx, invalid); err == nil {
		t.Fatal("invalid policy succeeded")
	}
	assertCriticalWriteFacts(t, runtime, before)

	denied := basePolicy
	denied.ActorID = viewerID
	denied.IdempotencyKey = "critical.policy.denied"
	if _, err := service.CreatePolicy(ctx, denied); criticalWriteAppCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("denied policy code = %d, error = %v", criticalWriteAppCode(err), err)
	}
	assertCriticalWriteFacts(t, runtime, before)

	created, err := service.CreatePolicy(ctx, basePolicy)
	if err != nil || created.IdempotentReplay || created.Policy.ID <= 0 {
		t.Fatalf("CreatePolicy(first) = %#v, %v", created, err)
	}
	afterPolicy := readCriticalWriteFacts(t, runtime)
	if afterPolicy.Policies != before.Policies+1 || afterPolicy.Batches != before.Batches || afterPolicy.Decisions != before.Decisions {
		t.Fatalf("first policy facts = %#v, before %#v", afterPolicy, before)
	}
	replayedPolicy, err := service.CreatePolicy(ctx, basePolicy)
	if err != nil || !replayedPolicy.IdempotentReplay || replayedPolicy.Policy.ID != created.Policy.ID {
		t.Fatalf("CreatePolicy(replay) = %#v, %v", replayedPolicy, err)
	}
	assertCriticalWriteFacts(t, runtime, afterPolicy)
	policyConflict := basePolicy
	policyConflict.BasisSummary = "different approved policy input"
	if _, err := service.CreatePolicy(ctx, policyConflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("CreatePolicy(conflict) error = %v, want conflict", err)
	}
	assertCriticalWriteFacts(t, runtime, afterPolicy)

	baseDecision := criticalRightsDecisionCommand(adminID, sourceID, created.Policy, "critical.decision.once", "b", "c")
	stale := baseDecision
	stale.IdempotencyKey = "critical.decision.stale"
	stale.ExpectedPolicyVersion++
	if _, err := service.RecordDecisions(ctx, stale); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("RecordDecisions(stale) error = %v, want not found", err)
	}
	assertCriticalWriteFacts(t, runtime, afterPolicy)

	firstDecision, err := service.RecordDecisions(ctx, baseDecision)
	if err != nil || firstDecision.IdempotentReplay || firstDecision.DecisionBatchID <= 0 || len(firstDecision.Decisions) != 1 {
		t.Fatalf("RecordDecisions(first) = %#v, %v", firstDecision, err)
	}
	afterDecision := readCriticalWriteFacts(t, runtime)
	if afterDecision.Policies != afterPolicy.Policies || afterDecision.Batches != afterPolicy.Batches+1 || afterDecision.Decisions != afterPolicy.Decisions+1 {
		t.Fatalf("first decision facts = %#v, before %#v", afterDecision, afterPolicy)
	}
	replayedDecision, err := service.RecordDecisions(ctx, baseDecision)
	if err != nil || !replayedDecision.IdempotentReplay || replayedDecision.DecisionBatchID != firstDecision.DecisionBatchID {
		t.Fatalf("RecordDecisions(replay) = %#v, %v", replayedDecision, err)
	}
	assertCriticalWriteFacts(t, runtime, afterDecision)
	decisionConflict := cloneCriticalRightsDecisionCommand(baseDecision)
	decisionConflict.Decisions[0].Evaluator = "different-evaluator"
	if _, err := service.RecordDecisions(ctx, decisionConflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("RecordDecisions(conflict) error = %v, want conflict", err)
	}
	assertCriticalWriteFacts(t, runtime, afterDecision)

	concurrent := criticalRightsDecisionCommand(adminID, sourceID, created.Policy, "critical.decision.concurrent", "d", "e")
	const writers = 8
	results := make(chan sourceapplication.RecordRightsDecisionResult, writers)
	errorsChannel := make(chan error, writers)
	var group sync.WaitGroup
	group.Add(writers)
	for range writers {
		go func() {
			defer group.Done()
			result, err := service.RecordDecisions(context.Background(), concurrent)
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
		t.Fatalf("concurrent RecordDecisions(): %v", err)
	}
	var concurrentBatchID int64
	firstWrites := 0
	for result := range results {
		if !result.IdempotentReplay {
			firstWrites++
		}
		if concurrentBatchID == 0 {
			concurrentBatchID = result.DecisionBatchID
		}
		if result.DecisionBatchID != concurrentBatchID || len(result.Decisions) != 1 {
			t.Fatalf("concurrent receipt drift = %#v, batch %d", result, concurrentBatchID)
		}
	}
	if firstWrites != 1 {
		t.Fatalf("concurrent first writes = %d, want 1", firstWrites)
	}
	finalFacts := readCriticalWriteFacts(t, runtime)
	if finalFacts.Policies != afterDecision.Policies || finalFacts.Batches != afterDecision.Batches+1 || finalFacts.Decisions != afterDecision.Decisions+1 {
		t.Fatalf("concurrent facts = %#v, before %#v", finalFacts, afterDecision)
	}

	assertCriticalRightsAuditMatrix(t, runtime)
	if _, err := runtime.SQL.Exec(`UPDATE audit_logs SET result='denied' WHERE id=(SELECT min(id) FROM audit_logs)`); err == nil {
		t.Error("audit update succeeded; audit facts must be append-only")
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM audit_logs WHERE id=(SELECT min(id) FROM audit_logs)`); err == nil {
		t.Error("audit delete succeeded; audit facts must be append-only")
	}
	assertCriticalRightsAuditMatrix(t, runtime)
}

type criticalWriteFacts struct {
	Policies  int
	Batches   int
	Decisions int
}

func readCriticalWriteFacts(t *testing.T, runtime *database.Runtime) criticalWriteFacts {
	t.Helper()
	var facts criticalWriteFacts
	if err := runtime.SQL.QueryRow(`SELECT
  (SELECT count(*) FROM source_rights_policies),
  (SELECT count(*) FROM source_rights_decision_batches),
  (SELECT count(*) FROM source_rights_decisions)`).Scan(&facts.Policies, &facts.Batches, &facts.Decisions); err != nil {
		t.Fatalf("read critical write facts: %v", err)
	}
	return facts
}

func assertCriticalWriteFacts(t *testing.T, runtime *database.Runtime, want criticalWriteFacts) {
	t.Helper()
	if got := readCriticalWriteFacts(t, runtime); got != want {
		t.Fatalf("critical write facts = %#v, want %#v", got, want)
	}
}

func assertCriticalRightsAuditMatrix(t *testing.T, runtime *database.Runtime) {
	t.Helper()
	want := map[string]int{
		"rights_policy.created|failure|invalid_input":                 1,
		"rights_policy.created|denied|authorization_denied":           1,
		"rights_policy.created|success|":                              1,
		"rights_policy.created|failure|idempotency_conflict":          1,
		"rights_decision_batch.recorded|failure|version_conflict":     1,
		"rights_decision_batch.recorded|success|":                     2,
		"rights_decision_batch.recorded|failure|idempotency_conflict": 1,
	}
	rows, err := runtime.SQL.Query(`SELECT action,result,COALESCE(after_data->>'reason_code',''),count(*) FROM audit_logs GROUP BY action,result,COALESCE(after_data->>'reason_code','')`)
	if err != nil {
		t.Fatalf("query critical write audits: %v", err)
	}
	defer func() { _ = rows.Close() }()
	got := make(map[string]int)
	for rows.Next() {
		var action, result, reason string
		var count int
		if err := rows.Scan(&action, &result, &reason, &count); err != nil {
			t.Fatalf("scan critical write audit: %v", err)
		}
		got[action+"|"+result+"|"+reason] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate critical write audits: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("critical write audit matrix = %#v, want %#v", got, want)
	}
	var leaked int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE COALESCE(before_data::text,'') LIKE '%critical-rights-secret%' OR COALESCE(after_data::text,'') LIKE '%critical-rights-secret%'`).Scan(&leaked); err != nil {
		t.Fatalf("scan audit leak sentinel: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("critical write audit leak sentinel count = %d", leaked)
	}
}

func seedCriticalRightsWriteActors(t *testing.T, runtime *database.Runtime) (int64, int64, int64) {
	t.Helper()
	var adminID, viewerID, sourceID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role,status) VALUES ('critical-rights-admin@example.test','fixture','Critical Rights Admin','admin','active') RETURNING id`).Scan(&adminID); err != nil {
		t.Fatalf("seed critical rights admin: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role,status) VALUES ('critical-rights-viewer@example.test','fixture','Critical Rights Viewer','viewer','active') RETURNING id`).Scan(&viewerID); err != nil {
		t.Fatalf("seed critical rights viewer: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type,name,endpoint,auth_type,created_by,updated_by) VALUES ('rss','critical-rights-source','https://feeds.example.test/critical-rights','none',$1,$1) RETURNING id`, adminID).Scan(&sourceID); err != nil {
		t.Fatalf("seed critical rights source: %v", err)
	}
	return adminID, viewerID, sourceID
}

func criticalRightsPolicyCommand(actorID, sourceID int64, key string) sourceapplication.CreateRightsPolicyCommand {
	now := time.Date(2026, time.August, 29, 6, 0, 0, 0, time.UTC)
	return sourceapplication.CreateRightsPolicyCommand{
		ActorID: actorID, IdempotencyKey: key, SourceConnectionID: &sourceID,
		ScopeType: string(sourcedomain.RightsScopeSourceEndpoint), ScopeSubject: "critical-rights-feed", Revision: 1,
		Priority: int(sourcedomain.RightsPriorityEndpointContract), BasisSummary: "approved critical-rights-secret policy terms",
		TermsURL: "https://publisher.example.test/terms", LicenseURI: "urn:license:critical-rights",
		EffectiveFrom: now, ApprovedByUserID: &actorID,
	}
}

func criticalRightsDecisionCommand(actorID, sourceID int64, policy sourceapplication.RightsPolicyDTO, key, subjectSeed, digestSeed string) sourceapplication.RecordRightsDecisionCommand {
	now := time.Date(2026, time.August, 29, 6, 30, 0, 0, time.UTC)
	return sourceapplication.RecordRightsDecisionCommand{
		ActorID: actorID, IdempotencyKey: key, SourceConnectionID: sourceID,
		PolicyID: policy.ID, ExpectedPolicyVersion: policy.Version,
		SubjectType: string(sourcedomain.RightsSubjectRawResponse), SubjectKey: strings.Repeat(subjectSeed, 64), InputDigest: strings.Repeat(digestSeed, 64),
		Decisions: []sourceapplication.RightsActionDecisionDTO{{
			Action: string(sourcedomain.RightsActionStoreRaw), Decision: string(sourcedomain.RightsAllow),
			ReasonCodes: []string{"terms_confirmed"}, Evaluator: "rights-admin", EvaluatedAt: now, EffectiveFrom: now,
		}},
	}
}

func cloneCriticalRightsDecisionCommand(value sourceapplication.RecordRightsDecisionCommand) sourceapplication.RecordRightsDecisionCommand {
	result := value
	result.Decisions = append([]sourceapplication.RightsActionDecisionDTO(nil), value.Decisions...)
	for index := range result.Decisions {
		result.Decisions[index].ReasonCodes = append([]string(nil), value.Decisions[index].ReasonCodes...)
	}
	return result
}

func criticalWriteAppCode(err error) int {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError.Code
	}
	return 0
}
