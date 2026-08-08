package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/application"
	agentdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/domain"
	agentpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/infrastructure/postgres"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	identitypostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

func TestAgentTokenLifecycleStoresOnlyHashAndRevokesImmediately(t *testing.T) {
	runtime := openAgentRuntime(t)
	now := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	userID := createAgentUser(t, runtime, "editor", "agent-editor@example.test")
	repository := agentpostgres.NewRepository(runtime)
	service, err := agentapplication.NewService(agentapplication.Dependencies{
		Runtime: runtime, Tokens: repository, Audit: identitypostgres.NewAuditRepository(runtime), Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	subject := identitydomain.Subject{UserID: userID, SessionID: 77, Role: identitydomain.RoleEditor}
	created, err := service.Create(context.Background(), agentapplication.CreateInput{
		Subject: subject, Name: "Research agent", LifetimeDays: 30,
		Scopes: []agentdomain.Scope{agentdomain.ScopeSearchRun, agentdomain.ScopeEventsRead},
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if !strings.HasPrefix(created.Raw, "hk_agent_") || created.Token.TokenHash == "" || strings.Contains(created.Token.TokenHash, created.Raw) {
		t.Fatalf("unsafe created credential projection: %#v", created)
	}
	var storedHash, storedPrefix string
	if err := runtime.SQL.QueryRow(`SELECT token_hash, token_prefix FROM agent_tokens WHERE id = $1`, created.Token.ID).Scan(&storedHash, &storedPrefix); err != nil {
		t.Fatalf("read stored token: %v", err)
	}
	if storedHash == created.Raw || storedPrefix != created.Token.TokenPrefix || strings.Contains(storedPrefix, created.Raw) {
		t.Fatalf("stored token = hash %q prefix %q", storedHash, storedPrefix)
	}

	listed, err := service.List(context.Background(), subject)
	if err != nil || len(listed) != 1 {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
	if listed[0].TokenHash != "" {
		t.Fatal("safe list loaded token hash")
	}
	principal, err := service.Authenticate(context.Background(), created.Raw)
	if err != nil || principal.UserID != userID || principal.Role != identitydomain.RoleEditor || len(principal.Scopes) != 2 {
		t.Fatalf("Authenticate() = %#v, %v", principal, err)
	}
	var lastUsedAt *time.Time
	if err := runtime.SQL.QueryRow(`SELECT last_used_at FROM agent_tokens WHERE id = $1`, created.Token.ID).Scan(&lastUsedAt); err != nil || lastUsedAt == nil {
		t.Fatalf("last_used_at = %v, %v", lastUsedAt, err)
	}

	if _, err := runtime.SQL.Exec(`UPDATE users SET role = 'viewer' WHERE id = $1`, userID); err != nil {
		t.Fatalf("downgrade user: %v", err)
	}
	principal, err = service.Authenticate(context.Background(), created.Raw)
	if err != nil || principal.Role != identitydomain.RoleViewer {
		t.Fatalf("Authenticate after downgrade = %#v, %v", principal, err)
	}

	revoked, err := service.Revoke(context.Background(), agentapplication.RevokeInput{Subject: identitydomain.Subject{UserID: userID, SessionID: 77, Role: identitydomain.RoleViewer}, TokenID: created.Token.ID, ExpectedVersion: created.Token.Version})
	if err != nil || revoked.RevokedAt == nil || revoked.Version != created.Token.Version+1 {
		t.Fatalf("Revoke() = %#v, %v", revoked, err)
	}
	requireCode(t, authenticateError(service, created.Raw), sharederrors.CodeUnauthenticated)
	if _, err := service.Revoke(context.Background(), agentapplication.RevokeInput{Subject: subject, TokenID: created.Token.ID, ExpectedVersion: created.Token.Version}); err == nil {
		t.Fatal("repeated stale revoke should conflict")
	} else {
		requireCode(t, err, sharederrors.CodeConflict)
	}

	var leaked int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE coalesce(before_data::text,'') LIKE '%' || $1 || '%' OR coalesce(after_data::text,'') LIKE '%' || $1 || '%'`, created.Raw).Scan(&leaked); err != nil {
		t.Fatalf("search audit leak: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("raw credential leaked into %d audit rows", leaked)
	}
}

func TestAgentTokenCreationEnforcesRoleAndActiveLimit(t *testing.T) {
	runtime := openAgentRuntime(t)
	now := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	userID := createAgentUser(t, runtime, "viewer", "agent-viewer@example.test")
	service, err := agentapplication.NewService(agentapplication.Dependencies{
		Runtime: runtime, Tokens: agentpostgres.NewRepository(runtime), Audit: identitypostgres.NewAuditRepository(runtime), Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	subject := identitydomain.Subject{UserID: userID, SessionID: 1, Role: identitydomain.RoleViewer}
	_, err = service.Create(context.Background(), agentapplication.CreateInput{Subject: subject, Name: "forbidden", LifetimeDays: 30, Scopes: []agentdomain.Scope{agentdomain.ScopeSearchRun}})
	requireCode(t, err, sharederrors.CodeForbidden)
	for index := 0; index < agentdomain.MaximumActiveTokens; index++ {
		if _, err := service.Create(context.Background(), agentapplication.CreateInput{Subject: subject, Name: "viewer token " + string(rune('A'+index)), LifetimeDays: 30, Scopes: []agentdomain.Scope{agentdomain.ScopeEventsRead}}); err != nil {
			t.Fatalf("Create token %d: %v", index, err)
		}
	}
	_, err = service.Create(context.Background(), agentapplication.CreateInput{Subject: subject, Name: "over limit", LifetimeDays: 30, Scopes: []agentdomain.Scope{agentdomain.ScopeEventsRead}})
	requireCode(t, err, sharederrors.CodeConflict)
}

func TestAgentTokenAuthenticationHidesInactiveCredentialReasons(t *testing.T) {
	runtime := openAgentRuntime(t)
	now := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	service, err := agentapplication.NewService(agentapplication.Dependencies{
		Runtime: runtime, Tokens: agentpostgres.NewRepository(runtime), Audit: identitypostgres.NewAuditRepository(runtime), Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}

	create := func(email string) (int64, agentapplication.CreatedToken) {
		t.Helper()
		userID := createAgentUser(t, runtime, "editor", email)
		created, createErr := service.Create(context.Background(), agentapplication.CreateInput{
			Subject: identitydomain.Subject{UserID: userID, SessionID: 1, Role: identitydomain.RoleEditor},
			Name:    "inactive credential", LifetimeDays: 30, Scopes: []agentdomain.Scope{agentdomain.ScopeEventsRead},
		})
		if createErr != nil {
			t.Fatalf("Create(): %v", createErr)
		}
		return userID, created
	}

	_, expired := create("agent-expired@example.test")
	if _, err := runtime.SQL.Exec(`UPDATE agent_tokens SET created_at = $1, updated_at = $1, expires_at = $2 WHERE id = $3`, now.Add(-48*time.Hour), now.Add(-24*time.Hour), expired.Token.ID); err != nil {
		t.Fatalf("expire token: %v", err)
	}
	requireCode(t, authenticateError(service, expired.Raw), sharederrors.CodeUnauthenticated)

	disabledUserID, disabled := create("agent-disabled@example.test")
	if _, err := runtime.SQL.Exec(`UPDATE users SET status = 'disabled' WHERE id = $1`, disabledUserID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	requireCode(t, authenticateError(service, disabled.Raw), sharederrors.CodeUnauthenticated)

	deletedUserID, deleted := create("agent-deleted@example.test")
	if _, err := runtime.SQL.Exec(`UPDATE users SET deleted_at = $1 WHERE id = $2`, now, deletedUserID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	requireCode(t, authenticateError(service, deleted.Raw), sharederrors.CodeUnauthenticated)
}

func authenticateError(service *agentapplication.Service, raw string) error {
	_, err := service.Authenticate(context.Background(), raw)
	return err
}

func requireCode(t *testing.T, err error, code int) {
	t.Helper()
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) || appError.Code != code {
		t.Fatalf("error = %v, want code %d", err, code)
	}
}

func openAgentRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	runtime, err := database.Open(context.Background(), postgresfixture.New(t))
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if err := database.InitializeEmpty(context.Background(), runtime.Pool); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return runtime
}

func createAgentUser(t *testing.T, runtime *database.Runtime, role, email string) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role,status) VALUES ($1,'unused','Agent user',$2,'active') RETURNING id`, email, role).Scan(&id); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}
