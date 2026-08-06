package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestInitialAdministratorDatabaseOperation(t *testing.T) {
	t.Run("success writes attributable audit", func(t *testing.T) {
		runtime := newIdentityRuntime(t)
		repository := NewUserRepository(runtime)
		target := createIdentityUser(t, repository, "initial-admin-success")
		const ticket = "HK-2301"

		executeOperationSQL(t, runtime, initialAdministratorSQL(t, target.Email, ticket))
		assertUserRole(t, runtime, target.ID, domain.RoleAdmin)

		var actorType, requestID, beforeRole, afterRole, principal, currentUser string
		if err := runtime.SQL.QueryRow(`
SELECT actor_type, request_id, before_data->>'role', after_data->>'role', after_data->>'database_principal'
FROM audit_logs
WHERE action = 'identity.initial_admin.designated' AND resource_id = $1`, target.ID).Scan(
			&actorType, &requestID, &beforeRole, &afterRole, &principal,
		); err != nil {
			t.Fatalf("read initial administrator audit: %v", err)
		}
		if err := runtime.SQL.QueryRow(`SELECT current_user`).Scan(&currentUser); err != nil {
			t.Fatalf("read database principal: %v", err)
		}
		if actorType != "database_operator" || requestID != ticket || beforeRole != "viewer" || afterRole != "admin" || principal != currentUser {
			t.Fatalf("audit = actor %q request %q before %q after %q principal %q, want database operator, ticket, viewer/admin, %q", actorType, requestID, beforeRole, afterRole, principal, currentUser)
		}
	})

	for _, scenario := range []string{"missing user", "disabled user", "deleted user", "editor user", "existing active administrator"} {
		scenario := scenario
		t.Run("refuses "+scenario, func(t *testing.T) {
			runtime := newIdentityRuntime(t)
			repository := NewUserRepository(runtime)
			target := createIdentityUser(t, repository, "initial-admin-refused")
			email := target.Email
			switch scenario {
			case "missing user":
				email = "missing-initial-admin@example.test"
			case "disabled user":
				mustExecIdentitySQL(t, runtime, `UPDATE users SET status = 'disabled' WHERE id = $1`, target.ID)
			case "deleted user":
				mustExecIdentitySQL(t, runtime, `UPDATE users SET deleted_at = now() WHERE id = $1`, target.ID)
			case "editor user":
				mustExecIdentitySQL(t, runtime, `UPDATE users SET role = 'editor' WHERE id = $1`, target.ID)
			case "existing active administrator":
				createIdentityAdmin(t, runtime, repository, "already-admin")
			}

			if _, err := runtime.SQL.Exec(initialAdministratorSQL(t, email, "HK-2302")); err == nil {
				t.Fatalf("database operation accepted %s", scenario)
			}
			var auditCount int
			if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'identity.initial_admin.designated'`).Scan(&auditCount); err != nil {
				t.Fatalf("count initial administrator audit: %v", err)
			}
			if auditCount != 0 {
				t.Fatalf("failed operation wrote %d audit rows, want 0", auditCount)
			}
			var role domain.Role
			if err := runtime.SQL.QueryRow(`SELECT role FROM users WHERE id = $1`, target.ID).Scan(&role); err != nil {
				t.Fatalf("read refused target role: %v", err)
			}
			if role == domain.RoleAdmin {
				t.Fatalf("failed operation promoted refused target %d", target.ID)
			}
		})
	}

	t.Run("requires a real change ticket", func(t *testing.T) {
		runtime := newIdentityRuntime(t)
		repository := NewUserRepository(runtime)
		target := createIdentityUser(t, repository, "initial-admin-ticket-required")
		script := strings.ReplaceAll(operationSQLBlock(t, "# 首管理员数据库指定操作"), "admin@example.com", target.Email)
		if _, err := runtime.SQL.Exec(script); err == nil {
			t.Fatal("database operation accepted the change-ticket placeholder")
		}
		assertUserRole(t, runtime, target.ID, domain.RoleViewer)
	})

	t.Run("concurrent targets produce exactly one administrator", func(t *testing.T) {
		runtime := newIdentityRuntime(t)
		repository := NewUserRepository(runtime)
		first := createIdentityUser(t, repository, "initial-admin-concurrent-one")
		second := createIdentityUser(t, repository, "initial-admin-concurrent-two")
		scripts := []string{
			initialAdministratorSQL(t, first.Email, "HK-2310"),
			initialAdministratorSQL(t, second.Email, "HK-2311"),
		}

		results := make(chan error, 2)
		for _, script := range scripts {
			script := script
			go func() {
				_, err := runtime.SQL.Exec(script)
				results <- err
			}()
		}
		successes := 0
		for range 2 {
			if err := <-results; err == nil {
				successes++
			}
		}
		var administrators, audits int
		if err := runtime.SQL.QueryRow(`SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active' AND deleted_at IS NULL`).Scan(&administrators); err != nil {
			t.Fatalf("count administrators: %v", err)
		}
		if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'identity.initial_admin.designated'`).Scan(&audits); err != nil {
			t.Fatalf("count initial administrator audits: %v", err)
		}
		if successes != 1 || administrators != 1 || audits != 1 {
			t.Fatalf("concurrent result successes=%d administrators=%d audits=%d, want 1/1/1", successes, administrators, audits)
		}
	})

	t.Run("audit failure rolls role change back", func(t *testing.T) {
		runtime := newIdentityRuntime(t)
		repository := NewUserRepository(runtime)
		target := createIdentityUser(t, repository, "initial-admin-audit-rollback")
		mustExecIdentitySQL(t, runtime, `
CREATE FUNCTION reject_initial_admin_audit_for_test() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.action = 'identity.initial_admin.designated' THEN
        RAISE EXCEPTION 'reject test audit';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER reject_initial_admin_audit_for_test
BEFORE INSERT ON audit_logs
FOR EACH ROW EXECUTE FUNCTION reject_initial_admin_audit_for_test()`)

		if _, err := runtime.SQL.Exec(initialAdministratorSQL(t, target.Email, "HK-2303")); err == nil {
			t.Fatal("database operation succeeded despite rejected audit")
		}
		assertUserRole(t, runtime, target.ID, domain.RoleViewer)
	})

	t.Run("explicit rollback leaves no role or audit change", func(t *testing.T) {
		runtime := newIdentityRuntime(t)
		repository := NewUserRepository(runtime)
		target := createIdentityUser(t, repository, "initial-admin-explicit-rollback")
		script := initialAdministratorSQL(t, target.Email, "HK-2304")
		lastCommit := strings.LastIndex(script, "COMMIT;")
		if lastCommit < 0 {
			t.Fatal("operation SQL is missing COMMIT")
		}
		script = script[:lastCommit] + "ROLLBACK;" + script[lastCommit+len("COMMIT;"):]
		executeOperationSQL(t, runtime, script)
		assertUserRole(t, runtime, target.ID, domain.RoleViewer)
		var audits int
		if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'identity.initial_admin.designated'`).Scan(&audits); err != nil {
			t.Fatalf("count rolled-back audit rows: %v", err)
		}
		if audits != 0 {
			t.Fatalf("explicit rollback retained %d audit rows, want 0", audits)
		}
	})
}

