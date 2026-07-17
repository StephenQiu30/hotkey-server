package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
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
	user := &domain.User{
		Email:        fmt.Sprintf("identity-%s@example.test", suffix),
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

func newIdentitySession(userID int64, now time.Time) domain.Session {
	return domain.NewSession(userID, uuid.NewString(), now)
}
