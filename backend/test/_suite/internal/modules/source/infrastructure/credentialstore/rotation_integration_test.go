package credentialstore

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestRotateBatchReencryptsPreviousCredentialsAndSupportsKeyRevocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runtime := openCredentialRotationRuntime(t, ctx)
	defer runtime.Close()
	actorID := seedCredentialRotationActor(t, runtime)
	oldKey := encodedRotationKey(0x31)
	newKey := encodedRotationKey(0x42)
	legacy, _ := NewCipherKeyring(1, oldKey, nil)
	want := make(map[int64]string)
	for index, plaintext := range []string{"first-provider-secret", "second-provider-secret"} {
		sourceID := seedCredentialRotationSource(t, runtime, actorID, fmt.Sprintf("rotation-source-%d", index))
		seedCredentialRotationRecord(t, runtime, legacy, sourceID, actorID, plaintext)
		want[sourceID] = plaintext
	}
	rolling, err := NewStoreWithKeyring(runtime, 2, newKey, map[int]string{1: oldKey})
	if err != nil {
		t.Fatal(err)
	}
	var result RotationBatchResult
	if err := runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		result, err = rolling.RotateBatch(transactionCtx, actorID, 100)
		return err
	}); err != nil {
		t.Fatalf("RotateBatch() error = %v", err)
	}
	if result != (RotationBatchResult{CurrentVersion: 2, Scanned: 2, Rotated: 2, Remaining: 0}) {
		t.Fatalf("RotateBatch() = %#v", result)
	}
	currentOnly, _ := NewStoreWithKeyring(runtime, 2, newKey, nil)
	for sourceID, plaintext := range want {
		if got, resolveErr := currentOnly.Resolve(ctx, sourceID); resolveErr != nil || got != plaintext {
			t.Fatalf("current-only Resolve(%d) = %q, %v", sourceID, got, resolveErr)
		}
	}
}

func TestRotateBatchRollsBackTheWholeBatchWhenOneRecordIsInvalid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runtime := openCredentialRotationRuntime(t, ctx)
	defer runtime.Close()
	actorID := seedCredentialRotationActor(t, runtime)
	oldKey := encodedRotationKey(0x53)
	newKey := encodedRotationKey(0x64)
	legacy, _ := NewCipherKeyring(1, oldKey, nil)
	firstID := seedCredentialRotationSource(t, runtime, actorID, "rollback-source-1")
	secondID := seedCredentialRotationSource(t, runtime, actorID, "rollback-source-2")
	seedCredentialRotationRecord(t, runtime, legacy, firstID, actorID, "must-remain-old")
	seedCredentialRotationRecord(t, runtime, legacy, secondID, actorID, "will-be-corrupted")
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE source_credentials SET ciphertext = set_byte(ciphertext, 0, get_byte(ciphertext, 0) # 255) WHERE source_connection_id = $1`, secondID); err != nil {
		t.Fatal(err)
	}
	rolling, _ := NewStoreWithKeyring(runtime, 2, newKey, map[int]string{1: oldKey})
	err := runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		_, rotateErr := rolling.RotateBatch(transactionCtx, actorID, 100)
		return rotateErr
	})
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("must-remain-old")) || bytes.Contains([]byte(err.Error()), []byte("will-be-corrupted")) {
		t.Fatalf("RotateBatch() error = %v, want redacted failure", err)
	}
	for _, sourceID := range []int64{firstID, secondID} {
		var version int
		if err := runtime.SQL.QueryRowContext(ctx, `SELECT key_version FROM source_credentials WHERE source_connection_id = $1`, sourceID).Scan(&version); err != nil || version != 1 {
			t.Fatalf("source %d version after rollback = %d, %v", sourceID, version, err)
		}
	}
}

func openCredentialRotationRuntime(t *testing.T, ctx context.Context) *database.Runtime {
	t.Helper()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	return runtime
}

func seedCredentialRotationActor(t *testing.T, runtime *database.Runtime) int64 {
	t.Helper()
	var actorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email, password_hash, display_name, role) VALUES ('rotation-admin@example.test', 'hash', 'Rotation Admin', 'admin') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	return actorID
}

func seedCredentialRotationSource(t *testing.T, runtime *database.Runtime, actorID int64, name string) int64 {
	t.Helper()
	var sourceID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint, auth_type, credential_ref, created_by, updated_by) VALUES ('rss', $1, 'https://feeds.example.test/rss', 'bearer', 'managed:v1', $2, $2) RETURNING id`, name, actorID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	return sourceID
}

func seedCredentialRotationRecord(t *testing.T, runtime *database.Runtime, value *Cipher, sourceID, actorID int64, plaintext string) {
	t.Helper()
	sealed, err := value.Encrypt(sourceID, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO source_credentials (source_connection_id, key_version, nonce, ciphertext, created_by, updated_by) VALUES ($1, $2, $3, $4, $5, $5)`, sourceID, sealed.KeyVersion, sealed.Nonce, sealed.Ciphertext, actorID); err != nil {
		t.Fatal(err)
	}
}

func encodedRotationKey(value byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}
