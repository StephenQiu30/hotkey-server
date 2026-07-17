package postgres

import (
	"context"
	"strings"
	"testing"
)

func TestAuditRepositoryRedactsCredentialsBeforePersisting(t *testing.T) {
	runtime := newIdentityRuntime(t)
	repository := NewAuditRepository(runtime)
	entry := AuditEntry{
		ActorType:    "user",
		ActorID:      42,
		Action:       "identity.password.changed",
		ResourceType: "user",
		ResourceID:   42,
		Result:       "success",
		BeforeData: map[string]any{
			"role":          "viewer",
			"email":         "private@example.test",
			"password_hash": "bcrypt-secret",
			"nested":        map[string]any{"refresh_token": "refresh-secret"},
		},
		AfterData: map[string]any{
			"role":              "admin",
			"verification_code": "123456",
		},
	}
	if err := repository.Create(context.Background(), entry); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	var before, after string
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(before_data::text, ''), COALESCE(after_data::text, '') FROM audit_logs WHERE action = $1`, entry.Action).Scan(&before, &after); err != nil {
		t.Fatalf("read audit data: %v", err)
	}
	for _, secret := range []string{"bcrypt-secret", "refresh-secret", "123456", "private@example.test"} {
		if strings.Contains(before, secret) || strings.Contains(after, secret) {
			t.Fatalf("audit persisted sensitive value %q: before=%s after=%s", secret, before, after)
		}
	}
	if !strings.Contains(before, "viewer") || !strings.Contains(after, "admin") {
		t.Fatalf("audit lost allowlisted state: before=%s after=%s", before, after)
	}
}
