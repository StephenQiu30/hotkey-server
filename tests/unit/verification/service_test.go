package verification_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	"github.com/StephenQiu30/hotkey-server/tests/testutil"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type fakeCodeGen struct {
	code string
}

func (g *fakeCodeGen) Generate() string { return g.code }

type fakeMailer struct {
	mu     sync.Mutex
	sent   []mailMsg
	fail   bool
	failCh chan struct{}
}

type mailMsg struct {
	to, subject, body string
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return "", errors.New("smtp send failed")
	}
	m.sent = append(m.sent, mailMsg{to: to, subject: subject, body: body})
	if m.failCh != nil {
		close(m.failCh)
	}
	return "msg-1", nil
}

func (m *fakeMailer) SentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func (m *fakeMailer) LastSent() *mailMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return nil
	}
	return &m.sent[len(m.sent)-1]
}

type fakeUserLookup struct {
	exists map[string]bool
}

func (l *fakeUserLookup) ExistsByEmail(_ context.Context, email string) bool {
	return l.exists[email]
}

// ---------------------------------------------------------------------------
// Test store — direct Redis access for assertions
// ---------------------------------------------------------------------------

type testStore struct {
	rdb    *redis.Client
	pepper string
}

func newTestStore(rdb *redis.Client, pepper string) *testStore {
	return &testStore{rdb: rdb, pepper: pepper}
}

func (s *testStore) CodeExists(ctx context.Context, email, purpose string) bool {
	hmac := service.HMACEmail(email, s.pepper)
	key := service.CodeKey(purpose, hmac)
	err := s.rdb.Get(ctx, key).Err()
	return err == nil
}

func (s *testStore) LockExists(ctx context.Context, email, purpose string) bool {
	hmac := service.HMACEmail(email, s.pepper)
	key := service.LockKey(purpose, hmac)
	err := s.rdb.Get(ctx, key).Err()
	return err == nil
}

func (s *testStore) FailCount(ctx context.Context, email, purpose string) int64 {
	hmac := service.HMACEmail(email, s.pepper)
	key := service.FailCountKey(purpose, hmac)
	n, err := s.rdb.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return n
}

func (s *testStore) TicketExists(ctx context.Context, ticket string) bool {
	key := service.TicketKey(ticket)
	err := s.rdb.Get(ctx, key).Err()
	return err == nil
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type harness struct {
	svc   *service.VerificationService
	store *testStore
	rdb   *redis.Client
	clock *fakeClock
	mail  *fakeMailer
	code  *fakeCodeGen
	users *fakeUserLookup
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	rdb := testutil.NewTestRedis(t)
	t.Cleanup(func() { testutil.CleanupTestRedis(t, rdb) })
	testutil.FlushTestRedis(t, rdb)

	clock := &fakeClock{now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	mail := &fakeMailer{}
	code := &fakeCodeGen{code: "123456"}
	users := &fakeUserLookup{exists: map[string]bool{"existing@example.com": true}}
	pepper := "test-verification-pepper-2026"

	svc := service.NewVerificationService(rdb, pepper, mail, clock, code, users)
	store := newTestStore(rdb, pepper)

	return &harness{
		svc:   svc,
		store: store,
		rdb:   rdb,
		clock: clock,
		mail:  mail,
		code:  code,
		users: users,
	}
}

func sendCode(t *testing.T, h *harness, email, purpose, ip string) {
	t.Helper()
	err := h.svc.SendVerificationCode(context.Background(), dto.VerificationSendInput{
		Email: email, Purpose: purpose, IP: ip,
	})
	if err != nil {
		t.Fatalf("SendVerificationCode(%s, %s): %v", email, purpose, err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSendAndConfirmHappyPath(t *testing.T) {
	h := newHarness(t)

	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.1")

	if !h.store.CodeExists(context.Background(), "existing@example.com", string(enum.VerificationPurposeRegister)) {
		t.Fatal("code should exist after send")
	}
	if h.mail.SentCount() != 1 {
		t.Fatalf("expected 1 email sent, got %d", h.mail.SentCount())
	}

	ticket, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "123456",
	})
	if err != nil {
		t.Fatalf("ConfirmCode: %v", err)
	}
	if ticket == "" {
		t.Fatal("expected non-empty ticket")
	}
	if h.store.CodeExists(context.Background(), "existing@example.com", string(enum.VerificationPurposeRegister)) {
		t.Fatal("code should be deleted after successful confirm")
	}
}

func TestWrongCodeReturnsError(t *testing.T) {
	h := newHarness(t)

	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.1")

	_, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "000000",
	})
	if !errors.Is(err, service.VerificationErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode, got %v", err)
	}
}

func TestResendLockPreventsRapidResends(t *testing.T) {
	h := newHarness(t)

	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.1")

	// Immediate resend should be locked
	err := h.svc.SendVerificationCode(context.Background(), dto.VerificationSendInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), IP: "127.0.0.1",
	})
	if !errors.Is(err, service.VerificationErrLocked) {
		t.Fatalf("expected ErrLocked for rapid resend, got %v", err)
	}
	if h.mail.SentCount() != 1 {
		t.Fatalf("expected 1 email sent, got %d", h.mail.SentCount())
	}

	// After lock expires, resend works
	h.clock.Advance(61 * time.Second)
	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.1")
	if h.mail.SentCount() != 2 {
		t.Fatalf("expected 2 emails sent, got %d", h.mail.SentCount())
	}
}

