package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestIntentRepositoryInitializesOnlyTheCurrentConfigurationDraft(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	monitorID, _ := insertUninitializedIntentConfiguration(t, runtime, "intent initialization")

	if _, err := repository.FindCurrent(context.Background(), monitorapplication.ReadCurrentIntentDraftRepositoryQuery{MonitorID: monitorID}); !errors.Is(err, monitorapplication.ErrIntentDraftNotFound) {
		t.Fatalf("FindCurrent(uninitialized) error = %v", err)
	}
	initial := monitorapplication.IntentDraftDTO{
		MonitorID: monitorID, ResourceVersion: 1, Objective: "Track launch disruption",
		Clauses:  []monitorapplication.IntentClauseDTO{{Operator: "must", Field: "action", Value: "launch"}},
		Entities: []monitorapplication.IntentEntityDTO{{CanonicalID: "product:hotkey", DisplayName: "HotKey", Aliases: []string{"热点键"}}},
		Examples: []monitorapplication.IntentExampleDTO{{Label: "positive", Text: "The launch is unavailable"}},
	}
	created, err := repository.InitializeCurrent(context.Background(), monitorapplication.InitializeCurrentIntentDraftMutationDTO{Initial: initial})
	if err != nil {
		t.Fatalf("InitializeCurrent(): %v", err)
	}
	if created.DraftID <= 0 || created.ResourceVersion != 1 || created.Objective != initial.Objective {
		t.Fatalf("created draft = %#v", created)
	}
	current, err := repository.FindCurrent(context.Background(), monitorapplication.ReadCurrentIntentDraftRepositoryQuery{MonitorID: monitorID})
	if err != nil || current.DraftID != created.DraftID || len(current.Clauses) != 1 || len(current.Entities) != 1 || len(current.Examples) != 1 {
		t.Fatalf("FindCurrent() = %#v / %v", current, err)
	}
	if _, err := repository.InitializeCurrent(context.Background(), monitorapplication.InitializeCurrentIntentDraftMutationDTO{Initial: initial}); !errors.Is(err, monitorapplication.ErrIntentVersionConflict) {
		t.Fatalf("duplicate initialization error = %v", err)
	}
	var drafts, revisions, legacyRules int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_intent_drafts WHERE monitor_id=$1`, monitorID).Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_intent_draft_revisions WHERE monitor_id=$1`, monitorID).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_rules r JOIN monitor_config_versions c ON c.id=r.config_version_id WHERE c.monitor_id=$1`, monitorID).Scan(&legacyRules); err != nil {
		t.Fatal(err)
	}
	if drafts != 1 || revisions != 1 || legacyRules != 0 {
		t.Fatalf("draft/revision/legacy-rule counts = %d/%d/%d", drafts, revisions, legacyRules)
	}
}

func TestIntentRepositoryConcurrentInitializationHasOneWinnerAndNewConfigGetsNewIdentity(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository, _ := monitorpostgres.NewIntentRepository(runtime)
	monitorID, configID := insertUninitializedIntentConfiguration(t, runtime, "intent concurrent initialization")
	mutation := monitorapplication.InitializeCurrentIntentDraftMutationDTO{Initial: monitorapplication.IntentDraftDTO{
		MonitorID: monitorID, ResourceVersion: 1, Objective: "Track one durable intent",
	}}

	const callers = 6
	var wait sync.WaitGroup
	results := make(chan monitorapplication.IntentDraftDTO, callers)
	errorsChannel := make(chan error, callers)
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, initializeErr := repository.InitializeCurrent(context.Background(), mutation)
			results <- result
			errorsChannel <- initializeErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	successes, conflicts := 0, 0
	var firstDraftID int64
	for initializeErr := range errorsChannel {
		switch {
		case initializeErr == nil:
			successes++
		case errors.Is(initializeErr, monitorapplication.ErrIntentVersionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent initialization error = %v", initializeErr)
		}
	}
	for result := range results {
		if result.DraftID > 0 {
			firstDraftID = result.DraftID
		}
	}
	if successes != 1 || conflicts != callers-1 || firstDraftID <= 0 {
		t.Fatalf("success/conflict/draft = %d/%d/%d", successes, conflicts, firstDraftID)
	}

	// Publishing the old configuration makes its intent historical. A new
	// configuration draft must allocate a distinct DraftID at resource v1.
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state='published',config_hash=$2,published_at=now() WHERE id=$1`, configID, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("publish old config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=NULL,published_config_version_id=$2 WHERE id=$1`, monitorID, configID); err != nil {
		t.Fatalf("move monitor pointers: %v", err)
	}
	var nextConfigID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id,revision,state) VALUES ($1,2,'draft') RETURNING id`, monitorID).Scan(&nextConfigID); err != nil {
		t.Fatalf("insert next config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=$2 WHERE id=$1`, monitorID, nextConfigID); err != nil {
		t.Fatalf("bind next config: %v", err)
	}
	next, err := repository.InitializeCurrent(context.Background(), mutation)
	if err != nil {
		t.Fatalf("InitializeCurrent(next config): %v", err)
	}
	if next.DraftID == firstDraftID || next.ResourceVersion != 1 {
		t.Fatalf("next draft reused identity/version: old=%d next=%#v", firstDraftID, next)
	}
}

