package redis

import (
	"context"
	"errors"
	"fmt"
	"net"
	stdhttp "net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	goredis "github.com/redis/go-redis/v9"
)

const testVerificationHMACSecret = "verification-hmac-secret-for-tests-32-bytes"

func TestVerificationStoreUsesAtomicCodeAndTicketConsumption(t *testing.T) {
	store, err := NewVerificationStoreFromURL(testRedisURL(t), testVerificationHMACSecret)
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

func TestVerificationStoreHashesEmailInCodeStateAndKeepsTicketAssociationMinimal(t *testing.T) {
	store, err := NewVerificationStoreFromURL(testRedisURL(t), testVerificationHMACSecret)
	if err != nil {
		t.Fatalf("NewVerificationStoreFromURL(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	email := fmt.Sprintf("  Hash-Only-%d@Example.Test ", time.Now().UnixNano())
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		t.Fatalf("NormalizeEmail(): %v", err)
	}
	if err := store.CreateCode(ctx, domain.VerificationPurposeRegistration, email, "135790", time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateCode(): %v", err)
	}
	state, err := store.client.HGetAll(ctx, store.codeKey(domain.VerificationPurposeRegistration, normalized)).Result()
	if err != nil {
		t.Fatalf("inspect Redis code state: %v", err)
	}
	if state["email_hash"] != hashString(normalized) || state["email"] != "" {
		t.Fatalf("code state = %#v, want only normalized email hash", state)
	}
	for _, value := range state {
		if strings.Contains(value, normalized) {
			t.Fatalf("code state leaked normalized email in %q", value)
		}
	}
	ticket, err := store.ConsumeCode(ctx, domain.VerificationPurposeRegistration, email, "135790")
	if err != nil {
		t.Fatalf("ConsumeCode(): %v", err)
	}
	if ticket.Email != normalized {
		t.Fatalf("ticket email = %q, want normalized email", ticket.Email)
	}
	// A ticket must retain the normalized email because registration consumes
	// only the opaque ticket and needs the verified address; the code record
	// itself never needs plaintext email and therefore does not retain it.
	ticketState, err := store.client.HGetAll(ctx, store.ticketKey(ticket.Token)).Result()
	if err != nil {
		t.Fatalf("inspect Redis ticket state: %v", err)
	}
	if ticketState["email"] != normalized || ticketState["email_hash"] != hashString(normalized) {
		t.Fatalf("ticket state = %#v, want necessary email association plus hash", ticketState)
	}
}

func TestVerificationStoreDoesNotPersistUnkeyedVerificationCodeHash(t *testing.T) {
	store, err := NewVerificationStoreFromURL(testRedisURL(t), testVerificationHMACSecret)
	if err != nil {
		t.Fatalf("NewVerificationStoreFromURL(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	email := uniqueVerificationEmail("hmac")
	const code = "482913"
	if err := store.CreateCode(ctx, domain.VerificationPurposeRegistration, email, code, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateCode(): %v", err)
	}
	state, err := store.client.HGetAll(ctx, store.codeKey(domain.VerificationPurposeRegistration, email)).Result()
	if err != nil {
		t.Fatalf("inspect Redis code state: %v", err)
	}
	if state["code_hash"] == hashString(code) {
		t.Fatalf("Redis code_hash = unkeyed SHA-256(%q), want purpose/email-bound HMAC", code)
	}
	if state["code_hash"] != store.codeHMAC(domain.VerificationPurposeRegistration, email, code) {
		t.Fatalf("Redis code_hash = %q, want purpose/email-bound HMAC", state["code_hash"])
	}
	if _, err := store.ConsumeCode(ctx, domain.VerificationPurposeRegistration, email, code); err != nil {
		t.Fatalf("ConsumeCode() with correct code after state inspection: %v", err)
	}
}

func TestVerificationStoreCountsFailedCodesAndAllowsOnlyOneConcurrentConsumption(t *testing.T) {
	store, err := NewVerificationStoreFromURL(testRedisURL(t), testVerificationHMACSecret)
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
	store := NewVerificationStore(nil, testVerificationHMACSecret)
	assertVerificationStoreUnavailable(t, store)
}

func TestVerificationStoreReportsUnavailableForClosedRedisClientOnEveryOperation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve closed Redis address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved Redis address: %v", err)
	}
	store := NewVerificationStore(goredis.NewClient(&goredis.Options{Addr: address, DB: 15, DialTimeout: 25 * time.Millisecond, MaxRetries: 0}), testVerificationHMACSecret)
	t.Cleanup(func() { _ = store.Close() })
	assertVerificationStoreUnavailable(t, store)
}

func TestVerificationStoreConcurrentCodeAndTicketConsumptionHaveOneWinner(t *testing.T) {
	store, err := NewVerificationStoreFromURL(testRedisURL(t), testVerificationHMACSecret)
	if err != nil {
		t.Fatalf("NewVerificationStoreFromURL(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	email := uniqueVerificationEmail("concurrent")
	if err := store.CreateCode(ctx, domain.VerificationPurposeRegistration, email, "246810", time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateCode(): %v", err)
	}
	start := make(chan struct{})
	type codeOutcome struct {
		ticket domain.VerificationTicket
		err    error
	}
	codeResults := make(chan codeOutcome, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			ticket, err := store.ConsumeCode(ctx, domain.VerificationPurposeRegistration, email, "246810")
			codeResults <- codeOutcome{ticket: ticket, err: err}
		}()
	}
	close(start)
	group.Wait()
	close(codeResults)

	var ticket domain.VerificationTicket
	var codeSuccesses, codeInvalid int
	for result := range codeResults {
		if result.err == nil {
			codeSuccesses++
			ticket = result.ticket
			continue
		}
		if appErrorCode(result.err) != sharederrors.CodeVerificationInvalid {
			t.Fatalf("concurrent ConsumeCode() error = %v", result.err)
		}
		codeInvalid++
	}
	if codeSuccesses != 1 || codeInvalid != 1 {
		t.Fatalf("code outcomes = %d success %d invalid, want 1 each", codeSuccesses, codeInvalid)
	}

	start = make(chan struct{})
	ticketResults := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := store.ConsumeTicket(ctx, domain.VerificationPurposeRegistration, ticket.Token)
			ticketResults <- err
		}()
	}
	close(start)
	group.Wait()
	close(ticketResults)
	var ticketSuccesses, ticketInvalid int
	for err := range ticketResults {
		if err == nil {
			ticketSuccesses++
			continue
		}
		if appErrorCode(err) != sharederrors.CodeVerificationInvalid {
			t.Fatalf("concurrent ConsumeTicket() error = %v", err)
		}
		ticketInvalid++
	}
	if ticketSuccesses != 1 || ticketInvalid != 1 {
		t.Fatalf("ticket outcomes = %d success %d invalid, want 1 each", ticketSuccesses, ticketInvalid)
	}
}

func assertVerificationStoreUnavailable(t *testing.T, store *VerificationStore) {
	t.Helper()
	ctx := context.Background()
	if err := store.CreateCode(ctx, domain.VerificationPurposeRegistration, uniqueVerificationEmail("unavailable"), "123456", time.Now().Add(time.Minute)); appErrorCode(err) != sharederrors.CodeUnavailable {
		t.Fatalf("CreateCode() error = %v, want CodeUnavailable", err)
	}
	if _, err := store.ConsumeCode(ctx, domain.VerificationPurposeRegistration, uniqueVerificationEmail("unavailable-consume"), "123456"); appErrorCode(err) != sharederrors.CodeUnavailable {
		t.Fatalf("ConsumeCode() error = %v, want CodeUnavailable", err)
	}
	if _, err := store.ConsumeTicket(ctx, domain.VerificationPurposeRegistration, "opaque-ticket"); appErrorCode(err) != sharederrors.CodeUnavailable {
		t.Fatalf("ConsumeTicket() error = %v, want CodeUnavailable", err)
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
