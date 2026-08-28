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
