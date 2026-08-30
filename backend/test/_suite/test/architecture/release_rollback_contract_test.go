package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestG6ReleaseReadinessAndRollbackRehearsalRemainFailClosedAndNonDestructive(t *testing.T) {
	t.Parallel()

	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	runtimeReadiness := readReleaseRollbackFile(t, repository, "backend/internal/bootstrap/runtime_compatibility_readiness.go")
	for _, marker := range []string{
		`runtimeCompatibilityOrder = []string{"configuration", "schema", "openapi"}`,
		"runtime compatibility check failed",
		"VerifyEmbeddedOpenAPICompatibility",
		`contract.Paths["/api/v1/capabilities"]`,
		`contract.SecurityDefinitions["BearerAuth"]`,
	} {
		if !strings.Contains(runtimeReadiness, marker) {
			t.Errorf("runtime compatibility readiness is missing %q", marker)
		}
	}

	app := readReleaseRollbackFile(t, repository, "backend/internal/bootstrap/app.go")
	for _, marker := range []string{
		"newRuntimeCompatibilityReadiness(",
		"runtimeConfigurationCompatibilityCheck(cfg)",
		"database.Verify(ctx, runtime.Pool)",
		"verifyEmbeddedOpenAPICompatibility()",
	} {
		if !strings.Contains(app, marker) {
			t.Errorf("API assembly is missing release readiness marker %q", marker)
		}
	}

	drill := readReleaseRollbackFile(t, repository, "backend/test/tools/repeated-restore-drill/main.go")
	for _, marker := range []string{
		`json:"application_rollback"`,
		`[]string{"schema", "openapi", "configuration"}`,
		"AdmittedBusinessRequests: admitted",
		"compareAssets(before, after)",
		"validateApplicationRollbackEvidence(result)",
		"incompatible instance was not stopped before traffic",
		"application rollback changed protected assets",
	} {
		if !strings.Contains(drill, marker) {
			t.Errorf("repeated restore rollback drill is missing %q", marker)
		}
	}

	workflow := readReleaseRollbackFile(t, repository, ".github/workflows/ci.yml")
	if !strings.Contains(workflow, "make repeated-restore-rehearsal-acceptance") || !strings.Contains(workflow, "repeated-restore-${{ github.run_id }}-${{ github.run_attempt }}") {
		t.Error("CI does not execute and retain the application rollback rehearsal")
	}
	plan := readReleaseRollbackFile(t, repository, "docs/plans/005-安全运维质量与交付计划.md")
	if !strings.Contains(plan, "- [ ] `CHK-005-G6-003`") {
		t.Error("G6-003 must remain unchecked until remote evidence and release approval are recorded")
	}
}

func readReleaseRollbackFile(t *testing.T, repository, relative string) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(payload)
}
