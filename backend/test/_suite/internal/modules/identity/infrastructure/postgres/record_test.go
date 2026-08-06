package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	"github.com/google/uuid"
)

func newIdentityRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func createIdentityUser(t *testing.T, repository *UserRepository, suffix string) *domain.User {
	t.Helper()
	safeSuffix := strings.Map(func(character rune) rune {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9', character == '-':
			return character
		default:
			return '-'
		}
	}, suffix)
	user := &domain.User{
		Email:        fmt.Sprintf("identity-%s@example.test", safeSuffix),
		PasswordHash: "bcrypt-hash",
		DisplayName:  "Identity User",
		Role:         domain.RoleViewer,
		Status:       domain.UserStatusActive,
	}
	if err := repository.Create(context.Background(), user); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	return user
}

func createIdentityAdmin(t *testing.T, runtime *database.Runtime, repository *UserRepository, suffix string) *domain.User {
	t.Helper()
	user := createIdentityUser(t, repository, suffix)
	if _, err := runtime.SQL.Exec(`UPDATE users SET role = 'admin', version = version + 1, updated_at = now() WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("promote test administrator: %v", err)
	}
	user.Role = domain.RoleAdmin
	return user
}

func newIdentitySession(userID int64, now time.Time) domain.Session {
	return domain.NewSession(userID, uuid.NewString(), now)
}