func TestSendLimitExceeded(t *testing.T) {
	h := newHarness(t)

	// Send 5 times quickly (advancing lock each time)
	for i := 0; i < 5; i++ {
		sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.2")
		h.clock.Advance(61 * time.Second)
	}

	// 6th send should be rate-limited
	err := h.svc.SendVerificationCode(context.Background(), dto.VerificationSendInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), IP: "127.0.0.2",
	})
	if !errors.Is(err, service.VerificationErrSendLimit) {
		t.Fatalf("expected ErrSendLimit, got %v", err)
	}
}

func TestIPSendLimitExceeded(t *testing.T) {
	h := newHarness(t)

	// Send 20 times from same IP, different emails
	emailPool := make([]string, 20)
	for i := 0; i < 20; i++ {
		emailPool[i] = fmt.Sprintf("user%d@example.com", i)
	}
	for i := 0; i < 20; i++ {
		sendCode(t, h, emailPool[i], string(enum.VerificationPurposeRegister), "10.0.0.1")
		h.clock.Advance(61 * time.Second)
	}

	// 21st send from same IP should be rate-limited
	err := h.svc.SendVerificationCode(context.Background(), dto.VerificationSendInput{
		Email: "overflow@example.com", Purpose: string(enum.VerificationPurposeRegister), IP: "10.0.0.1",
	})
	if !errors.Is(err, service.VerificationErrIPLimit) {
		t.Fatalf("expected ErrIPLimit, got %v", err)
	}
}

func TestFifthWrongCodeDeletesVerification(t *testing.T) {
	h := newHarness(t)

	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.3")

	for i := 0; i < 5; i++ {
		_, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
			Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "000000",
		})
		if i < 4 && !errors.Is(err, service.VerificationErrInvalidCode) {
			t.Fatalf("attempt %d: expected ErrInvalidCode, got %v", i+1, err)
		}
	}

	// Code should be deleted after 5 failed attempts
	if h.store.CodeExists(context.Background(), "existing@example.com", string(enum.VerificationPurposeRegister)) {
		t.Fatal("code still exists after 5 wrong attempts")
	}
}

func TestTicketOneTimeUse(t *testing.T) {
	h := newHarness(t)

	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.4")

	ticket, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "123456",
	})
	if err != nil {
		t.Fatalf("ConfirmCode: %v", err)
	}

	// Claim the ticket
	data, err := h.svc.ClaimTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}
	if data == nil {
		t.Fatal("expected ticket data")
	}
	if data.Email != "existing@example.com" {
		t.Fatalf("expected email existing@example.com, got %s", data.Email)
	}
	if string(data.Purpose) != string(enum.VerificationPurposeRegister) {
		t.Fatalf("expected purpose register, got %s", data.Purpose)
	}

	// Second claim should fail (already claimed)
	_, err = h.svc.ClaimTicket(context.Background(), ticket)
	if !errors.Is(err, service.VerificationErrTicketClaimed) {
		t.Fatalf("expected ErrTicketClaimed, got %v", err)
	}
}

func TestTicketCompleteAndRelease(t *testing.T) {
	h := newHarness(t)

	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.5")

	ticket, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "123456",
	})
	if err != nil {
		t.Fatalf("ConfirmCode: %v", err)
	}

	// Claim
	_, err = h.svc.ClaimTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}

	// Complete
	err = h.svc.CompleteTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("CompleteTicket: %v", err)
	}

	// Claim after complete should fail
	_, err = h.svc.ClaimTicket(context.Background(), ticket)
	if !errors.Is(err, service.VerificationErrTicketClaimed) {
		t.Fatalf("expected ErrTicketClaimed after complete, got %v", err)
	}
}

func TestReleaseTicket(t *testing.T) {
	h := newHarness(t)

	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.6")

	ticket, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "123456",
	})
	if err != nil {
		t.Fatalf("ConfirmCode: %v", err)
	}

	// Claim
	_, err = h.svc.ClaimTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ClaimTicket: %v", err)
	}

	// Release
	err = h.svc.ReleaseTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ReleaseTicket: %v", err)
	}

	// Re-claim should work after release
	_, err = h.svc.ClaimTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ClaimTicket after release: %v", err)
	}
}

