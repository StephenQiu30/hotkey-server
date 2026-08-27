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
		"needs: [backend-static-acceptance, backend-acceptance, backend-vulnerability-acceptance, worker-recovery-acceptance, frontend-acceptance, agent-acceptance, compose-acceptance]",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("CI must parallelize backend gates without dropping browser prerequisite %q", fragment)
		}
	}
	if strings.Contains(workflow, "run: make ci\n") {
		t.Error("hosted CI must not serialize every backend gate behind one make ci step")
	}
}
