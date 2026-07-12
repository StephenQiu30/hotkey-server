package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
)

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	VerificationErrNotFound       = errors.New("verification code not found")
	VerificationErrInvalidCode    = errors.New("invalid verification code")
	VerificationErrLocked         = errors.New("resend locked, try again later")
	VerificationErrSendLimit      = errors.New("send limit exceeded")
	VerificationErrIPLimit        = errors.New("IP send limit exceeded")
	VerificationErrTicketNotFound = errors.New("ticket not found")
	VerificationErrTicketClaimed  = errors.New("ticket already claimed")
	VerificationErrRedisDown      = errors.New("verification service unavailable")
)

// ---------------------------------------------------------------------------
// Pluggable interfaces
// ---------------------------------------------------------------------------

// Clock abstracts time.Now for testability.
type Clock interface {
	Now() time.Time
}

// CodeGenerator abstracts 6-digit code generation for testability.
type CodeGenerator interface {
	Generate() string
}

// UserLookup is the minimal interface for checking user existence.
type UserLookup interface {
	ExistsByEmail(ctx context.Context, email string) bool
}

// ---------------------------------------------------------------------------
// Ticket data
// ---------------------------------------------------------------------------

// TicketData describes the payload carried by a one-time verification ticket.
type TicketData struct {
	Email   string                   `json:"email"`
	Purpose enum.VerificationPurpose `json:"purpose"`
	Status  string                   `json:"status"` // ready, processing, completed
}

// ---------------------------------------------------------------------------
// Redis key helpers (exported for test inspection)
// ---------------------------------------------------------------------------

const (
	codeTTL      = 10 * time.Minute
	lockDuration = 60 * time.Second
	ticketTTL    = 10 * time.Minute
	completedTTL = 2 * time.Minute
)

const (
	emailSendLimit    = 5
	ipSendLimit       = 20
	maxFailedAttempts = 5
)

// HMACEmail derives a safe key fragment from an email address using HMAC-SHA256
// and the configured pepper. This keeps raw email addresses out of Redis keys.
func HMACEmail(email, pepper string) string {
	return security.HMACDigest(email, pepper)
}

// CodeKey returns the Redis key for a verification code.
func CodeKey(purpose, hmac string) string {
	return fmt.Sprintf("ver:code:%s:%s", purpose, hmac)
}

// LockKey returns the Redis key for the resend lock.
func LockKey(purpose, hmac string) string {
	return fmt.Sprintf("ver:lock:%s:%s", purpose, hmac)
}

// SendCountKey returns the Redis key for per-email send counters.
func SendCountKey(purpose, hmac, hour string) string {
	return fmt.Sprintf("ver:send:%s:%s:%s", purpose, hmac, hour)
}

// SendIPCountKey returns the Redis key for per-IP send counters.
func SendIPCountKey(purpose, ip, hour string) string {
	return fmt.Sprintf("ver:send_ip:%s:%s:%s", purpose, ip, hour)
}

// FailCountKey returns the Redis key for failed attempt counters.
func FailCountKey(purpose, hmac string) string {
	return fmt.Sprintf("ver:fail:%s:%s", purpose, hmac)
}

// TicketKey returns the Redis key for a ticket.
func TicketKey(ticket string) string {
	return fmt.Sprintf("ver:ticket:%s", ticket)
}

func hourBucket(t time.Time) string {
	return t.Format("2006010215")
}

func purposeLabel(purpose string) string {
	switch purpose {
	case string(enum.VerificationPurposeRegister):
		return "registration"
	case string(enum.VerificationPurposeResetPassword):
		return "password reset"
	default:
		return purpose
	}
}

func newTicketID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Lua scripts
// ---------------------------------------------------------------------------

// scriptFailedAttempt: KEYS[1]=failCountKey, KEYS[2]=codeKey
// Returns "deleted" if code was removed (>=5 failures), "ok" otherwise.
var scriptFailedAttempt = redis.NewScript(fmt.Sprintf(`
	local fail = redis.call('INCR', KEYS[1])
	redis.call('EXPIRE', KEYS[1], 600)
	if tonumber(fail) >= %d then
		redis.call('DEL', KEYS[2])
		redis.call('DEL', KEYS[1])
		return 'deleted'
	end
	return 'ok'
`, maxFailedAttempts))

