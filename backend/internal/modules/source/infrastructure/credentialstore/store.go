package credentialstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Store struct {
	runtime *database.Runtime
	cipher  *Cipher
}

type RotationBatchResult struct {
	CurrentVersion int
	Scanned        int
	Rotated        int
	Remaining      int64
}

var ErrRotationTransactionRequired = errors.New("source credential rotation requires a caller transaction")

var _ domain.ManagedCredentialStore = (*Store)(nil)

// NewStore deliberately permits an empty key so existing env-backed sources
// and credential-free installations keep running. Managed writes and reads
// fail closed until the independent master key is configured.
func NewStore(runtime *database.Runtime, encodedKey string) (*Store, error) {
	return NewStoreWithKeyring(runtime, currentKeyVersion, encodedKey, nil)
}

// NewStoreWithKeyring writes only with currentVersion while retaining the
// explicitly supplied previous keys for a bounded migration window.
func NewStoreWithKeyring(runtime *database.Runtime, currentVersion int, encodedKey string, previous map[int]string) (*Store, error) {
	if runtime == nil {
		return nil, errors.New("source credential database runtime is required")
	}
	store := &Store{runtime: runtime}
	if strings.TrimSpace(encodedKey) == "" {
		return store, nil
	}
	value, err := NewCipherKeyring(currentVersion, encodedKey, previous)
	if err != nil {
		return nil, err
	}
	store.cipher = value
	return store, nil
}

func (store *Store) Store(ctx context.Context, sourceID int64, plaintext string, actorID int64) error {
	if store == nil || store.runtime == nil || store.cipher == nil {
		return sharedrepository.ErrUnavailable
	}
	if sourceID <= 0 || actorID <= 0 || strings.TrimSpace(plaintext) == "" {
		return sharedrepository.ErrInvalidInput
	}
	sealed, err := store.cipher.Encrypt(sourceID, []byte(plaintext))
	if err != nil {
		return fmt.Errorf("%w: encrypt source credential", sharedrepository.ErrUnavailable)
	}
	query := `
INSERT INTO source_credentials
    (source_connection_id, key_version, nonce, ciphertext, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (source_connection_id) DO UPDATE
SET key_version = EXCLUDED.key_version, nonce = EXCLUDED.nonce,
    ciphertext = EXCLUDED.ciphertext, updated_by = EXCLUDED.updated_by,
    updated_at = now()`
	if transaction, found := database.TransactionFromContext(ctx); found {
		_, err = transaction.SQL.ExecContext(ctx, query, sourceID, sealed.KeyVersion, sealed.Nonce, sealed.Ciphertext, actorID)
	} else if store.runtime.SQL != nil {
		_, err = store.runtime.SQL.ExecContext(ctx, query, sourceID, sealed.KeyVersion, sealed.Nonce, sealed.Ciphertext, actorID)
	} else {
		return sharedrepository.ErrUnavailable
	}
	return databaserepository.MapError(err)
}

func (store *Store) Delete(ctx context.Context, sourceID int64) error {
	if store == nil || store.runtime == nil || sourceID <= 0 {
		return sharedrepository.ErrInvalidInput
	}
	query := `DELETE FROM source_credentials WHERE source_connection_id = $1`
	var err error
	if transaction, found := database.TransactionFromContext(ctx); found {
		_, err = transaction.SQL.ExecContext(ctx, query, sourceID)
	} else if store.runtime.SQL != nil {
		_, err = store.runtime.SQL.ExecContext(ctx, query, sourceID)
	} else {
		return sharedrepository.ErrUnavailable
	}
	return databaserepository.MapError(err)
}

