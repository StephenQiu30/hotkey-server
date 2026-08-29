package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
)

func TestJointRecoveryConfigRequiresIsolationAndReproducibilityMetadata(t *testing.T) {
	for key, value := range map[string]string{
		"HOTKEY_TEST_DSN":                                  "postgres://fixture:fixture@127.0.0.1:5432/hotkey_test?sslmode=disable",
		"HOTKEY_TEST_MINIO_ENDPOINT":                       "127.0.0.1:9000",
		"HOTKEY_TEST_MINIO_ACCESS_KEY":                     "fixture-access",
		"HOTKEY_TEST_MINIO_SECRET_KEY":                     "fixture-secret",
		"HOTKEY_TEST_MINIO_BUCKET":                         "hotkey-fixture",
		"HOTKEY_JOINT_RECOVERY_OUTPUT":                     filepath.Join(t.TempDir(), "result.json"),
		"HOTKEY_JOINT_RECOVERY_ENVIRONMENT":                "isolated-test",
		"HOTKEY_JOINT_RECOVERY_HARDWARE":                   "4 cpu / 8 GiB / local SSD",
		"HOTKEY_JOINT_RECOVERY_GIT_REVISION":               "0123456789abcdef0123456789abcdef01234567",
		"HOTKEY_JOINT_RECOVERY_PRODUCTION_EGRESS_DISABLED": "true",
	} {
		t.Setenv(key, value)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if !got.ProductionEgressDisabled || got.Environment != "isolated-test" || got.Hardware == "" {
		t.Fatalf("loadConfig() = %#v", got)
	}
	t.Setenv("HOTKEY_JOINT_RECOVERY_PRODUCTION_EGRESS_DISABLED", "false")
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "PRODUCTION_EGRESS_DISABLED") {
		t.Fatalf("loadConfig(non-isolated) error = %v", err)
	}
}

func TestDatabaseDSNAndBucketNamesStayInsideGeneratedIsolationBoundary(t *testing.T) {
	got, err := databaseDSN("postgres://hotkey:secret@127.0.0.1:5432/hotkey_test?sslmode=disable", "hotkey_joint_source_0123")
	if err != nil || got != "postgres://hotkey:secret@127.0.0.1:5432/hotkey_joint_source_0123?sslmode=disable" {
		t.Fatalf("databaseDSN() = %q/%v", got, err)
	}
	if _, err := databaseDSN("postgres://hotkey:secret@127.0.0.1:5432/hotkey_test", "unsafe-name"); err == nil {
		t.Fatal("databaseDSN() accepted unsafe database name")
	}
	commandDSN, environment, err := postgresCommandConnection(got)
	if err != nil || strings.Contains(commandDSN, "secret") || len(environment) != 1 || environment[0] != "PGPASSWORD=secret" {
		t.Fatalf("postgresCommandConnection() = %q/%v/%v", commandDSN, environment, err)
	}
	bucket := recoveryBucketName(strings.Repeat("a", 70), "restore", "0123456789ab")
	if len(bucket) > 63 || !strings.HasSuffix(bucket, "-restore-0123456789ab") {
		t.Fatalf("recoveryBucketName() = %q", bucket)
	}
}

func TestRecoveryInventoriesAreStableAndCoverVersionedObjectsAndManualBytes(t *testing.T) {
	left := recordsInventory([]string{"b", "a"})
	right := recordsInventory([]string{"a", "b"})
	if left != right || left.Count != 2 || len(left.SHA256) != 64 {
		t.Fatalf("recordsInventory() = %#v / %#v", left, right)
	}
	objects := objectBackupsInventory([]objectBackup{
		{Key: "raw/z", ContentType: "application/json", SourceVersionID: "v2", Body: []byte("two"), UserMetadata: map[string]string{"fixture-version": "v1"}},
		{Key: "raw/a", ContentType: "text/plain", SourceVersionID: "v1", Body: []byte("one")},
	})
	if objects.Count != 2 || objects.VersionedCount != 2 || len(objects.SHA256) != 64 {
		t.Fatalf("objectBackupsInventory() = %#v", objects)
	}

	root := t.TempDir()
	input := knowledgedomain.VaultDocumentRenderInput{DocumentID: 17, RevisionNo: 3, Type: knowledgedomain.DocumentReport, SourceID: 91, Title: "Recovery", Generated: "generated"}
	content, err := knowledgedomain.RenderVaultDocument(input)
	if err != nil {
		t.Fatal(err)
	}
	content = strings.Replace(content, knowledgedomain.HumanRegionBegin+"\n"+knowledgedomain.HumanRegionEnd,
		knowledgedomain.HumanRegionBegin+"\nmanual bytes  \n"+knowledgedomain.HumanRegionEnd, 1)
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reports", "17.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	allFiles, manualRegions, err := vaultInventories(root)
	if err != nil {
		t.Fatal(err)
	}
	if allFiles.Count != 1 || manualRegions.Count != 1 || len(allFiles.SHA256) != 64 || len(manualRegions.SHA256) != 64 {
		t.Fatalf("vaultInventories() = %#v / %#v", allFiles, manualRegions)
	}
}

func TestJointRecoveryEvidenceCannotOverwriteAnExistingRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence", "joint.json")
	value := report{Version: toolVersion, Status: "reconciled"}
	if err := writeExclusiveJSON(path, value); err != nil {
		t.Fatalf("writeExclusiveJSON(first) error = %v", err)
	}
	if err := writeExclusiveJSON(path, value); err == nil {
		t.Fatal("writeExclusiveJSON(second) overwrote immutable evidence")
	}
}
