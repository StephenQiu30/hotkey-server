package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
)

// rotateLuaScript atomically rotates the refresh token hash in a session.
// Returns user_id on success, or one of the error replies below.
//
// KEYS[1] = session:{sessionID}
// ARGV[1] = old_hash
// ARGV[2] = new_hash
// ARGV[3] = new_expires_at (RFC3339)
//
// Error replies:
//
//	SESSION_NOT_FOUND
//	SESSION_REVOKED
//	TOKEN_REUSED
const rotateLuaScript = `
local data = redis.call("HGETALL", KEYS[1])
if #data == 0 then
    return redis.error_reply("SESSION_NOT_FOUND")
end
local m = {}
for i = 1, #data, 2 do m[data[i]] = data[i+1] end
if m["status"] == "revoked" then
    return redis.error_reply("SESSION_REVOKED")
end
if m["current_hash"] ~= ARGV[1] then
    redis.call("DEL", KEYS[1])
    return redis.error_reply("TOKEN_REUSED")
end
redis.call("HSET", KEYS[1],
    "current_hash", ARGV[2],
    "last_refreshed", ARGV[3])
redis.call("EXPIRE", KEYS[1], 604800)
return m["user_id"]
`

// RedisAuthSessionRepository implements service.AuthSessionRepository
// using Redis Hashes with TTL.
type RedisAuthSessionRepository struct {
	client    *redis.Client
	rotateSHA string // cached SHA of the rotate Lua script
	now       func() time.Time
}

// NewRedisAuthSessionRepository creates a new RedisAuthSessionRepository.
// It registers the rotate Lua script at construction time.
func NewRedisAuthSessionRepository(client *redis.Client) *RedisAuthSessionRepository {
	r := &RedisAuthSessionRepository{
		client: client,
		now:    time.Now,
	}
	// Register the rotate script at construction time
	sha, err := client.ScriptLoad(context.Background(), rotateLuaScript).Result()
	if err != nil {
		panic(fmt.Sprintf("redis: failed to load rotate script: %v", err))
	}
	r.rotateSHA = sha
	return r
}

// SetNow overrides the clock (for testing).
func (r *RedisAuthSessionRepository) SetNow(now func() time.Time) {
	r.now = now
}