func TestInitialAdministratorBreakGlassOperation(t *testing.T) {
	t.Run("atomically corrects mistaken administrator", func(t *testing.T) {
		runtime := newIdentityRuntime(t)
		repository := NewUserRepository(runtime)
		mistaken := createIdentityAdmin(t, runtime, repository, "mistaken-admin")
		correct := createIdentityUser(t, repository, "correct-admin")

		executeOperationSQL(t, runtime, breakGlassAdministratorSQL(t, mistaken.Email, correct.Email, "HK-2310"))
		assertUserRole(t, runtime, mistaken.ID, domain.RoleViewer)
		assertUserRole(t, runtime, correct.ID, domain.RoleAdmin)
		var audits int
		if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'identity.initial_admin.corrected' AND request_id = 'HK-2310'`).Scan(&audits); err != nil {
			t.Fatalf("count break-glass audits: %v", err)
		}
		if audits != 2 {
			t.Fatalf("break-glass audit rows = %d, want 2", audits)
		}
	})

	t.Run("invalid correction rolls every change back", func(t *testing.T) {
		runtime := newIdentityRuntime(t)
		repository := NewUserRepository(runtime)
		mistaken := createIdentityAdmin(t, runtime, repository, "mistaken-admin-rollback")
		correct := createIdentityUser(t, repository, "correct-admin-rollback")

		if _, err := runtime.SQL.Exec(breakGlassAdministratorSQL(t, mistaken.Email, "missing-correct-admin@example.test", "HK-2311")); err == nil {
			t.Fatal("break-glass operation accepted a missing correct user")
		}
		assertUserRole(t, runtime, mistaken.ID, domain.RoleAdmin)
		assertUserRole(t, runtime, correct.ID, domain.RoleViewer)
		var audits int
		if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'identity.initial_admin.corrected'`).Scan(&audits); err != nil {
			t.Fatalf("count failed break-glass audits: %v", err)
		}
		if audits != 0 {
			t.Fatalf("failed break-glass retained %d audit rows, want 0", audits)
		}
	})
}