func (store *Store) Resolve(ctx context.Context, sourceID int64) (string, error) {
	if store == nil || store.runtime == nil || store.runtime.SQL == nil || store.cipher == nil || sourceID <= 0 {
		return "", sharedrepository.ErrUnavailable
	}
	var sealed SealedCredential
	err := store.runtime.SQL.QueryRowContext(ctx, `
SELECT key_version, nonce, ciphertext
FROM source_credentials
WHERE source_connection_id = $1`, sourceID).Scan(&sealed.KeyVersion, &sealed.Nonce, &sealed.Ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", sharedrepository.ErrUnavailable
	}
	if err != nil {
		return "", databaserepository.MapError(err)
	}
	plaintext, err := store.cipher.Decrypt(sourceID, sealed)
	if err != nil || len(plaintext) == 0 {
		return "", sharedrepository.ErrUnavailable
	}
	return string(plaintext), nil
}

// RotateBatch re-encrypts one bounded, locked batch with the current key. The
// caller owns the transaction so the credential updates and sanitized audit
// record can commit or roll back together.
func (store *Store) RotateBatch(ctx context.Context, actorID int64, batchSize int) (RotationBatchResult, error) {
	if store == nil || store.runtime == nil || store.cipher == nil {
		return RotationBatchResult{}, sharedrepository.ErrUnavailable
	}
	if actorID <= 0 || batchSize < 1 || batchSize > 1000 {
		return RotationBatchResult{}, sharedrepository.ErrInvalidInput
	}
	transaction, found := database.TransactionFromContext(ctx)
	if !found {
		return RotationBatchResult{}, ErrRotationTransactionRequired
	}
	currentVersion := store.cipher.currentVersion
	rows, err := transaction.SQL.QueryContext(ctx, `
SELECT source_connection_id, key_version, nonce, ciphertext
FROM source_credentials
WHERE key_version <> $1
ORDER BY id
LIMIT $2
FOR UPDATE`, currentVersion, batchSize)
	if err != nil {
		return RotationBatchResult{}, databaserepository.MapError(err)
	}
	type candidate struct {
		sourceID int64
		sealed   SealedCredential
	}
	candidates := make([]candidate, 0, batchSize)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.sourceID, &item.sealed.KeyVersion, &item.sealed.Nonce, &item.sealed.Ciphertext); err != nil {
			_ = rows.Close()
			return RotationBatchResult{}, databaserepository.MapError(err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return RotationBatchResult{}, databaserepository.MapError(err)
	}
	if err := rows.Close(); err != nil {
		return RotationBatchResult{}, databaserepository.MapError(err)
	}

	result := RotationBatchResult{CurrentVersion: currentVersion, Scanned: len(candidates)}
	for _, item := range candidates {
		plaintext, err := store.cipher.Decrypt(item.sourceID, item.sealed)
		if err != nil || len(plaintext) == 0 {
			return RotationBatchResult{}, fmt.Errorf("%w: source credential rotation authentication failed", sharedrepository.ErrUnavailable)
		}
		rotated, encryptErr := store.cipher.Encrypt(item.sourceID, plaintext)
		clear(plaintext)
		if encryptErr != nil {
			return RotationBatchResult{}, fmt.Errorf("%w: source credential rotation encryption failed", sharedrepository.ErrUnavailable)
		}
		updateResult, err := transaction.SQL.ExecContext(ctx, `
UPDATE source_credentials
SET key_version = $1, nonce = $2, ciphertext = $3, updated_by = $4, updated_at = now()
WHERE source_connection_id = $5 AND key_version = $6`,
			rotated.KeyVersion, rotated.Nonce, rotated.Ciphertext, actorID, item.sourceID, item.sealed.KeyVersion)
		if err != nil {
			return RotationBatchResult{}, databaserepository.MapError(err)
		}
		affected, err := updateResult.RowsAffected()
		if err != nil || affected != 1 {
			return RotationBatchResult{}, fmt.Errorf("%w: source credential rotation changed unexpectedly", sharedrepository.ErrConflict)
		}
		result.Rotated++
	}
	if err := transaction.SQL.QueryRowContext(ctx, `SELECT count(*) FROM source_credentials WHERE key_version <> $1`, currentVersion).Scan(&result.Remaining); err != nil {
		return RotationBatchResult{}, databaserepository.MapError(err)
	}
	return result, nil
}
