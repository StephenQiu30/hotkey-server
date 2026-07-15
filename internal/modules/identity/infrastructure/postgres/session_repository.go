package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

var (
	ErrRefreshReplay  = errors.New("refresh token replay detected")
	ErrRefreshInvalid = errors.New("refresh token is invalid")
)

type SessionRepository struct {
	runtime *database.Runtime
}

var _ domain.SessionRepository = (*SessionRepository)(nil)

func NewSessionRepository(runtime *database.Runtime) *SessionRepository {
	return &SessionRepository{runtime: runtime}
}

func (repository *SessionRepository) Create(ctx context.Context, session *domain.Session, refreshToken *domain.RefreshToken) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if session == nil || refreshToken == nil {
		return fmt.Errorf("%w: session and refresh token are required", sharedrepository.ErrInvalidInput)
	}
	return useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		return createSessionAndToken(ctx, transaction, session, refreshToken)
	})
}

func (repository *SessionRepository) FindByRefreshTokenHash(ctx context.Context, hash string) (*domain.Session, *domain.RefreshToken, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, nil, sharedrepository.ErrUnavailable
	}
	if strings.TrimSpace(hash) == "" {
		return nil, nil, fmt.Errorf("%w: refresh token hash is required", sharedrepository.ErrInvalidInput)
	}
	session, token, err := findSessionAndToken(ctx, transactionSQL(ctx, repository.runtime), hash, false)
	if err != nil {
		return nil, nil, err
	}
	return &session, &token, nil
}

// Rotate serializes consumption on the current refresh-token row. A second
// consumer observes used_at while holding that row lock, revokes the complete
// session family in the same transaction, and receives ErrRefreshReplay.
func (repository *SessionRepository) Rotate(ctx context.Context, currentHash string, replacement *domain.RefreshToken, now time.Time) (*domain.Session, *domain.RefreshToken, error) {
	if repository == nil || repository.runtime == nil {
		return nil, nil, sharedrepository.ErrUnavailable
	}
	if strings.TrimSpace(currentHash) == "" || replacement == nil || strings.TrimSpace(replacement.TokenHash) == "" {
		return nil, nil, fmt.Errorf("%w: refresh token hashes are required", sharedrepository.ErrInvalidInput)
	}
	now = now.UTC()
	var session domain.Session
	var token domain.RefreshToken
	var replayDetected bool
	err := useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		var err error
		session, token, err = findSessionAndToken(ctx, transaction.SQL, currentHash, true)
		if err != nil {
			if errors.Is(err, sharedrepository.ErrNotFound) {
				return ErrRefreshInvalid
			}
			return err
		}
		if token.UsedAt != nil {
			if err := revokeSession(ctx, transaction, session.ID, "refresh_replay", now); err != nil {
				return err
			}
			replayDetected = true
			return nil
		}
		if token.RevokedAt != nil || !token.ExpiresAt.After(now) || session.RevokedAt != nil || !session.AbsoluteExpiresAt.After(now) {
			return ErrRefreshInvalid
		}
		if replacement.ExpiresAt.After(session.AbsoluteExpiresAt) || !replacement.ExpiresAt.After(now) {
			return fmt.Errorf("%w: replacement expiry is outside the session lifetime", sharedrepository.ErrInvalidInput)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE auth_refresh_tokens SET used_at = $1 WHERE id = $2 AND used_at IS NULL`, now, token.ID); err != nil {
			return mapRepositoryError(err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE auth_sessions SET last_seen_at = $1 WHERE id = $2`, now, session.ID); err != nil {
			return mapRepositoryError(err)
		}
		replacement.SessionID = session.ID
		var record refreshTokenRecord
		err = transaction.SQL.QueryRowContext(ctx, `
INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at, created_at)
VALUES ($1, $2, $3, $4)
RETURNING id, session_id, token_hash, expires_at, used_at, revoked_at, created_at`,
			replacement.SessionID, replacement.TokenHash, replacement.ExpiresAt.UTC(), replacement.CreatedAt.UTC(),
		).Scan(&record.ID, &record.SessionID, &record.TokenHash, &record.ExpiresAt, &record.UsedAt, &record.RevokedAt, &record.CreatedAt)
		if err != nil {
			return mapRepositoryError(err)
		}
		*replacement = record.domainRefreshToken()
		session.LastSeenAt = now
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if replayDetected {
		return nil, nil, ErrRefreshReplay
	}
	return &session, replacement, nil
}

func (repository *SessionRepository) RevokeSession(ctx context.Context, sessionID int64, reason string, now time.Time) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if sessionID <= 0 {
		return fmt.Errorf("%w: session ID must be positive", sharedrepository.ErrInvalidInput)
	}
	return useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		return revokeSession(ctx, transaction, sessionID, reason, now.UTC())
	})
}