func initialAdministratorSQL(t *testing.T, email, ticket string) string {
	t.Helper()
	script := operationSQLBlock(t, "# 首管理员数据库指定操作")
	return strings.ReplaceAll(strings.ReplaceAll(script, "admin@example.com", email), "CHANGE-TICKET-ID", ticket)
}

func breakGlassAdministratorSQL(t *testing.T, mistakenEmail, correctEmail, ticket string) string {
	t.Helper()
	script := operationSQLBlock(t, "## Break-glass：纠正误指定")
	script = strings.ReplaceAll(script, "wrong-admin@example.com", mistakenEmail)
	script = strings.ReplaceAll(script, "correct-admin@example.com", correctEmail)
	return strings.ReplaceAll(script, "CHANGE-TICKET-ID", ticket)
}

func operationSQLBlock(t *testing.T, heading string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "initial-admin-operation.md"))
	if err != nil {
		t.Fatalf("read initial administrator operation fixture: %v", err)
	}
	section := string(content)
	headingIndex := strings.Index(section, heading)
	if headingIndex < 0 {
		t.Fatalf("operation heading %q is missing", heading)
	}
	section = section[headingIndex+len(heading):]
	start := strings.Index(section, "```sql")
	if start < 0 {
		t.Fatalf("SQL fence after %q is missing", heading)
	}
	section = section[start+len("```sql"):]
	end := strings.Index(section, "```")
	if end < 0 {
		t.Fatalf("SQL fence after %q is not closed", heading)
	}
	return strings.TrimSpace(section[:end])
}

func executeOperationSQL(t *testing.T, runtime *database.Runtime, script string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := runtime.SQL.ExecContext(ctx, script); err != nil {
		t.Fatalf("execute documented database operation: %v", err)
	}
}

func mustExecIdentitySQL(t *testing.T, runtime *database.Runtime, statement string, arguments ...any) {
	t.Helper()
	if _, err := runtime.SQL.Exec(statement, arguments...); err != nil {
		t.Fatalf("execute identity fixture SQL: %v", err)
	}
}

func assertUserRole(t *testing.T, runtime *database.Runtime, userID int64, expected domain.Role) {
	t.Helper()
	var role domain.Role
	if err := runtime.SQL.QueryRow(`SELECT role FROM users WHERE id = $1`, userID).Scan(&role); err != nil {
		t.Fatalf("read user role: %v", err)
	}
	if role != expected {
		t.Fatalf("user %d role = %q, want %q", userID, role, expected)
	}
}
