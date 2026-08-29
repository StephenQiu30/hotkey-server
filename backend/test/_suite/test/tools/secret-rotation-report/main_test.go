package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretRotationReportIsExclusivePrivateAndContainsCompleteMatrix(t *testing.T) {
	output := filepath.Join(t.TempDir(), "secret-rotation.json")
	t.Setenv("HOTKEY_SECRET_ROTATION_OUTPUT", output)
	t.Setenv("HOTKEY_SECRET_ROTATION_ENVIRONMENT", "isolated-fixture")
	t.Setenv("HOTKEY_SECRET_ROTATION_HARDWARE", "test-runner")
	t.Setenv("HOTKEY_SECRET_ROTATION_GIT_REVISION", "0123456789abcdef0123456789abcdef01234567")
	t.Setenv("HOTKEY_SECRET_ROTATION_PRODUCTION_EGRESS_DISABLED", "true")
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if err := run(); err == nil {
		t.Fatal("existing evidence report was overwritten")
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode = %o, want 600", info.Mode().Perm())
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var result report
	if err := json.Unmarshal(contents, &result); err != nil {
		t.Fatal(err)
	}
	if result.Version != reportVersion || result.Status != "verified" || len(result.Matrix) != 8 || len(result.Differences) != 0 {
		t.Fatalf("unexpected secret rotation report: %#v", result)
	}
	for _, item := range result.Matrix {
		if item.CredentialType == "" || item.CompatibilityMode == "" || !item.Preflight || !item.Rolled || !item.OldRevoked || !item.RollbackVerified || item.PlaintextInReport || len(item.AcceptanceTests) == 0 {
			t.Fatalf("incomplete matrix entry: %#v", item)
		}
	}
}
