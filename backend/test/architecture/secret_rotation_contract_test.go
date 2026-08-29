package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretRotationAcceptanceCoversEveryApprovedSyntheticCredentialBoundary(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	required := map[string][]string{
		"backend/internal/modules/intelligence/infrastructure/provider/codex_cli.go": {
			"AuthFile", "materializeCodexCLIAuth", "O_EXCL", "os.SameFile", "codex-home", "auth.json",
		},
		"backend/test/_suite/internal/modules/intelligence/infrastructure/provider/codex_cli_test.go": {
			"TestCodexCLIAuthFileRotationPrechecksRollsBackAndRevokesWithoutLeakingSecrets",
			"TestCodexCLIAuthFileMaterializationRequiresPrivateRegularJSONAndWritesPrivateCopy",
		},
		"backend/test/_suite/internal/platform/database/credential_rotation_integration_test.go": {
			"TestDatabaseCredentialRotationPrechecksRollsBackAndRevokesOldLogin", "NOLOGIN", "preserved",
		},
		"backend/test/_suite/internal/modules/ingestion/infrastructure/minio/credential_rotation_integration_test.go": {
			"TestMinIOCredentialRotationPrechecksRollsBackAndRevokesScopedOldUser", "policy attach", "user disable",
		},
		"backend/test/_suite/internal/modules/identity/infrastructure/smtp/mailer_test.go": {
			"TestIdentitySMTPCredentialRotationPrechecksRollsBackAndRevokesOldPassword",
		},
		"backend/test/_suite/internal/modules/delivery/infrastructure/smtp/mailer_test.go": {
			"TestSMTPCredentialRotationPrechecksRollsBackAndRevokesOldPassword",
		},
		"backend/test/tools/secret-rotation-report/main.go": {
			"hotkey-secret-rotation-matrix-v1", "automated_isolated_fixture", "plaintext_in_report", "O_EXCL",
		},
		"backend/Makefile": {
			"secret-rotation-core-acceptance", "secret-rotation-acceptance", "TestDatabaseCredentialRotation", "TestMinIOCredentialRotation", "Test.*SMTPCredentialRotation", "TestCodexCLIAuthFile",
		},
		".github/workflows/ci.yml": {
			"Synthetic credential rotation acceptance", "Upload sanitized credential rotation evidence", "HOTKEY_SECRET_ROTATION_PRODUCTION_EGRESS_DISABLED",
		},
		"docs/operations/006-密钥轮换与泄漏响应.md": {
			"make secret-rotation-acceptance", "HOTKEY_CODEX_AUTH_FILE", "33264824771",
		},
		"docs/acceptance/evidence/005/secret-rotation-github-ubuntu-7b13a1cc.json": {
			"hotkey-secret-rotation-matrix-v1", "7b13a1ccaf7ee103f0bd4e718b8297d51b933a20",
			"database_login", "minio_scoped_user", "smtp_identity", "smtp_delivery", "codex_auth_file",
			`"differences": []`,
		},
		".env.prod.example": {
			"HOTKEY_CODEX_AUTH_FILE=/run/secrets/hotkey-codex-auth.json",
		},
	}
	for path, fragments := range required {
		contents := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(contents, fragment) {
				t.Errorf("%s is missing secret rotation contract %q", path, fragment)
			}
		}
	}
	plan := readRepositoryFile(t, repository, "docs/plans/005-安全运维质量与交付计划.md")
	if !strings.Contains(plan, "- [x] `CHK-005-G4-010`") ||
		!strings.Contains(plan, "c2cd60e82200ccdcf4374798f7b91236cfca921d3205f362f4bd6a02740d99b8") ||
		!strings.Contains(plan, "42a0b6f1ea9953e73d00d7beba684a7e29fdd653ec96212524f0631cdaa83a60") {
		t.Error("secret rotation checklist must pin the fixed matrix and same-run secret-scan evidence")
	}
}
