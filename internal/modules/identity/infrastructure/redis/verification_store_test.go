package redis

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func TestVerificationStoreUsesAtomicCodeAndTicketConsumption(t *testing.T) {
	store, err := NewVerificationStoreFromURL(testRedisURL(t))
	if err != nil {
		t.Fatalf("NewVerificationStoreFromURL(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	email := uniqueVerificationEmail("atomic")
	if err := store.CreateCode(ctx, domain.VerificationPurposeRegistration, email, "123456", time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateCode(): %v", err)
	}
	if err := store.CreateCode(ctx, domain.VerificationPurposeRegistration, email, "654321", time.Now().Add(2*time.Minute)); appErrorCode(err) != sharederrors.CodeRateLimited {
		t.Fatalf("second CreateCode() error = %v, want rate limited", err)
	}

	ticket, err := store.ConsumeCode(ctx, domain.VerificationPurposeRegistration, email, "123456")
	if err != nil {
		t.Fatalf("ConsumeCode(): %v", err)
	}
	if ticket.Token == "" || ticket.Email != email || ticket.Purpose != domain.VerificationPurposeRegistration {
		t.Fatalf("ticket = %#v, want single-use registration ticket", ticket)
	}
	if _, err := store.ConsumeCode(ctx, domain.VerificationPurposeRegistration, email, "123456"); appErrorCode(err) != sharederrors.CodeVerificationInvalid {
		t.Fatalf("second ConsumeCode() error = %v, want verification invalid", err)
	}
	consumed, err := store.ConsumeTicket(ctx, domain.VerificationPurposeRegistration, ticket.Token)
	if err != nil {
		t.Fatalf("ConsumeTicket(): %v", err)
	}
	if consumed.Email != email || consumed.Purpose != domain.VerificationPurposeRegistration {
		t.Fatalf("consumed ticket = %#v, want original verification", consumed)
	}
	if _, err := store.ConsumeTicket(ctx, domain.VerificationPurposeRegistration, ticket.Token); appErrorCode(err) != sharederrors.CodeVerificationInvalid {
		t.Fatalf("replayed ConsumeTicket() error = %v, want verification invalid", err)
	}
}

func TestVerificationStoreCountsFailedCodesAndAllowsOnlyOneConcurrentConsumption(t *testing.T) {
	store, err := NewVerificationStoreFromURL(testRedisURL(t))
	if err != nil {
		t.Fatalf("NewVerificationStoreFromURL(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	email := uniqueVerificationEmail("failures")
	if err := store.CreateCode(ctx, domain.VerificationPurposePasswordReset, email, "987654", time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateCode(): %v", err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := store.ConsumeCode(ctx, domain.VerificationPurposePasswordReset, email, "000000"); appErrorCode(err) != sharederrors.CodeVerificationInvalid {
			t.Fatalf("failed ConsumeCode(%d) error = %v, want verification invalid", attempt, err)
		}
	}
	if _, err := store.ConsumeCode(ctx, domain.VerificationPurposePasswordReset, email, "987654"); appErrorCode(err) != sharederrors.CodeVerificationInvalid {
		t.Fatalf("locked ConsumeCode() error = %v, want verification invalid", err)
	}
}

func TestVerificationStoreReportsUnavailableForNilClient(t *testing.T) {
	store := NewVerificationStore(nil)
	err := store.CreateCode(context.Background(), domain.VerificationPurposeRegistration, "unavailable@example.test", "123456", time.Now().Add(time.Minute))
	if appErrorCode(err) != sharederrors.CodeUnavailable {
		t.Fatalf("CreateCode() error = %v, want CodeUnavailable", err)
	}
}

func appErrorCode(err error) int {
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) {
		return 0
	}
	if appError.HTTPStatus != stdhttp.StatusServiceUnavailable && appError.Code == sharederrors.CodeUnavailable {
		return 0
	}
	return appError.Code
}

func uniqueVerificationEmail(prefix string) string {
	return fmt.Sprintf("verification-%s-%d@example.test", prefix, time.Now().UnixNano())
}