// CreateSession inserts a new active session into Redis and returns it.
func (r *RedisAuthSessionRepository) CreateSession(
	ctx context.Context,
	userID int64,
	tokenHash, familyHash, ip, ua string,
	expiresAt, absoluteExpiresAt time.Time,
) (entity.AuthSession, error) {
	now := r.now()

	// Generate a session ID using Redis INCR
	id, err := r.client.Incr(ctx, "session_seq").Result()
	if err != nil {
		return entity.AuthSession{}, fmt.Errorf("redis: incr session_seq: %w", err)
	}

	fields := map[string]interface{}{
		"user_id":        strconv.FormatInt(userID, 10),
		"family_hash":    familyHash,
		"current_hash":   tokenHash,
		"access_jti":     "",
		"status":         "active",
		"ip_address":     ip,
		"user_agent":     ua,
		"created_at":     now.UTC().Format(time.RFC3339),
		"expires_at":     expiresAt.UTC().Format(time.RFC3339),
		"absolute_exp":   absoluteExpiresAt.UTC().Format(time.RFC3339),
		"last_refreshed": now.UTC().Format(time.RFC3339),
	}

	key := sessionKey(id)
	if err := r.client.HSet(ctx, key, fields).Err(); err != nil {
		return entity.AuthSession{}, fmt.Errorf("redis: hset %s: %w", key, err)
	}
	// Set TTL: idle expiry (7 days)
	if err := r.client.Expire(ctx, key, 7*24*time.Hour).Err(); err != nil {
		return entity.AuthSession{}, fmt.Errorf("redis: expire %s: %w", key, err)
	}
	// Track in user_tokens set
	if err := r.client.SAdd(ctx, userTokensKey(userID), id).Err(); err != nil {
		return entity.AuthSession{}, fmt.Errorf("redis: sadd %s: %w", userTokensKey(userID), err)
	}

	return entity.AuthSession{
		ID:                id,
		UserID:            userID,
		TokenHash:         tokenHash,
		FamilyHash:        familyHash,
		Status:            "active",
		IPAddress:         ip,
		UserAgent:         ua,
		ExpiresAt:         expiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		LastRefreshedAt:   &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// GetSession retrieves a session by its primary key.
// Returns nil, nil when the key does not exist.
func (r *RedisAuthSessionRepository) GetSession(ctx context.Context, sessionID int64) (*entity.AuthSession, error) {
	data, err := r.client.HGetAll(ctx, sessionKey(sessionID)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: hgetall %s: %w", sessionKey(sessionID), err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	return r.hashToSession(data), nil
}

// RevokeSession marks a single session as revoked.
func (r *RedisAuthSessionRepository) RevokeSession(ctx context.Context, sessionID int64, reason string) error {
	key := sessionKey(sessionID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("redis: exists %s: %w", key, err)
	}
	if exists == 0 {
		return ErrSessionNotFound
	}
	if err := r.client.HSet(ctx, key, "status", "revoked").Err(); err != nil {
		return fmt.Errorf("redis: hset %s status=revoked: %w", key, err)
	}
	return nil
}

// RevokeUserSessions marks all active sessions for a user as revoked.
func (r *RedisAuthSessionRepository) RevokeUserSessions(ctx context.Context, userID int64, reason string) error {
	members, err := r.client.SMembers(ctx, userTokensKey(userID)).Result()
	if err != nil {
		return fmt.Errorf("redis: smembers %s: %w", userTokensKey(userID), err)
	}
	for _, sid := range members {
		id, _ := strconv.ParseInt(sid, 10, 64)
		if err := r.client.HSet(ctx, sessionKey(id), "status", "revoked").Err(); err != nil {
			return fmt.Errorf("redis: hset %s status=revoked: %w", sessionKey(id), err)
		}
	}
	return nil
}

// RotateSession atomically rotates the token hash for a session using a Lua script.
//
// If currentTokenHash does not match the stored hash (possible token reuse),
// the session is deleted and ErrTokenMismatch is returned.
func (r *RedisAuthSessionRepository) RotateSession(
	ctx context.Context,
	sessionID int64,
	currentTokenHash, newTokenHash string,
	now time.Time,
) (entity.AuthSession, error) {
	nowStr := now.UTC().Format(time.RFC3339)
	key := sessionKey(sessionID)

	_, err := r.client.EvalSha(ctx, r.rotateSHA, []string{key}, currentTokenHash, newTokenHash, nowStr).Result()
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "SESSION_NOT_FOUND"):
			return entity.AuthSession{}, ErrSessionNotFound
		case strings.Contains(errMsg, "SESSION_REVOKED"):
			return entity.AuthSession{}, ErrSessionRevoked
		case strings.Contains(errMsg, "TOKEN_REUSED"):
			return entity.AuthSession{}, ErrTokenMismatch
		default:
			return entity.AuthSession{}, fmt.Errorf("redis: rotate session %s: %w", key, err)
		}
	}

	// Re-read the updated session
	session, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return entity.AuthSession{}, err
	}
	if session == nil {
		return entity.AuthSession{}, ErrSessionNotFound
	}
	return *session, nil
}

// hashToSession converts a Redis hash map to an entity.AuthSession.
func (r *RedisAuthSessionRepository) hashToSession(data map[string]string) *entity.AuthSession {
	userID, _ := strconv.ParseInt(data["user_id"], 10, 64)
	sessionID, _ := strconv.ParseInt(data["session_id"], 10, 64)
	expiresAt, _ := time.Parse(time.RFC3339, data["expires_at"])
	absoluteExp, _ := time.Parse(time.RFC3339, data["absolute_exp"])
	createdAt, _ := time.Parse(time.RFC3339, data["created_at"])
	lastRefreshed, _ := time.Parse(time.RFC3339, data["last_refreshed"])

	return &entity.AuthSession{
		ID:                sessionID,
		UserID:            userID,
		TokenHash:         data["current_hash"],
		FamilyHash:        data["family_hash"],
		Status:            data["status"],
		IPAddress:         data["ip_address"],
		UserAgent:         data["user_agent"],
		ExpiresAt:         expiresAt,
		AbsoluteExpiresAt: absoluteExp,
		LastRefreshedAt:   &lastRefreshed,
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
}
