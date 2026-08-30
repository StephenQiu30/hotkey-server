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
		"ci-test: test-env",
		"grep -v -e '/internal/platform/database$$' -e '/internal/shared/repository$$'",
		"test -p=2 $$packages -count=1",
		"ci-runtime: database-runtime schema ci-test",
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
		"HOTKEY_RC_GATE_BACKEND_STATIC: ${{ needs.backend-static-acceptance.result }}",
		"HOTKEY_RC_GATE_BACKEND_RUNTIME: ${{ needs.backend-acceptance.result }}",
		"HOTKEY_RC_GATE_BACKEND_VULNERABILITY: ${{ needs.backend-vulnerability-acceptance.result }}",
		"HOTKEY_RC_GATE_WORKER_RECOVERY: ${{ needs.worker-recovery-acceptance.result }}",
		"HOTKEY_RC_GATE_FRONTEND: ${{ needs.frontend-acceptance.result }}",
		"HOTKEY_RC_GATE_PYTHON_AGENT: ${{ needs.agent-acceptance.result }}",
		"HOTKEY_RC_GATE_COMPOSE: ${{ needs.compose-acceptance.result }}",
		"HOTKEY_RC_GATE_BROWSER_BUSINESS_FLOW: ${{ needs.browser-smoke-acceptance.result }}",
		"run: make rc-candidate-assessment",
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

func TestBrowserCIA11yAuditsWaitForVisualStateToSettle(t *testing.T) {
	backend := repositoryRoot(t)
	repository := filepath.Clean(filepath.Join(backend, ".."))
	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")

	searchResult := strings.Index(workflow, `agent-browser wait --text "BrowserAcceptanceTopic2026"`)
	desktopSettle := strings.Index(workflow, "agent-browser wait 300")
	desktopAudit := strings.Index(workflow, "hotkey-a11y-search-desktop.json")
	viewport := strings.Index(workflow, "agent-browser set viewport 390 844")
	if searchResult < 0 || desktopSettle <= searchResult || desktopAudit <= desktopSettle || viewport <= desktopAudit {
		t.Fatal("desktop search a11y audit must wait for CSS transitions to settle")
	}
	mobileWorkflow := workflow[viewport:]
	mobileSettle := strings.Index(mobileWorkflow, "agent-browser wait 300")
	mobileAudit := strings.Index(mobileWorkflow, "hotkey-a11y-search-mobile.json")
	if mobileSettle < 0 || mobileAudit <= mobileSettle {
		t.Fatal("mobile search a11y audit must wait for responsive layout transitions to settle")
	}
}

func TestBrowserCIExercisesFourRoleRouteAndKeyboardMatrix(t *testing.T) {
	backend := repositoryRoot(t)
	repository := filepath.Clean(filepath.Join(backend, ".."))
	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	fixture := readRepositoryFile(t, repository, "backend/test/fixtures/browser-acceptance/seed.sql")
	verifier := readRepositoryFile(t, repository, "frontend/test/browser/verify-artifacts.mjs")

	for _, fragment := range []string{
		"HOTKEY_BROWSER_VIEWER_EMAIL:",
		"HOTKEY_BROWSER_ANALYST_EMAIL:",
		"HOTKEY_BROWSER_EMAIL:",
		"HOTKEY_SECRET_ADMIN_EMAIL:",
		"hotkey-role-viewer.json",
		"hotkey-role-analyst.json",
		"hotkey-role-editor.json",
		"hotkey-role-admin.json",
		"hotkey-keyboard-focus.json",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("browser CI must retain four-role and keyboard evidence fragment %q", fragment)
		}
		if strings.HasPrefix(fragment, "hotkey-") && !strings.Contains(verifier, fragment) {
			t.Errorf("browser evidence verifier must fail closed on missing fragment %q", fragment)
		}
	}

	if got := strings.Count(workflow, `agent-browser find role menuitem click --name "退出登录" --exact`); got != 3 {
		t.Errorf("browser CI must switch four roles through three audited logout transitions, got %d", got)
	}
	if got := strings.Count(workflow, "agent-browser cookies clear"); got != 1 {
		t.Errorf("browser CI must clear cookies only before network capture starts, got %d", got)
	}

	for _, fragment := range []string{
		"browser-viewer@example.test",
		"'viewer'",
	} {
		if !strings.Contains(fixture, fragment) {
			t.Errorf("browser fixture must retain Viewer identity fragment %q", fragment)
		}
	}
}