func TestSeparatePurposeIsolation(t *testing.T) {
	h := newHarness(t)

	// Send code for register
	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeRegister), "127.0.0.7")

	// Confirm with wrong purpose should fail with NotFound (different key namespace)
	_, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeResetPassword), Code: "123456",
	})
	if !errors.Is(err, service.VerificationErrNotFound) {
		t.Fatalf("expected ErrNotFound for different purpose, got %v", err)
	}

	// Send code for reset_password
	sendCode(t, h, "existing@example.com", string(enum.VerificationPurposeResetPassword), "127.0.0.7")

	// Confirm register still works
	ticket, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "123456",
	})
	if err != nil {
		t.Fatalf("ConfirmCode for register: %v", err)
	}
	if ticket == "" {
		t.Fatal("expected non-empty ticket")
	}

	// Confirm reset works too
	h.code.code = "123456"
	ticket2, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeResetPassword), Code: "123456",
	})
	if err != nil {
		t.Fatalf("ConfirmCode for reset: %v", err)
	}
	if ticket2 == "" {
		t.Fatal("expected non-empty ticket for reset")
	}

	// Tickets should be different
	if ticket == ticket2 {
		t.Fatal("expected different tickets for different purposes")
	}
}

func TestUnknownEmailPasswordResetGenericSuccess(t *testing.T) {
	h := newHarness(t)

	// Send code for password reset to an unknown email
	err := h.svc.SendVerificationCode(context.Background(), dto.VerificationSendInput{
		Email: "unknown@example.com", Purpose: string(enum.VerificationPurposeResetPassword), IP: "127.0.0.8",
	})
	if err != nil {
		t.Fatalf("expected success (generic) for unknown email, got %v", err)
	}

	// No email should have been sent
	if h.mail.SentCount() != 0 {
		t.Fatalf("expected no email sent for unknown email, got %d", h.mail.SentCount())
	}

	// Code should NOT have been stored (we shouldn't waste resources)
	if h.store.CodeExists(context.Background(), "unknown@example.com", string(enum.VerificationPurposeResetPassword)) {
		t.Fatal("code should not be stored for unknown email password reset")
	}
}

func TestUnknownEmailPasswordResetNoLock(t *testing.T) {
	h := newHarness(t)

	// Send code for password reset to an unknown email - should succeed silently
	err := h.svc.SendVerificationCode(context.Background(), dto.VerificationSendInput{
		Email: "ghost@example.com", Purpose: string(enum.VerificationPurposeResetPassword), IP: "127.0.0.9",
	})
	if err != nil {
		t.Fatalf("expected success for unknown email, got %v", err)
	}

	// No lock should be stored either
	if h.store.LockExists(context.Background(), "ghost@example.com", string(enum.VerificationPurposeResetPassword)) {
		t.Fatal("lock should not be stored for unknown email")
	}
}

func TestRedisFailureFailsClosed(t *testing.T) {
	h := newHarness(t)

	// Close Redis to simulate failure (testutil.Cleanup in t.Cleanup will handle close)
	h.rdb.Close()

	// Send should fail
	err := h.svc.SendVerificationCode(context.Background(), dto.VerificationSendInput{
		Email: "existing@example.com", Purpose: string(enum.VerificationPurposeRegister), IP: "127.0.0.99",
	})
	if err == nil {
		t.Fatal("expected error when Redis is down")
	}
}

func TestConfirmCodeNoCodeStored(t *testing.T) {
	h := newHarness(t)

	_, err := h.svc.ConfirmCode(context.Background(), dto.VerificationConfirmInput{
		Email: "nobody@example.com", Purpose: string(enum.VerificationPurposeRegister), Code: "123456",
	})
	if !errors.Is(err, service.VerificationErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTicketNotFound(t *testing.T) {
	h := newHarness(t)

	_, err := h.svc.ClaimTicket(context.Background(), "nonexistent-ticket")
	if !errors.Is(err, service.VerificationErrTicketNotFound) {
		t.Fatalf("expected ErrTicketNotFound, got %v", err)
	}

	err = h.svc.CompleteTicket(context.Background(), "nonexistent-ticket")
	if !errors.Is(err, service.VerificationErrTicketNotFound) {
		t.Fatalf("expected ErrTicketNotFound, got %v", err)
	}

	err = h.svc.ReleaseTicket(context.Background(), "nonexistent-ticket")
	if !errors.Is(err, service.VerificationErrTicketNotFound) {
		t.Fatalf("expected ErrTicketNotFound, got %v", err)
	}
}
