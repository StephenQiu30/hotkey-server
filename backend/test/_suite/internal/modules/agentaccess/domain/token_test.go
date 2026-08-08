package domain_test

import (
	"testing"
	"time"

	agentdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
)

func TestNormalizeScopesRejectsUnknownAndOrdersUniqueValues(t *testing.T) {
	got, err := agentdomain.NormalizeScopes([]agentdomain.Scope{agentdomain.ScopeReportsRead, agentdomain.ScopeEventsRead, agentdomain.ScopeReportsRead})
	if err != nil {
		t.Fatalf("normalize scopes: %v", err)
	}
	if len(got) != 2 || got[0] != agentdomain.ScopeEventsRead || got[1] != agentdomain.ScopeReportsRead {
		t.Fatalf("scopes = %#v", got)
	}
	if _, err := agentdomain.NormalizeScopes([]agentdomain.Scope{"users.write"}); err == nil {
		t.Fatal("unknown scope should fail")
	}
}

func TestScopesAllowedForRoleKeepsSearchAboveViewer(t *testing.T) {
	if agentdomain.ScopesAllowedForRole(identitydomain.RoleViewer, []agentdomain.Scope{agentdomain.ScopeSearchRun}) {
		t.Fatal("viewer must not receive search.run")
	}
	if !agentdomain.ScopesAllowedForRole(identitydomain.RoleEditor, []agentdomain.Scope{agentdomain.ScopeSearchRun, agentdomain.ScopeEventsRead}) {
		t.Fatal("editor should receive search.run")
	}
	if !agentdomain.ScopesAllowedForRole(identitydomain.RoleViewer, []agentdomain.Scope{agentdomain.ScopeAlertsWrite}) {
		t.Fatal("viewer may acknowledge and resolve alerts")
	}
}

func TestTokenValidateEnforcesNameScopesAndLifetime(t *testing.T) {
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	valid := agentdomain.Token{UserID: 1, Name: "Research agent", Scopes: []agentdomain.Scope{agentdomain.ScopeEventsRead}, ExpiresAt: now.Add(30 * 24 * time.Hour)}
	if err := valid.Validate(now); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	for _, token := range []agentdomain.Token{
		{UserID: 1, Name: " padded ", Scopes: valid.Scopes, ExpiresAt: valid.ExpiresAt},
		{UserID: 1, Name: "missing scopes", ExpiresAt: valid.ExpiresAt},
		{UserID: 1, Name: "too short", Scopes: valid.Scopes, ExpiresAt: now.Add(time.Hour)},
		{UserID: 1, Name: "too long", Scopes: valid.Scopes, ExpiresAt: now.Add(91 * 24 * time.Hour)},
	} {
		if err := token.Validate(now); err == nil {
			t.Fatalf("invalid token accepted: %#v", token)
		}
	}
}
