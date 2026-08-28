package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendCIPreservesCanonicalCoverageAcrossParallelGates(t *testing.T) {
	backend := repositoryRoot(t)
	repository := filepath.Clean(filepath.Join(backend, ".."))
	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")

	for _, fragment := range []string{
		"ci-static: openapi-check vet build architecture repository",
		"ci-runtime: database-runtime schema test",
		"ci-vulnerability: vulnerability",
		"ci: ci-static ci-runtime ci-vulnerability",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("backend Makefile must preserve canonical CI coverage through %q", fragment)
		}
	}

	for _, fragment := range []string{
		"backend-static-acceptance:",
		"run: make ci-static",
		"backend-acceptance:",
		"run: make ci-runtime",
		"backend-vulnerability-acceptance:",
		"run: make ci-vulnerability",
		"all-acceptance:",
		"if: ${{ always() }}",
		"needs: [backend-static-acceptance, backend-acceptance, backend-vulnerability-acceptance, worker-recovery-acceptance, frontend-acceptance, agent-acceptance, compose-acceptance, browser-smoke-acceptance]",
		`test "${{ needs.browser-smoke-acceptance.result }}" = "success"`,
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("CI must parallelize gates without dropping final acceptance prerequisite %q", fragment)
		}
	}
	if strings.Contains(workflow, "run: make ci\n") {
		t.Error("hosted CI must not serialize every backend gate behind one make ci step")
	}
	browserStart := strings.Index(workflow, "  browser-smoke-acceptance:")
	finalStart := strings.Index(workflow, "  all-acceptance:")
	if browserStart < 0 || finalStart <= browserStart {
		t.Fatal("CI must declare browser smoke before the final acceptance gate")
	}
	if strings.Contains(workflow[browserStart:finalStart], "\n    needs:") {
		t.Error("browser smoke must start in parallel because it consumes no preceding job artifact")
	}
	browserWorkflow := workflow[browserStart:finalStart]
	anonymousReady := strings.Index(browserWorkflow, "agent-browser wait --load networkidle")
	networkReset := strings.Index(browserWorkflow, "agent-browser network requests --clear")
	errorReset := strings.Index(browserWorkflow, "agent-browser errors --clear")
	if anonymousReady < 0 || networkReset <= anonymousReady || errorReset <= anonymousReady {
		t.Error("browser smoke must finish anonymous login bootstrap before resetting captured network and page errors")
	}
}

func TestBrowserCIEnforcesSyntheticSecretScanningBeforeEvidenceUpload(t *testing.T) {
	backend := repositoryRoot(t)
	repository := filepath.Clean(filepath.Join(backend, ".."))
	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	browserStart := strings.Index(workflow, "  browser-smoke-acceptance:")
	finalStart := strings.Index(workflow, "  all-acceptance:")
	if browserStart < 0 || finalStart <= browserStart {
		t.Fatal("CI must preserve the browser acceptance job")
	}
	browserWorkflow := workflow[browserStart:finalStart]
	for _, fragment := range []string{
		"Generate masked synthetic secret canaries",
		"Exercise credential and HTTP error surfaces",
		"Export database secret surfaces",
		"Copy frontend and Vault delivery surfaces",
		"Scan synthetic secrets across outbound surfaces",
		"frontend/test/security/verify-secret-surfaces.mjs",
		"backend/test/fixtures/browser-acceptance/export-secret-surfaces.sql",
		"/tmp/hotkey-secret-surface-scan.json",
	} {
		if !strings.Contains(browserWorkflow, fragment) {
			t.Errorf("browser CI must retain synthetic secret acceptance fragment %q", fragment)
		}
	}
	generate := strings.Index(browserWorkflow, "Generate masked synthetic secret canaries")
	start := strings.Index(browserWorkflow, "Start a fresh container stack")
	scan := strings.Index(browserWorkflow, "Scan synthetic secrets across outbound surfaces")
	upload := strings.Index(browserWorkflow, "Upload sanitized browser acceptance evidence")
	if generate < 0 || start <= generate || scan <= start || upload <= scan {
		t.Error("secret canaries must be injected before startup and every surface scanned before evidence upload")
	}
}
