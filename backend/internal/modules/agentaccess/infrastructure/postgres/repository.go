package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

var _ domain.TokenRepository = (*Repository)(nil)

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) CountActive(ctx context.Context, userID int64, now time.Time) (int, error) {
	if repository == nil || repository.runtime == nil || userID <= 0 {
		return 0, sharedrepository.ErrUnavailable
	}
	var lockedUserID int64
	if err := executor(ctx, repository.runtime).QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		return 0, databaserepository.MapError(err)
	}
	var count int
	err := executor(ctx, repository.runtime).QueryRowContext(ctx, `SELECT count(*) FROM agent_tokens WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > $2`, userID, now.UTC()).Scan(&count)
	return count, databaserepository.MapError(err)
}

func (repository *Repository) Create(ctx context.Context, token *domain.Token) error {
	if repository == nil || repository.runtime == nil || token == nil {
		return sharedrepository.ErrUnavailable
	}
	row := executor(ctx, repository.runtime).QueryRowContext(ctx, `
INSERT INTO agent_tokens (user_id, name, token_prefix, token_hash, scopes, expires_at, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$7)
RETURNING id, version, created_at, updated_at`, token.UserID, token.Name, token.TokenPrefix, token.TokenHash, textArray(domain.ScopeStrings(token.Scopes)), token.ExpiresAt.UTC(), token.CreatedAt.UTC())
	return databaserepository.MapError(row.Scan(&token.ID, &token.Version, &token.CreatedAt, &token.UpdatedAt))
}

func (repository *Repository) ListByUser(ctx context.Context, userID int64) ([]domain.Token, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || userID <= 0 {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := rowsExecutor(ctx, repository.runtime).QueryContext(ctx, `
SELECT id, version, user_id, name, token_prefix, scopes, expires_at, last_used_at, revoked_at, created_at, updated_at
FROM agent_tokens WHERE user_id = $1 ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]domain.Token, 0)
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, token)
	}
	return result, databaserepository.MapError(rows.Err())
}

func (repository *Repository) Revoke(ctx context.Context, userID, tokenID, expectedVersion int64, now time.Time) (*domain.Token, error) {
	if repository == nil || repository.runtime == nil || userID <= 0 || tokenID <= 0 || expectedVersion <= 0 {
		return nil, sharedrepository.ErrInvalidInput
	}
	row := executor(ctx, repository.runtime).QueryRowContext(ctx, `
UPDATE agent_tokens SET revoked_at = $1, updated_at = $1, version = version + 1
WHERE id = $2 AND user_id = $3 AND version = $4 AND revoked_at IS NULL
RETURNING id, version, user_id, name, token_prefix, scopes, expires_at, last_used_at, revoked_at, created_at, updated_at`, now.UTC(), tokenID, userID, expectedVersion)
	token, err := scanToken(row)
	if err == nil {
		return &token, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, databaserepository.MapError(err)
	}
	var version int64
	if lookupErr := executor(ctx, repository.runtime).QueryRowContext(ctx, `SELECT version FROM agent_tokens WHERE id = $1 AND user_id = $2`, tokenID, userID).Scan(&version); lookupErr != nil {
		if errors.Is(lookupErr, sql.ErrNoRows) {
			return nil, sharedrepository.ErrNotFound
		}
		return nil, databaserepository.MapError(lookupErr)
	}
	return nil, sharedrepository.ErrConflict
}

func (repository *Repository) Authenticate(ctx context.Context, tokenHash string, now time.Time) (domain.Principal, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || len(tokenHash) != 64 {
		return domain.Principal{}, sharedrepository.ErrNotFound
	}
	var principal domain.Principal
	var role string
	var scopes []string
	err := repository.runtime.SQL.QueryRowContext(ctx, `
WITH touched AS (
    UPDATE agent_tokens AS token
    SET last_used_at = $2
    WHERE token.token_hash = $1
      AND token.revoked_at IS NULL
      AND token.expires_at > $2
      AND EXISTS (
          SELECT 1 FROM users AS owner
          WHERE owner.id = token.user_id AND owner.status = 'active' AND owner.deleted_at IS NULL
      )
    RETURNING token.id, token.user_id, token.scopes
)
SELECT touched.id, touched.user_id, owner.role, touched.scopes
FROM touched JOIN users AS owner ON owner.id = touched.user_id`, tokenHash, now.UTC()).Scan(&principal.TokenID, &principal.UserID, &role, textArrayScan{destination: &scopes})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Principal{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return domain.Principal{}, databaserepository.MapError(err)
	}
	principal.Role = identitydomain.Role(role)
	principal.Scopes = make([]domain.Scope, len(scopes))
	for index, scope := range scopes {
		principal.Scopes[index] = domain.Scope(scope)
	}
	return principal, nil
}

type scanner interface{ Scan(...any) error }

func scanToken(row scanner) (domain.Token, error) {
	var token domain.Token
	var scopes []string
	var lastUsedAt, revokedAt sql.NullTime
	err := row.Scan(&token.ID, &token.Version, &token.UserID, &token.Name, &token.TokenPrefix, textArrayScan{destination: &scopes}, &token.ExpiresAt, &lastUsedAt, &revokedAt, &token.CreatedAt, &token.UpdatedAt)
	if err != nil {
		return domain.Token{}, databaserepository.MapError(err)
	}
	token.Scopes = make([]domain.Scope, len(scopes))
	for index, scope := range scopes {
		token.Scopes[index] = domain.Scope(scope)
	}
	if lastUsedAt.Valid {
		value := lastUsedAt.Time.UTC()
		token.LastUsedAt = &value
	}
	if revokedAt.Valid {
		value := revokedAt.Time.UTC()
		token.RevokedAt = &value
	}
	return token, nil
}

type textArrayScan struct{ destination *[]string }

func (scan textArrayScan) Scan(value any) error {
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	default:
		return fmt.Errorf("scan text array: %T", value)
	}
	raw = strings.TrimSpace(raw)
	if raw == "{}" {
		*scan.destination = []string{}
		return nil
	}
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return fmt.Errorf("scan text array: malformed value")
	}
	*scan.destination = strings.Split(raw[1:len(raw)-1], ",")
	return nil
}

func textArray(values []string) string { return "{" + strings.Join(values, ",") + "}" }

func executor(ctx context.Context, runtime *database.Runtime) interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
} {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return runtime.SQL
}

func rowsExecutor(ctx context.Context, runtime *database.Runtime) interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
} {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return runtime.SQL
}