// scriptConsumeCode: KEYS[1]=codeKey, KEYS[2]=ticketKey
// ARGV[1]=email, ARGV[2]=purpose, ARGV[3]=ticketTTL seconds.
// Returns "ok" on success, "not_found" if no code exists.
var scriptConsumeCode = redis.NewScript(`
	local code = redis.call('GETDEL', KEYS[1])
	if not code then
		return 'not_found'
	end
	redis.call('HMSET', KEYS[2], 'email', ARGV[1], 'purpose', ARGV[2], 'status', 'ready')
	redis.call('EXPIRE', KEYS[2], ARGV[3])
	return 'ok'
`)

// scriptClaimTicket: KEYS[1]=ticketKey, ARGV[1]=ttl seconds.
// Returns "ok", "not_found", or "claimed".
var scriptClaimTicket = redis.NewScript(`
	local status = redis.call('HGET', KEYS[1], 'status')
	if not status then
		return 'not_found'
	end
	if status ~= 'ready' then
		return 'claimed'
	end
	redis.call('HSET', KEYS[1], 'status', 'processing')
	redis.call('EXPIRE', KEYS[1], ARGV[1])
	return 'ok'
`)

// scriptCompleteTicket: KEYS[1]=ticketKey, ARGV[1]=ttl seconds.
// Returns "ok" or "not_found".
var scriptCompleteTicket = redis.NewScript(`
	local status = redis.call('HGET', KEYS[1], 'status')
	if not status then
		return 'not_found'
	end
	redis.call('HSET', KEYS[1], 'status', 'completed')
	redis.call('EXPIRE', KEYS[1], ARGV[1])
	return 'ok'
`)

// scriptReleaseTicket: KEYS[1]=ticketKey, ARGV[1]=ttl seconds.
// Returns "ok", "not_found", or "invalid" (not in processing state).
var scriptReleaseTicket = redis.NewScript(`
	local status = redis.call('HGET', KEYS[1], 'status')
	if not status then
		return 'not_found'
	end
	if status ~= 'processing' then
		return 'invalid'
	end
	redis.call('HSET', KEYS[1], 'status', 'ready')
	redis.call('EXPIRE', KEYS[1], ARGV[1])
	return 'ok'
`)

