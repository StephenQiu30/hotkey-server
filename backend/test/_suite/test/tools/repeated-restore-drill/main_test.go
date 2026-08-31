package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepeatedRestoreConfigRequiresRootComposeIsolationAndReproducibility(t *testing.T) {
	composeFile := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte("name: hotkey\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"HOTKEY_REPEATED_RESTORE_OUTPUT":                     filepath.Join(t.TempDir(), "result.json"),
		"HOTKEY_REPEATED_RESTORE_ENVIRONMENT":                "isolated-test",
		"HOTKEY_REPEATED_RESTORE_HARDWARE":                   "4 cpu / 8 GiB / local SSD",
		"HOTKEY_REPEATED_RESTORE_GIT_REVISION":               "0123456789abcdef0123456789abcdef01234567",
		"HOTKEY_REPEATED_RESTORE_PRODUCTION_EGRESS_DISABLED": "true",
		"HOTKEY_REPEATED_RESTORE_COMPOSE_FILE":               composeFile,
	} {
		t.Setenv(key, value)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if !got.ProductionEgressDisabled || got.ComposeFile != composeFile || got.Environment != "isolated-test" {
		t.Fatalf("loadConfig() = %#v", got)
	}
	t.Setenv("HOTKEY_REPEATED_RESTORE_PRODUCTION_EGRESS_DISABLED", "false")
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "PRODUCTION_EGRESS_DISABLED") {
		t.Fatalf("loadConfig(non-isolated) error = %v", err)
	}
}

func TestComposeProjectNamesAreUniqueBoundedAndCannotTargetFormalProject(t *testing.T) {
	roles := []string{"source", "restore-a", "restore-b", "failure"}
	seen := make(map[string]bool, len(roles))
	for _, role := range roles {
		name, err := composeProjectName("0123456789ab", role)
		if err != nil {
			t.Fatalf("composeProjectName(%q) error = %v", role, err)
		}
		if name == "hotkey" || len(name) > 63 || seen[name] || !strings.HasPrefix(name, "hkr-") {
			t.Fatalf("unsafe compose project name %q", name)
		}
		seen[name] = true
	}
	if _, err := composeProjectName("0123456789ab", "../../hotkey"); err == nil {
		t.Fatal("composeProjectName accepted an unsafe role")
	}
}

func TestBackupPreflightRejectsMissingCorruptAndIncompatiblePackagesBeforeMutation(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "postgres.dump")
	minioRoot := filepath.Join(root, "minio")
	vaultRoot := filepath.Join(root, "vault")
	if err := os.MkdirAll(minioRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(vaultRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(database, []byte("database-backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(minioRoot, "evidence.json"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "manual.md"), []byte("manual bytes  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := createBackupManifest(root, strings.Repeat("a", 64), strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateBackupPackage(root, manifest, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
		t.Fatalf("validateBackupPackage(valid) error = %v", err)
	}

	if err := os.Remove(database); err != nil {
		t.Fatal(err)
	}
	if err := validateBackupPackage(root, manifest, strings.Repeat("a", 64), strings.Repeat("b", 64)); failureCode(err) != "backup_missing" {
		t.Fatalf("missing failure = %v, code = %q", err, failureCode(err))
	}
	if err := os.WriteFile(database, []byte("database-backup-corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateBackupPackage(root, manifest, strings.Repeat("a", 64), strings.Repeat("b", 64)); failureCode(err) != "backup_checksum_mismatch" {
		t.Fatalf("corrupt failure = %v, code = %q", err, failureCode(err))
	}
	if err := validateBackupPackage(root, manifest, strings.Repeat("c", 64), strings.Repeat("b", 64)); failureCode(err) != "schema_incompatible" {
		t.Fatalf("schema failure = %v, code = %q", err, failureCode(err))
	}
}

func TestDirectoryInventoryIsStableAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b", "two"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "one"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := directoryInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := directoryInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.Count != 2 || len(first.SHA256) != 64 {
		t.Fatalf("inventory = %#v / %#v", first, second)
	}
	if err := os.Symlink(filepath.Join(root, "one"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := directoryInventory(root); err == nil {
		t.Fatal("directoryInventory accepted a symlink")
	}
}

func TestRepeatedRestoreEvidenceCannotOverwriteAnExistingRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence", "repeated-restore.json")
	value := report{Version: reportVersion, Status: "verified"}
	if err := writeExclusiveJSON(path, value); err != nil {
		t.Fatalf("writeExclusiveJSON(first) error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode = %v / %v", info, err)
	}
	if err := writeExclusiveJSON(path, value); err == nil {
		t.Fatal("writeExclusiveJSON(second) overwrote immutable evidence")
	}
}

func TestApplicationRollbackEvidenceRequiresZeroTrafficAndUnchangedAssets(t *testing.T) {
	t.Parallel()

	assets := []assetComparison{
		{Name: "postgres_facts", ExpectedCount: 2, ActualCount: 2, ExpectedSHA256: strings.Repeat("a", 64), ActualSHA256: strings.Repeat("a", 64)},
		{Name: "minio_evidence", ExpectedCount: 2, ActualCount: 2, ExpectedSHA256: strings.Repeat("b", 64), ActualSHA256: strings.Repeat("b", 64)},
		{Name: "vault_all_files", ExpectedCount: 2, ActualCount: 2, ExpectedSHA256: strings.Repeat("c", 64), ActualSHA256: strings.Repeat("c", 64)},
		{Name: "vault_manual_regions", ExpectedCount: 2, ActualCount: 2, ExpectedSHA256: strings.Repeat("d", 64), ActualSHA256: strings.Repeat("d", 64)},
		{Name: "river_jobs_attempts", ExpectedCount: 2, ActualCount: 2, ExpectedSHA256: strings.Repeat("e", 64), ActualSHA256: strings.Repeat("e", 64)},
	}
	valid := applicationRollbackResult{
		IncompatibleInstances: []readinessFixtureResult{
			{Contract: "schema", ReadinessStatus: 503, AdmittedBusinessRequests: 0, MutationStarted: false},
			{Contract: "openapi", ReadinessStatus: 503, AdmittedBusinessRequests: 0, MutationStarted: false},
			{Contract: "configuration", ReadinessStatus: 503, AdmittedBusinessRequests: 0, MutationStarted: false},
		},
		CompatibleReadinessStatus:          200,
		CompatibleAdmittedBusinessRequests: 1,
		Assets:                             assets,
		Differences:                        []string{},
	}
	if err := validateApplicationRollbackEvidence(valid); err != nil {
		t.Fatalf("validateApplicationRollbackEvidence(valid) error = %v", err)
	}

	invalid := valid
	invalid.IncompatibleInstances = append([]readinessFixtureResult(nil), valid.IncompatibleInstances...)
	invalid.IncompatibleInstances[1].AdmittedBusinessRequests = 1
	if err := validateApplicationRollbackEvidence(invalid); err == nil {
		t.Fatal("validateApplicationRollbackEvidence accepted traffic for an incompatible instance")
	}

	invalid = valid
	invalid.Assets = append([]assetComparison(nil), valid.Assets...)
	invalid.Assets[0].ActualSHA256 = strings.Repeat("f", 64)
	if err := validateApplicationRollbackEvidence(invalid); err == nil {
		t.Fatal("validateApplicationRollbackEvidence accepted a destructive rollback")
	}
}
