package bootstrap

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	sourcecredentialstore "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/credentialstore"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestSourceCredentialRotationCommandDryRunsThenAppliesWithSanitizedAudit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn := postgresfixture.New(t)
	runtime, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x71}, 32))
	newKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x72}, 32))
	actorID, sourceID := seedSourceCredentialRotationCommand(t, runtime, oldKey)
	_ = runtime.Close()
	cfg := config.Default()
	cfg.Role = "worker"
	cfg.DatabaseURL = dsn
	cfg.SourceCredentialMasterKey = newKey
	cfg.SourceCredentialMasterKeyVersion = 2
	cfg.SourceCredentialPreviousMasterKey = oldKey
	cfg.SourceCredentialPreviousMasterKeyVersion = 1

	arguments := []string{"--batch-size", "100", "--actor-user-id", fmt.Sprint(actorID)}
	var dryRunOutput strings.Builder
	if err := runSourceCredentialRotationCommand(ctx, cfg, append(arguments, "--dry-run"), &dryRunOutput); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	runtime, _ = database.Open(ctx, dsn)
	defer runtime.Close()
	assertSourceCredentialRotationVersion(t, runtime, sourceID, 1)
	var auditCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = 'source_credentials.rotated'`).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("dry-run audit count = %d, %v", auditCount, err)
	}

	var applyOutput strings.Builder
	if err := runSourceCredentialRotationCommand(ctx, cfg, append(arguments, "--apply"), &applyOutput); err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertSourceCredentialRotationVersion(t, runtime, sourceID, 2)
	var beforeData, afterData string
	if err := runtime.SQL.QueryRow(`SELECT before_data::text, after_data::text FROM audit_logs WHERE action = 'source_credentials.rotated'`).Scan(&beforeData, &afterData); err != nil {
		t.Fatal(err)
	}
	combined := dryRunOutput.String() + applyOutput.String() + beforeData + afterData
	if strings.Contains(combined, oldKey) || strings.Contains(combined, newKey) ||
		!strings.Contains(applyOutput.String(), "current_key_version=2") ||
		!strings.Contains(afterData, `"rotated_count": 1`) || !strings.Contains(afterData, `"remaining_count": 0`) {
		t.Fatalf("rotation evidence is incomplete or leaked secret material: %s", combined)
	}
}

func TestSourceCredentialRotationCommandRejectsUnsafeOrAmbiguousFlags(t *testing.T) {
	for _, arguments := range [][]string{
		nil,
		{"--dry-run", "--apply", "--actor-user-id", "1"},
		{"--apply", "--actor-user-id", "0"},
		{"--dry-run", "--actor-user-id", "1", "--batch-size", "0"},
		{"--dry-run", "--actor-user-id", "1", "unexpected"},
	} {
		if err := runSourceCredentialRotationCommand(t.Context(), config.Default(), arguments, &strings.Builder{}); err == nil {
			t.Fatalf("arguments %#v were accepted", arguments)
		}
	}
}

func seedSourceCredentialRotationCommand(t *testing.T, runtime *database.Runtime, oldKey string) (int64, int64) {
	t.Helper()
	var actorID, sourceID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email, password_hash, display_name, role) VALUES ('rotation-command-admin@example.test', 'hash', 'Rotation Admin', 'admin') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint, auth_type, credential_ref, created_by, updated_by) VALUES ('rss', 'rotation-command-source', 'https://feeds.example.test/rss', 'bearer', 'managed:v1', $1, $1) RETURNING id`, actorID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	legacy, _ := sourcecredentialstore.NewCipherKeyring(1, oldKey, nil)
	sealed, _ := legacy.Encrypt(sourceID, []byte("command-provider-secret"))
	if _, err := runtime.SQL.Exec(`INSERT INTO source_credentials (source_connection_id, key_version, nonce, ciphertext, created_by, updated_by) VALUES ($1, $2, $3, $4, $5, $5)`, sourceID, sealed.KeyVersion, sealed.Nonce, sealed.Ciphertext, actorID); err != nil {
		t.Fatal(err)
	}
	return actorID, sourceID
}

func assertSourceCredentialRotationVersion(t *testing.T, runtime *database.Runtime, sourceID int64, want int) {
	t.Helper()
	var got int
	if err := runtime.SQL.QueryRow(`SELECT key_version FROM source_credentials WHERE source_connection_id = $1`, sourceID).Scan(&got); err != nil || got != want {
		t.Fatalf("source credential version = %d, %v; want %d", got, err, want)
	}
}