func TestIntentRepositoryAuthorizerReloadsDurableRoleAndStatus(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository, _ := monitorpostgres.NewIntentRepository(runtime)
	monitorID, _ := insertUninitializedIntentConfiguration(t, runtime, "intent authorization")
	actor := func(email, role, status string) int64 {
		t.Helper()
		var id int64
		if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role,status) VALUES ($1,'not-a-credential',$2,$3,$4) RETURNING id`, email, role, role, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	editorID := actor("intent-editor@example.test", "editor", "active")
	adminID := actor("intent-admin@example.test", "admin", "active")
	disabledID := actor("intent-disabled@example.test", "admin", "disabled")

	if err := repository.AuthorizeIntentControl(context.Background(), monitorapplication.AuthorizeIntentControlQueryDTO{ActorUserID: editorID, MonitorID: monitorID, Operation: monitorapplication.IntentControlReadDraft}); err != nil {
		t.Fatalf("active editor read: %v", err)
	}
	for _, query := range []monitorapplication.AuthorizeIntentControlQueryDTO{
		{ActorUserID: editorID, MonitorID: monitorID, Operation: monitorapplication.IntentControlSubmitExpansion},
		{ActorUserID: disabledID, MonitorID: monitorID, Operation: monitorapplication.IntentControlReadDraft},
	} {
		if err := repository.AuthorizeIntentControl(context.Background(), query); !errors.Is(err, monitorapplication.ErrIntentAuthorizationDenied) {
			t.Fatalf("authorization should fail closed for %#v: %v", query, err)
		}
	}
	if err := repository.AuthorizeIntentControl(context.Background(), monitorapplication.AuthorizeIntentControlQueryDTO{ActorUserID: adminID, MonitorID: monitorID, Operation: monitorapplication.IntentControlReviewCandidate}); err != nil {
		t.Fatalf("active admin review: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE users SET status='disabled' WHERE id=$1`, adminID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AuthorizeIntentControl(context.Background(), monitorapplication.AuthorizeIntentControlQueryDTO{ActorUserID: adminID, MonitorID: monitorID, Operation: monitorapplication.IntentControlReviewCandidate}); !errors.Is(err, monitorapplication.ErrIntentAuthorizationDenied) {
		t.Fatalf("disabled admin replay authorization = %v", err)
	}
}

func insertUninitializedIntentConfiguration(t *testing.T, runtime *database.Runtime, label string) (int64, int64) {
	t.Helper()
	var monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name,status) VALUES ($1,'draft') RETURNING id`, fmt.Sprintf("%s-%s", label, strings.Repeat("x", 8))).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id,revision,state) VALUES ($1,1,'draft') RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=$2 WHERE id=$1`, monitorID, configID); err != nil {
		t.Fatal(err)
	}
	return monitorID, configID
}