// Helper: safely extract a string result from a Lua script Cmd.
func scriptResult(cmd *redis.Cmd) (string, error) {
	val, err := cmd.Result()
	if err == redis.Nil {
		return "not_found", nil
	}
	if err != nil {
		return "", err
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("unexpected script result type: %T", val)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// VerificationService
// ---------------------------------------------------------------------------

// VerificationService provides email verification code operations backed by
// Redis. It fails closed — when Redis is unreachable every operation returns
// an error so callers never silently skip verification.
type VerificationService struct {
	rdb      *redis.Client
	pepper   string
	mailer   Mailer
	clock    Clock
	codeGen  CodeGenerator
	userLook UserLookup
}

// NewVerificationService creates a new VerificationService.
func NewVerificationService(
	rdb *redis.Client,
	pepper string,
	mailer Mailer,
	clock Clock,
	codeGen CodeGenerator,
	userLook UserLookup,
) *VerificationService {
	return &VerificationService{
		rdb:      rdb,
		pepper:   pepper,
		mailer:   mailer,
		clock:    clock,
		codeGen:  codeGen,
		userLook: userLook,
	}
}

// checkRedis verifies that the underlying Redis connection is responsive.
func (s *VerificationService) checkRedis(ctx context.Context) error {
	if s.rdb == nil {
		return VerificationErrRedisDown
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		log.Printf("verification: redis ping failed: %v", err)
		return VerificationErrRedisDown
	}
	return nil
}

// SendVerificationCode generates a 6-digit code, stores it in Redis, and
// dispatches it via the configured Mailer. For password-reset requests
// targeting an unregistered email it returns nil (generic success) without
// storing a code or sending an email.
func (s *VerificationService) SendVerificationCode(ctx context.Context, input dto.VerificationSendInput) error {
	if err := s.checkRedis(ctx); err != nil {
		return err
	}

	normalized, err := security.NormalizeEmail(input.Email)
	if err != nil {
		return VerificationErrInvalidCode
	}
	hmac := HMACEmail(normalized, s.pepper)
	now := s.clock.Now()
	hour := hourBucket(now)

	purpose := input.Purpose

	// --- Rate limit checks ---

	// 1. Resend lock — uses clock-aware timestamp so fake clocks work in tests.
	lockKey := LockKey(purpose, hmac)
	lockVal, err := s.rdb.Get(ctx, lockKey).Int64()
	if err == nil {
		elapsed := now.Unix() - lockVal
		if elapsed < int64(lockDuration.Seconds()) {
			return VerificationErrLocked
		}
	}

	// 2. Per-email send limit (hourly)
	sendKey := SendCountKey(purpose, hmac, hour)
	sendCount, err := s.rdb.Get(ctx, sendKey).Int64()
	if err == nil && sendCount >= emailSendLimit {
		return VerificationErrSendLimit
	}

	// 3. Per-IP send limit (hourly)
	ipKey := SendIPCountKey(purpose, input.IP, hour)
	ipCount, err := s.rdb.Get(ctx, ipKey).Int64()
	if err == nil && ipCount >= ipSendLimit {
		return VerificationErrIPLimit
	}

	// 4. Password reset to unregistered email → generic success
	if purpose == string(enum.VerificationPurposeResetPassword) && !s.userLook.ExistsByEmail(ctx, normalized) {
		return nil
	}

	// --- Store HMAC of code (never store plaintext codes in Redis) ---
	code := s.codeGen.Generate()
	codeHash := security.HMACDigest(normalized+code, s.pepper)
	codeKey := CodeKey(purpose, hmac)
	if err := s.rdb.Set(ctx, codeKey, codeHash, codeTTL).Err(); err != nil {
		log.Printf("verification: failed to store code: %v", err)
		return VerificationErrRedisDown
	}

	// --- Set resend lock (store unix timestamp, use same TTL for safety) ---
	if err := s.rdb.Set(ctx, lockKey, now.Unix(), lockDuration).Err(); err != nil {
		log.Printf("verification: failed to set lock: %v", err)
	}

	// --- Increment send counters (pipeline) ---
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, sendKey)
	pipe.Expire(ctx, sendKey, 1*time.Hour)
	pipe.Incr(ctx, ipKey)
	pipe.Expire(ctx, ipKey, 1*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("verification: failed to increment counters: %v", err)
	}

	// --- Send email ---
	subject := fmt.Sprintf("[HotKey] Your %s verification code", purposeLabel(purpose))
	body := fmt.Sprintf("Your verification code is: %s\n\nThis code expires in 10 minutes.", code)
	if _, err := s.mailer.Send(ctx, normalized, subject, body); err != nil {
		log.Printf("verification: failed to send email: %v", err)
		s.rdb.Del(ctx, codeKey, lockKey)
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}

// ConfirmCode verifies a stored code against the user-supplied value. On
// success it atomically consumes the code and issues a one-time ticket.
func (s *VerificationService) ConfirmCode(ctx context.Context, input dto.VerificationConfirmInput) (string, error) {
	if err := s.checkRedis(ctx); err != nil {
		return "", err
	}

	normalized, err := security.NormalizeEmail(input.Email)
	if err != nil {
		return "", VerificationErrInvalidCode
	}
	hmac := HMACEmail(normalized, s.pepper)
	codeKey := CodeKey(input.Purpose, hmac)
	failKey := FailCountKey(input.Purpose, hmac)

	// Retrieve stored HMAC for comparison.
	storedDigest, err := s.rdb.Get(ctx, codeKey).Result()
	if err == redis.Nil {
		return "", VerificationErrNotFound
	}
	if err != nil {
		log.Printf("verification: redis get failed: %v", err)
		return "", VerificationErrRedisDown
	}

	// Compare HMAC of input code against stored HMAC (codes are never stored in plaintext).
	expectedDigest := security.HMACDigest(normalized+input.Code, s.pepper)
	if storedDigest != expectedDigest {
		// Wrong code: atomically increment fail count, delete code on >= 5.
		res, scriptErr := scriptResult(scriptFailedAttempt.Run(ctx, s.rdb, []string{failKey, codeKey}))
		if scriptErr != nil {
			log.Printf("verification: failed attempt script error: %v", scriptErr)
		}
		_ = res
		return "", VerificationErrInvalidCode
	}

	// --- Code matches: atomically consume and issue ticket ---
	ticket := newTicketID()
	ticketKey := TicketKey(ticket)

	res, scriptErr := scriptResult(scriptConsumeCode.Run(ctx, s.rdb, []string{codeKey, ticketKey},
		normalized, input.Purpose, int(ticketTTL.Seconds()),
	))
	if scriptErr != nil {
		log.Printf("verification: consume script error: %v", scriptErr)
		return "", VerificationErrRedisDown
	}
	if res == "not_found" {
		return "", VerificationErrNotFound
	}

	return ticket, nil
}

// ClaimTicket atomically marks a ticket as "processing" and returns its data.
func (s *VerificationService) ClaimTicket(ctx context.Context, ticket string) (*TicketData, error) {
	if err := s.checkRedis(ctx); err != nil {
		return nil, err
	}

	ticketKey := TicketKey(ticket)
	res, err := scriptResult(scriptClaimTicket.Run(ctx, s.rdb, []string{ticketKey}, int(ticketTTL.Seconds())))
	if err != nil {
		log.Printf("verification: claim script error: %v", err)
		return nil, VerificationErrRedisDown
	}
	switch res {
	case "not_found":
		return nil, VerificationErrTicketNotFound
	case "claimed":
		return nil, VerificationErrTicketClaimed
	}

	data, err := s.getTicketData(ctx, ticketKey)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// CompleteTicket marks a ticket as "completed" and sets a short TTL for
// replay protection.
func (s *VerificationService) CompleteTicket(ctx context.Context, ticket string) error {
	if err := s.checkRedis(ctx); err != nil {
		return err
	}

	ticketKey := TicketKey(ticket)
	res, err := scriptResult(scriptCompleteTicket.Run(ctx, s.rdb, []string{ticketKey}, int(completedTTL.Seconds())))
	if err != nil {
		log.Printf("verification: complete script error: %v", err)
		return VerificationErrRedisDown
	}
	if res == "not_found" {
		return VerificationErrTicketNotFound
	}
	return nil
}

// ReleaseTicket reverts a "processing" ticket back to "ready", allowing it
// to be claimed again.
func (s *VerificationService) ReleaseTicket(ctx context.Context, ticket string) error {
	if err := s.checkRedis(ctx); err != nil {
		return err
	}

	ticketKey := TicketKey(ticket)
	res, err := scriptResult(scriptReleaseTicket.Run(ctx, s.rdb, []string{ticketKey}, int(ticketTTL.Seconds())))
	if err != nil {
		log.Printf("verification: release script error: %v", err)
		return VerificationErrRedisDown
	}
	switch res {
	case "not_found":
		return VerificationErrTicketNotFound
	case "invalid":
		return VerificationErrTicketClaimed
	}
	return nil
}

// getTicketData reads and parses a ticket hash from Redis.
func (s *VerificationService) getTicketData(ctx context.Context, ticketKey string) (*TicketData, error) {
	fields, err := s.rdb.HGetAll(ctx, ticketKey).Result()
	if err != nil {
		log.Printf("verification: failed to read ticket: %v", err)
		return nil, VerificationErrRedisDown
	}
	if len(fields) == 0 {
		return nil, VerificationErrTicketNotFound
	}

	return &TicketData{
		Email:   fields["email"],
		Purpose: enum.VerificationPurpose(fields["purpose"]),
		Status:  fields["status"],
	}, nil
}