func (repository *SessionRepository) RevokeAllForUser(ctx context.Context, userID int64, reason string, now time.Time) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if userID <= 0 {
		return fmt.Errorf("%w: user ID must be positive", sharedrepository.ErrInvalidInput)
	}
	now = now.UTC()
	return useTransaction(ctx, repository.runtime, func(ctx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = COALESCE(revoked_at, $1), revoke_reason = COALESCE(revoke_reason, $2)
WHERE user_id = $3`, now, strings.TrimSpace(reason), userID); err != nil {
			return mapRepositoryError(err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE auth_refresh_tokens AS token
SET revoked_at = COALESCE(token.revoked_at, $1)
FROM auth_sessions AS session
WHERE token.session_id = session.id AND session.user_id = $2`, now, userID); err != nil {
			return mapRepositoryError(err)
		}
		return nil
	})
}

func createSessionAndToken(ctx context.Context, transaction database.Transaction, session *domain.Session, token *domain.RefreshToken) error {
	if transaction.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if session.UserID <= 0 || strings.TrimSpace(session.FamilyID) == "" || !session.AbsoluteExpiresAt.After(session.CreatedAt) || strings.TrimSpace(token.TokenHash) == "" || !token.ExpiresAt.After(token.CreatedAt) || token.ExpiresAt.After(session.AbsoluteExpiresAt) {
		return fmt.Errorf("%w: invalid session or refresh token", sharedrepository.ErrInvalidInput)
	}
	var sessionRecord sessionRecord
	err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO auth_sessions (user_id, family_id, absolute_expires_at, last_seen_at, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, family_id, absolute_expires_at, last_seen_at, revoked_at, revoke_reason, created_at`,
		session.UserID, session.FamilyID, session.AbsoluteExpiresAt.UTC(), session.LastSeenAt.UTC(), session.CreatedAt.UTC(),
	).Scan(&sessionRecord.ID, &sessionRecord.UserID, &sessionRecord.FamilyID, &sessionRecord.AbsoluteExpiresAt, &sessionRecord.LastSeenAt, &sessionRecord.RevokedAt, &sessionRecord.RevokeReason, &sessionRecord.CreatedAt)
	if err != nil {
		return mapRepositoryError(err)
	}
	*session = sessionRecord.domainSession()
	token.SessionID = session.ID

	var tokenRecord refreshTokenRecord
	err = transaction.SQL.QueryRowContext(ctx, `
INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at, created_at)
VALUES ($1, $2, $3, $4)
RETURNING id, session_id, token_hash, expires_at, used_at, revoked_at, created_at`,
		token.SessionID, token.TokenHash, token.ExpiresAt.UTC(), token.CreatedAt.UTC(),
	).Scan(&tokenRecord.ID, &tokenRecord.SessionID, &tokenRecord.TokenHash, &tokenRecord.ExpiresAt, &tokenRecord.UsedAt, &tokenRecord.RevokedAt, &tokenRecord.CreatedAt)
	if err != nil {
		return mapRepositoryError(err)
	}
	*token = tokenRecord.domainRefreshToken()
	return nil
}

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func findSessionAndToken(ctx context.Context, queryer rowQueryer, hash string, lock bool) (domain.Session, domain.RefreshToken, error) {
	query := `
SELECT session.id, session.user_id, session.family_id, session.absolute_expires_at, session.last_seen_at, session.revoked_at, session.revoke_reason, session.created_at,
       token.id, token.session_id, token.token_hash, token.expires_at, token.used_at, token.revoked_at, token.created_at
FROM auth_refresh_tokens AS token
JOIN auth_sessions AS session ON session.id = token.session_id
WHERE token.token_hash = $1`
	if lock {
		query += ` FOR UPDATE OF token, session`
	}
	var sessionRecord sessionRecord
	var tokenRecord refreshTokenRecord
	err := queryer.QueryRowContext(ctx, query, hash).Scan(
		&sessionRecord.ID, &sessionRecord.UserID, &sessionRecord.FamilyID, &sessionRecord.AbsoluteExpiresAt, &sessionRecord.LastSeenAt, &sessionRecord.RevokedAt, &sessionRecord.RevokeReason, &sessionRecord.CreatedAt,
		&tokenRecord.ID, &tokenRecord.SessionID, &tokenRecord.TokenHash, &tokenRecord.ExpiresAt, &tokenRecord.UsedAt, &tokenRecord.RevokedAt, &tokenRecord.CreatedAt,
	)
	if err != nil {
		return domain.Session{}, domain.RefreshToken{}, mapRepositoryError(err)
	}
	return sessionRecord.domainSession(), tokenRecord.domainRefreshToken(), nil
}

func revokeSession(ctx context.Context, transaction database.Transaction, sessionID int64, reason string, now time.Time) error {
	if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = COALESCE(revoked_at, $1), revoke_reason = COALESCE(revoke_reason, $2)
WHERE id = $3`, now, strings.TrimSpace(reason), sessionID); err != nil {
		return mapRepositoryError(err)
	}
	if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE auth_refresh_tokens
SET revoked_at = COALESCE(revoked_at, $1)
WHERE session_id = $2`, now, sessionID); err != nil {
		return mapRepositoryError(err)
	}
	return nil
}
