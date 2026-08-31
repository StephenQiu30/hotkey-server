package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssessmentSeparatesGreenTechnicalGatesFromProductReleaseReadiness(t *testing.T) {
	repository := fixtureRepository(t, map[string]string{
		"001": "failed",
		"002": "failed",
		"005": "failed",
	})
	result, err := assess(validConfig(t, repository))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" || result.ReleaseReady || !result.Coverage.FullCI || !result.Coverage.E2E || !result.Coverage.Performance || !result.Coverage.Security || !result.Coverage.DocumentIntegrity {
		t.Fatalf("unexpected blocked RC assessment: %#v", result)
	}
	wantDefects := []string{
		"RC-001-ACCEPTANCE-NOT-PASSED",
		"RC-002-ACCEPTANCE-NOT-PASSED",
		"RC-003-ACCEPTANCE-MISSING",
		"RC-004-ACCEPTANCE-MISSING",
		"RC-005-ACCEPTANCE-NOT-PASSED",
	}
	if got := defectIDs(result.Defects); strings.Join(got, ",") != strings.Join(wantDefects, ",") {
		t.Fatalf("defect IDs = %v, want %v", got, wantDefects)
	}
	if len(result.Differences) != 0 || !result.P0Scope.Passed || result.P0Scope.UsedDeferredCapabilitiesAsEvidence {
		t.Fatalf("unexpected integrity or scope result: %#v", result)
	}
}

func TestAssessmentMarksCandidateOnlyWhenAllFiveAcceptancesPassed(t *testing.T) {
	repository := fixtureRepository(t, map[string]string{
		"001": "passed", "002": "passed", "003": "passed", "004": "passed", "005": "passed",
	})
	result, err := assess(validConfig(t, repository))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "candidate" || !result.ReleaseReady || len(result.Defects) != 0 || len(result.Documents) != 5 {
		t.Fatalf("unexpected candidate assessment: %#v", result)
	}
	for _, document := range result.Documents {
		if !document.TraceabilityComplete || document.AcceptanceStatus != "passed" {
			t.Fatalf("incomplete release document: %#v", document)
		}
	}
}

func TestRunWritesEvidenceThenFailsWhenARequiredTechnicalGateIsNotGreen(t *testing.T) {
	repository := fixtureRepository(t, map[string]string{
		"001": "passed", "002": "passed", "003": "passed", "004": "passed", "005": "passed",
	})
	cfg := validConfig(t, repository)
	cfg.RequiredGates["browser_business_flow"] = "failure"
	err := run(cfg)
	if !errors.Is(err, errTechnicalGate) {
		t.Fatalf("run() error = %v, want technical gate failure", err)
	}
	contents, readErr := os.ReadFile(cfg.OutputPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var result report
	if err := json.Unmarshal(contents, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.ReleaseReady || result.Coverage.E2E {
		t.Fatalf("technical failure was not preserved in report: %#v", result)
	}
	if err := run(cfg); err == nil {
		t.Fatal("a second run overwrote immutable RC evidence")
	}
	info, err := os.Stat(cfg.OutputPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode = %v / %v", info, err)
	}
}

func TestAssessmentRejectsBrokenCanonicalPathAndMarkdownLink(t *testing.T) {
	repository := fixtureRepository(t, map[string]string{
		"001": "passed", "002": "passed", "003": "passed", "004": "passed", "005": "passed",
	})
	plan := filepath.Join(repository, "docs", "plans", "003-计划.md")
	if err := os.WriteFile(plan, []byte(documentPayload("Plan", "003", "planned", "docs/plans/wrong.md")+"\n[TASK-003-S01](../missing.md) `SPEC-003-X` `CHK-003-X` `AC-003-001`\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := assess(validConfig(t, repository)); err == nil || !strings.Contains(err.Error(), "document integrity") {
		t.Fatalf("assessment error = %v", err)
	}
}

func validConfig(t *testing.T, repository string) config {
	t.Helper()
	return config{
		RepositoryRoot:           repository,
		OutputPath:               filepath.Join(t.TempDir(), "rc-assessment.json"),
		Environment:              "isolated-test",
		Hardware:                 "test-runner",
		GitRevision:              "0123456789abcdef0123456789abcdef01234567",
		CIRunID:                  "123456789",
		ProductionEgressDisabled: true,
		RequiredGates: map[string]string{
			"backend_static": "success", "backend_runtime": "success", "backend_race": "success", "backend_vulnerability": "success",
			"worker_recovery": "success", "frontend": "success", "python_agent": "success",
			"compose": "success", "browser_business_flow": "success",
		},
	}
}

func fixtureRepository(t *testing.T, acceptanceStatuses map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"design", "prd", "plans", "acceptance"} {
		if err := os.MkdirAll(filepath.Join(root, "docs", directory), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for number := 1; number <= 5; number++ {
		docNo := "00" + string(rune('0'+number))
		designRelative := "docs/design/" + docNo + "-设计.md"
		prdRelative := "docs/prd/" + docNo + "-需求.md"
		planRelative := "docs/plans/" + docNo + "-计划.md"
		writeFixture(t, root, designRelative, documentPayload("Design", docNo, "accepted", designRelative))
		writeFixture(t, root, prdRelative, documentPayload("PRD", docNo, "approved", prdRelative)+"\n`AC-"+docNo+"-001`\n")
		writeFixture(t, root, planRelative, documentPayload("Plan", docNo, "in_progress", planRelative)+
			"\n[Design](../design/"+docNo+"-设计.md) `TASK-"+docNo+"-S01-T01` `SPEC-"+docNo+"-CORE-001` `CHK-"+docNo+"-G4-001` `AC-"+docNo+"-001`\n")
		if status, ok := acceptanceStatuses[docNo]; ok {
			acceptanceRelative := "docs/acceptance/" + docNo + "-验收.md"
			writeFixture(t, root, acceptanceRelative, documentPayload("Acceptance", docNo, status, acceptanceRelative)+
				"\n| AC | 结果 |\n|---|---|\n| `AC-"+docNo+"-001` | "+map[bool]string{true: "passed", false: "partial"}[status == "passed"]+" |\n")
		}
	}
	writeFixture(t, root, "docker-compose.yml", "services:\n  postgres:\n  redis:\n  minio:\n  hotkey-agent:\n  hotkey-server:\n  hotkey-web:\n")
	writeFixture(t, root, "docker-compose-prod.yml", "services:\n  postgres:\n  redis:\n  minio:\n  hotkey-agent:\n  hotkey-server:\n  hotkey-web:\n")
	writeFixture(t, root, "backend/go.mod", "module example.test/hotkey\n\ngo 1.26\n")
	writeFixture(t, root, "frontend/package.json", "{}\n")
	writeFixture(t, root, "agent/pyproject.toml", "[project]\nname='agent'\n")
	return root
}

func documentPayload(layer, docNo, status, canonicalPath string) string {
	return "---\nlayer: " + layer + "\ndoc_no: \"" + docNo + "\"\nstatus: " + status + "\ncanonical_path: " + canonicalPath + "\n---\n\n# Fixture\n"
}

func writeFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func defectIDs(defects []defect) []string {
	ids := make([]string, len(defects))
	for index, item := range defects {
		ids[index] = item.ID
	}
	return ids
}
