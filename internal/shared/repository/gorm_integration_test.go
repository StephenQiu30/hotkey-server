package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database/model"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestGORMCRUDUsesMappedVersionSoftDeleteAndCursor(t *testing.T) {
	runtime := openRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	prefix := fmt.Sprintf("repository-%d", time.Now().UnixNano())
	defer func() {
		_, _ = runtime.SQL.Exec("DELETE FROM users WHERE email LIKE $1", prefix+"%")
	}()

	repo, err := NewGORMCRUD[model.User](runtime.GORM, "users")
	if err != nil {
		t.Fatalf("NewGORMCRUD() error = %v", err)
	}
	ctx := context.Background()
	created := make([]model.User, 0, 3)
	for number := 1; number <= 3; number++ {
		user := testUser(prefix, number)
		if err := repo.Create(ctx, &user); err != nil {
			t.Fatalf("Create(%d): %v", number, err)
		}
		if user.ID == 0 || user.Version != 1 {
			t.Fatalf("created user = %#v, want assigned id and version 1", user)
		}
		created = append(created, user)
	}

	first, err := repo.GetByID(ctx, created[0].ID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	first.DisplayName = "updated"
	if err := repo.Update(ctx, first); err != nil {
		t.Fatalf("Update(): %v", err)
	}
	if first.Version != 2 {
		t.Fatalf("updated version = %d, want 2", first.Version)
	}
	stale := *first
	stale.Version = 1
	if err := repo.Update(ctx, &stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale Update() error = %v, want conflict", err)
	}

	concurrentA, err := repo.GetByID(ctx, created[1].ID)
	if err != nil {
		t.Fatalf("load concurrent A: %v", err)
	}
	concurrentB, err := repo.GetByID(ctx, created[1].ID)
	if err != nil {
		t.Fatalf("load concurrent B: %v", err)
	}
	concurrentA.DisplayName = "concurrent-a"
	concurrentB.DisplayName = "concurrent-b"
	start := make(chan struct{})
	errorsByUpdate := make(chan error, 2)
	var updates sync.WaitGroup
	for _, candidate := range []*model.User{concurrentA, concurrentB} {
		updates.Add(1)
		go func(candidate *model.User) {
			defer updates.Done()
			<-start
			errorsByUpdate <- repo.Update(ctx, candidate)
		}(candidate)
	}
	close(start)
	updates.Wait()
	close(errorsByUpdate)
	var successes, conflicts int
	for updateErr := range errorsByUpdate {
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent Update() error = %v", updateErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent update outcomes = %d success, %d conflict; want 1 each", successes, conflicts)
	}

	page, err := repo.List(ctx, PageQuery{Limit: 2, Sort: "id", FilterFingerprint: "all-users"})
	if err != nil {
		t.Fatalf("first List(): %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("first page = %#v, want two items and next cursor", page)
	}
	next, err := repo.List(ctx, PageQuery{Limit: 2, Sort: "id", FilterFingerprint: "all-users", Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("next List(): %v", err)
	}
	if len(next.Items) == 0 || next.Items[0].ID <= page.Items[len(page.Items)-1].ID {
		t.Fatalf("next page = %#v, want records strictly after first page cursor", next)
	}
	if _, err := repo.List(ctx, PageQuery{Limit: 2, Sort: "id", FilterFingerprint: "other-filter", Cursor: page.NextCursor}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-filter cursor error = %v, want invalid input", err)
	}
	if _, err := repo.List(ctx, PageQuery{Limit: 2, Sort: "id", Descending: true, FilterFingerprint: "all-users", Cursor: page.NextCursor}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-direction cursor error = %v, want invalid input", err)
	}

	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete(): %v", err)
	}
	if _, err := repo.GetByID(ctx, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted GetByID() error = %v, want not found", err)
	}

	if _, err := NewGORMHistory[model.AuditLog](runtime.GORM, "audit_logs"); err != nil {
		t.Fatalf("NewGORMHistory(audit_logs): %v", err)
	}
	if _, err := NewGORMHistory[model.User](runtime.GORM, "users"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NewGORMHistory(users) error = %v, want invalid input", err)
	}
}

func TestMapErrorUsesStableCategories(t *testing.T) {
	for _, test := range []struct {
		code string
		want error
	}{
		{code: "23505", want: ErrConflict},
		{code: "40001", want: ErrConflict},
		{code: "23503", want: ErrConstraint},
		{code: "23514", want: ErrConstraint},
		{code: "57014", want: ErrUnavailable},
	} {
		if got := MapError(&pgconn.PgError{Code: test.code}); !errors.Is(got, test.want) {
			t.Errorf("MapError(%s) = %v, want %v", test.code, got, test.want)
		}
	}
	for _, contextErr := range []error{context.Canceled, context.DeadlineExceeded} {
		if got := MapError(contextErr); !errors.Is(got, ErrUnavailable) {
			t.Errorf("MapError(%v) = %v, want unavailable", contextErr, got)
		}
	}
}

func testUser(prefix string, number int) model.User {
	return model.User{
		Email:        fmt.Sprintf("%s-%d@example.test", prefix, number),
		PasswordHash: "hash",
		DisplayName:  fmt.Sprintf("user-%d", number),
		Role:         "viewer",
		Status:       "active",
	}
}

func openRepositoryRuntime(t *testing.T) *database.Runtime {
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
	return runtime
}
