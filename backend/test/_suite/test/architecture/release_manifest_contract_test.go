package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestG5ReleaseManifestBindsBuiltImagesSBOMLocksContractsAndScanGates(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	tool := readRepositoryFile(t, repository, "backend/test/tools/release-manifest/main.go")
	for _, marker := range []string{
		`manifestVersion = "hotkey-release-manifest-v1"`,
		`BOMFormat: "CycloneDX", SpecVersion: "1.6"`,
		`DependencyLocksRequired: true`,
		`ImmutableImageIDsRequired: true`,
		`ReleaseReady:`,
		`os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600`,
	} {
		if !strings.Contains(tool, marker) {
			t.Errorf("release manifest tool is missing %q", marker)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	if !strings.Contains(makefile, "release-manifest-acceptance:") || !strings.Contains(makefile, "$(GO) run ./test/tools/release-manifest") {
		t.Error("backend Makefile does not expose the release manifest gate")
	}
	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, marker := range []string{
		"Record content-addressed release image inventory",
		"HOTKEY_RELEASE_GATE_BACKEND_VULNERABILITY: ${{ needs.backend-vulnerability-acceptance.result }}",
		"HOTKEY_RELEASE_GATE_FRONTEND_VULNERABILITY: ${{ needs.frontend-acceptance.result }}",
		"HOTKEY_RELEASE_GATE_AGENT_VULNERABILITY: ${{ needs.agent-acceptance.result }}",
		"release-manifest-${{ github.run_id }}-${{ github.run_attempt }}",
	} {
		if !strings.Contains(workflow, marker) {
			t.Errorf("CI does not bind release evidence marker %q", marker)
		}
	}
}
