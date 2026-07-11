package repository

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Sentinel errors returned by AuthSessionRepo. Services map these to stable ErrorCodes.
var (
	ErrSessionNotFound  = errors.New("auth session not found")
	ErrSessionRevoked   = errors.New("auth session is revoked")
	ErrSessionExpired   = errors.New("auth session has expired")
	ErrTokenMismatch    = errors.New("auth session token hash mismatch")
)

// AuthSession is a convenience alias exported for consumers.
type AuthSession = entity.AuthSession

// AuthSessionRepo implements session persistence via GORM.
type AuthSessionRepo struct {
	db *gorm.DB
}

// NewAuthSessionRepo creates a new AuthSessionRepo.
func NewAuthSessionRepo(db *gorm.DB) *AuthSessionRepo {
	return &AuthSessionRepo{db: db}
}

// CreateSession inserts a new active session row and returns it.
func (r *AuthSessionRepo) CreateSession(
	ctx context.Context,
	userID int64,
	tokenHash, familyHash, ip, ua string,
	expiresAt, absoluteExpiresAt time.Time,
) (AuthSession, error) {
	now := time.Now()
	session := entity.AuthSession{
		UserID:            userID,
		TokenHash:         tokenHash,
		FamilyHash:        familyHash,
		Status:            "active",
		IPAddress:         ip,
		UserAgent:         ua,
		ExpiresAt:         expiresAt,
		AbsoluteExpiresAt: absoluteExpiresAt,
		LastRefreshedAt:   &now,
	}
	if err := r.db.WithContext(ctx).Create(&session).Error; err != nil {
		return AuthSession{}, err
	}
	return session, nil
}

// GetSession retrieves a session by its primary key.
// Returns nil, nil when the row does not exist.
func (r *AuthSessionRepo) GetSession(ctx context.Context, sessionID int64) (*AuthSession, error) {
	var session entity.AuthSession
	if err := r.db.WithContext(ctx).First(&session, sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// RevokeSession marks a single session as revoked.
func (r *AuthSessionRepo) RevokeSession(ctx context.Context, sessionID int64, reason string) error {
	result := r.db.WithContext(ctx).Model(&entity.AuthSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"status":     "revoked",
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// RevokeUserSessions marks all active sessions for a user as revoked.
func (r *AuthSessionRepo) RevokeUserSessions(ctx context.Context, userID int64, reason string) error {
	return r.db.WithContext(ctx).Model(&entity.AuthSession{}).
		Where("user_id = ? AND status = ?", userID, "active").
		Updates(map[string]any{
			"status":     "revoked",
			"updated_at": time.Now(),
		}).Error
}

// RotateSession atomically rotates the token hash for a session.
//
// It uses SELECT FOR UPDATE to prevent concurrent races. The method verifies
// the current token hash and time bounds before replacing the hash.
//
// If currentTokenHash does not match the stored hash (possible token reuse),
// ALL sessions sharing the same FamilyHash are revoked and ErrTokenMismatch
// is returned.
func (r *AuthSessionRepo) RotateSession(
	ctx context.Context,
	sessionID int64,
	currentTokenHash, newTokenHash string,
	now time.Time,
) (AuthSession, error) {
	var session entity.AuthSession

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the row for the duration of the transaction.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&session, sessionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSessionNotFound
			}
			return err
		}

		// Reject revoked sessions.
		if session.Status == "revoked" {
			return ErrSessionRevoked
		}

		// Reject expired sessions.
		if now.After(session.AbsoluteExpiresAt) || now.After(session.ExpiresAt) {
			return ErrSessionExpired
		}

		// If the token hash doesn't match, this could be a token reuse attack.
		// Revoke ALL sessions in the same family to invalidate any stolen tokens.
		if session.TokenHash != currentTokenHash {
			tx.Model(&entity.AuthSession{}).
				Where("family_hash = ? AND status = ?", session.FamilyHash, "active").
				Updates(map[string]any{
					"status":     "revoked",
					"updated_at": now,
				})
			return ErrTokenMismatch
		}

		// Atomically replace the token hash and refresh the timestamp.
		result := tx.Model(&session).Updates(map[string]any{
			"token_hash":        newTokenHash,
			"last_refreshed_at": now,
			"updated_at":        now,
		})
		if result.Error != nil {
			return result.Error
		}

		// Re-read to return the updated row.
		return tx.First(&session, sessionID).Error
	})

	if err != nil {
		return AuthSession{}, err
	}
	return session, nil
}
