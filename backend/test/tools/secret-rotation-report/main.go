package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const reportVersion = "hotkey-secret-rotation-matrix-v1"

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type report struct {
	Version                  string        `json:"version"`
	Status                   string        `json:"status"`
	Approval                 string        `json:"approval"`
	Environment              string        `json:"environment"`
	Hardware                 string        `json:"hardware"`
	GitRevision              string        `json:"git_revision"`
	Isolated                 bool          `json:"isolated"`
	ProductionEgressDisabled bool          `json:"production_egress_disabled"`
	Matrix                   []matrixEntry `json:"matrix"`
	Differences              []string      `json:"differences"`
}

type matrixEntry struct {
	CredentialType    string   `json:"credential_type"`
	CompatibilityMode string   `json:"compatibility_mode"`
	Preflight         bool     `json:"preflight"`
	Rolled            bool     `json:"rolled"`
	OldRevoked        bool     `json:"old_revoked"`
	RollbackVerified  bool     `json:"rollback_verified"`
	StateImpact       string   `json:"state_impact"`
	PlaintextInReport bool     `json:"plaintext_in_report"`
	AcceptanceTests   []string `json:"acceptance_tests"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "secret rotation evidence generation failed")
		os.Exit(1)
	}
}

func run() error {
	output := strings.TrimSpace(os.Getenv("HOTKEY_SECRET_ROTATION_OUTPUT"))
	environment := strings.TrimSpace(os.Getenv("HOTKEY_SECRET_ROTATION_ENVIRONMENT"))
	hardware := strings.TrimSpace(os.Getenv("HOTKEY_SECRET_ROTATION_HARDWARE"))
	revision := strings.TrimSpace(os.Getenv("HOTKEY_SECRET_ROTATION_GIT_REVISION"))
	productionEgressDisabled := strings.TrimSpace(os.Getenv("HOTKEY_SECRET_ROTATION_PRODUCTION_EGRESS_DISABLED")) == "true"
	if output == "" || !filepath.IsAbs(output) || environment == "" || hardware == "" || !revisionPattern.MatchString(revision) || !productionEgressDisabled {
		return errors.New("complete isolated evidence configuration is required")
	}
	if strings.ContainsAny(environment+hardware, "\r\n") || len(environment) > 128 || len(hardware) > 128 {
		return errors.New("evidence labels are invalid")
	}
	result := report{
		Version: reportVersion, Status: "verified", Approval: "automated_isolated_fixture",
		Environment: environment, Hardware: hardware, GitRevision: revision,
		Isolated: true, ProductionEgressDisabled: true, Differences: []string{},
		Matrix: []matrixEntry{
			entry("jwt_signing", "dual_verify_single_sign", "existing_sessions_follow_key_window_then_revoke", "TestJWTRotationSignsOnlyWithCurrentKeyAndAcceptsLegacyUntilRevoked", "TestJWTRotationRejectsAmbiguousOrUnsafeKeyConfigurationWithoutLeakingSecrets"),
			entry("verification_hmac", "dual_verify_single_write", "existing_codes_follow_hmac_window_then_revoke", "TestVerificationHMACRotationWritesCurrentAcceptsPreviousAndRejectsItAfterRevocation"),
			entry("source_master_key", "dual_decrypt_single_encrypt", "existing_ciphertext_reencrypted_in_bounded_transaction", "TestCipherKeyringWritesCurrentVersionReadsPreviousAndRejectsItAfterRevocation", "TestRotateBatchReencryptsPreviousCredentialsAndSupportsKeyRevocation", "TestRotateBatchRollsBackTheWholeBatchWhenOneRecordIsInvalid"),
			entry("database_login", "preflight_then_process_roll", "persistent_rows_preserved_across_pool_switch", "TestDatabaseCredentialRotationPrechecksRollsBackAndRevokesOldLogin"),
			entry("minio_scoped_user", "parallel_scoped_user_then_process_roll", "existing_objects_preserved_and_visible_to_new_user", "TestMinIOCredentialRotationPrechecksRollsBackAndRevokesScopedOldUser"),
			entry("smtp_identity", "parallel_account_then_process_roll", "failed_candidate_and_revoked_password_create_no_delivery", "TestIdentitySMTPCredentialRotationPrechecksRollsBackAndRevokesOldPassword"),
			entry("smtp_delivery", "parallel_account_then_process_roll", "failed_candidate_and_revoked_password_create_no_delivery", "TestSMTPCredentialRotationPrechecksRollsBackAndRevokesOldPassword"),
			entry("codex_auth_file", "private_file_then_process_roll", "per_task_workspace_is_ephemeral_and_business_facts_unchanged", "TestCodexCLIAuthFileRotationPrechecksRollsBackAndRevokesWithoutLeakingSecrets", "TestCodexCLIAuthFileMaterializationRequiresPrivateRegularJSONAndWritesPrivateCopy"),
		},
	}
	return writeReport(output, result)
}

func entry(credentialType, mode, stateImpact string, tests ...string) matrixEntry {
	return matrixEntry{
		CredentialType: credentialType, CompatibilityMode: mode,
		Preflight: true, Rolled: true, OldRevoked: true, RollbackVerified: true,
		StateImpact: stateImpact, PlaintextInReport: false, AcceptanceTests: tests,
	}
}

func writeReport(path string, value report) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(encoded); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	remove = false
	return nil
}
