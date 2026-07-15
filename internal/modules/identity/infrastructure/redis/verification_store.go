package redis

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	goredis "github.com/redis/go-redis/v9"
)

const (
	verificationPrefix      = "hotkey:identity:verification:v1"
	verificationCooldown    = time.Minute
	verificationTicketTTL   = 10 * time.Minute
	verificationMaxAttempts = 5
)

var (
	createCodeScript = goredis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
redis.call('HSET', KEYS[1], 'purpose', ARGV[1], 'email_hash', ARGV[2], 'code_hash', ARGV[3], 'attempts', '0')
redis.call('PEXPIRE', KEYS[1], ARGV[4])
redis.call('SET', KEYS[2], '1', 'PX', ARGV[5])
return 1`)
	consumeCodeScript = goredis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 0
end
if redis.call('HGET', KEYS[1], 'purpose') ~= ARGV[1] or redis.call('HGET', KEYS[1], 'email_hash') ~= ARGV[2] then
  return 0
end
if redis.call('HGET', KEYS[1], 'code_hash') ~= ARGV[4] then
  local attempts = redis.call('HINCRBY', KEYS[1], 'attempts', 1)
  if attempts >= tonumber(ARGV[5]) then
    redis.call('DEL', KEYS[1])
  end
  return 0
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then
  return 0
end
local ticket_ttl = math.min(ttl, tonumber(ARGV[6]))
-- The ticket is consumed later using only its opaque value, so it retains the
-- verified email; the short-lived code state above retains only its hash.
redis.call('HSET', KEYS[2], 'email', ARGV[3], 'email_hash', ARGV[2], 'purpose', ARGV[1])
redis.call('PEXPIRE', KEYS[2], ticket_ttl)
redis.call('DEL', KEYS[1])
return ticket_ttl`)
	consumeTicketScript = goredis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return nil
end
if redis.call('HGET', KEYS[1], 'purpose') ~= ARGV[1] then
  return nil
end
local email = redis.call('HGET', KEYS[1], 'email')
redis.call('DEL', KEYS[1])
return email`)
)

type VerificationStore struct {
	client goredis.UniversalClient
	now    func() time.Time
}

var _ domain.VerificationStore = (*VerificationStore)(nil)

func NewVerificationStore(client goredis.UniversalClient) *VerificationStore {
	return &VerificationStore{client: client, now: time.Now}
}

func NewVerificationStoreFromURL(rawURL string) (*VerificationStore, error) {
	options, err := goredis.ParseURL(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	return NewVerificationStore(goredis.NewClient(options)), nil
}

func (store *VerificationStore) Close() error {
	if store == nil || store.client == nil {
		return nil
	}
	return store.client.Close()
}

func (store *VerificationStore) CreateCode(ctx context.Context, purpose domain.VerificationPurpose, email, code string, expiresAt time.Time) error {
	normalizedEmail, err := validVerificationInput(purpose, email)
	if err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return domain.VerificationInvalid()
	}
	now := store.currentTime()
	ttl := expiresAt.UTC().Sub(now)
	if ttl <= 0 {
		return domain.VerificationInvalid()
	}
	if !store.available() {
		return unavailable()
	}

	result, err := createCodeScript.Run(ctx, store.client, []string{store.codeKey(purpose, normalizedEmail), store.cooldownKey(purpose, normalizedEmail)}, string(purpose), hashString(normalizedEmail), hashString(code), ttl.Milliseconds(), verificationCooldown.Milliseconds()).Int64()
	if err != nil {
		return unavailable()
	}
	if result == 0 {
		return sharederrors.New(sharederrors.CodeRateLimited, stdhttp.StatusTooManyRequests, "")
	}
	return nil
}

func (store *VerificationStore) ConsumeCode(ctx context.Context, purpose domain.VerificationPurpose, email, code string) (domain.VerificationTicket, error) {
	normalizedEmail, err := validVerificationInput(purpose, email)
	if err != nil {
		return domain.VerificationTicket{}, err
	}
	if strings.TrimSpace(code) == "" {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	if !store.available() {
		return domain.VerificationTicket{}, unavailable()
	}
	token, err := newOpaqueTicket()
	if err != nil {
		return domain.VerificationTicket{}, unavailable()
	}
	result, err := consumeCodeScript.Run(ctx, store.client, []string{store.codeKey(purpose, normalizedEmail), store.ticketKey(token)}, string(purpose), hashString(normalizedEmail), normalizedEmail, hashString(code), verificationMaxAttempts, verificationTicketTTL.Milliseconds()).Int64()
	if err != nil {
		return domain.VerificationTicket{}, unavailable()
	}
	if result <= 0 {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	return domain.VerificationTicket{
		Token:     token,
		Email:     normalizedEmail,
		Purpose:   purpose,
		ExpiresAt: store.currentTime().Add(time.Duration(result) * time.Millisecond).UTC(),
	}, nil
}

func (store *VerificationStore) ConsumeTicket(ctx context.Context, purpose domain.VerificationPurpose, token string) (domain.VerificationTicket, error) {
	if !purpose.Valid() || strings.TrimSpace(token) == "" {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	if !store.available() {
		return domain.VerificationTicket{}, unavailable()
	}
	email, err := consumeTicketScript.Run(ctx, store.client, []string{store.ticketKey(token)}, string(purpose)).Text()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return domain.VerificationTicket{}, domain.VerificationInvalid()
		}
		return domain.VerificationTicket{}, unavailable()
	}
	if strings.TrimSpace(email) == "" {
		return domain.VerificationTicket{}, domain.VerificationInvalid()
	}
	return domain.VerificationTicket{Token: token, Email: email, Purpose: purpose}, nil
}

func (store *VerificationStore) available() bool {
	return store != nil && store.client != nil
}

func (store *VerificationStore) currentTime() time.Time {
	if store != nil && store.now != nil {
		return store.now().UTC()
	}
	return time.Now().UTC()
}

func (store *VerificationStore) codeKey(purpose domain.VerificationPurpose, email string) string {
	return verificationPrefix + ":code:" + string(purpose) + ":" + hashString(email)
}

func (store *VerificationStore) cooldownKey(purpose domain.VerificationPurpose, email string) string {
	return verificationPrefix + ":cooldown:" + string(purpose) + ":" + hashString(email)
}

func (store *VerificationStore) ticketKey(token string) string {
	return verificationPrefix + ":ticket:" + hashString(token)
}

func validVerificationInput(purpose domain.VerificationPurpose, email string) (string, error) {
	if !purpose.Valid() {
		return "", domain.VerificationInvalid()
	}
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return "", domain.VerificationInvalid()
	}
	return normalized, nil
}

func newOpaqueTicket() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func unavailable() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "")
}
