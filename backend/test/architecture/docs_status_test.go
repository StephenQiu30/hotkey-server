package architecture_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDocumentationLifecycleStatusesStayConsistent(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	docsRoot := filepath.Join(repository, "docs")

	for number := 1; number <= 5; number++ {
		docNo := fmt.Sprintf("%03d", number)
		designPath := oneNumberedDocument(t, filepath.Join(docsRoot, "design"), docNo)
		prdPath := oneNumberedDocument(t, filepath.Join(docsRoot, "prd"), docNo)
		planPath := oneNumberedDocument(t, filepath.Join(docsRoot, "plans"), docNo)

		designStatus := frontmatterStatus(t, designPath)
		prdStatus := frontmatterStatus(t, prdPath)
		planStatus := frontmatterStatus(t, planPath)

		assertLifecycleStatus(t, designPath, designStatus, "proposed", "accepted", "superseded")
		assertLifecycleStatus(t, prdPath, prdStatus, "draft", "approved", "implemented", "cancelled")
		assertLifecycleStatus(t, planPath, planStatus, "planned", "in_progress", "blocked", "completed")

		if planStatus != "planned" && designStatus != "accepted" {
			t.Errorf("%s plan status %q requires accepted Design, got %q", docNo, planStatus, designStatus)
		}
		if planStatus != "planned" && prdStatus != "approved" && prdStatus != "implemented" {
			t.Errorf("%s plan status %q requires approved or implemented PRD, got %q", docNo, planStatus, prdStatus)
		}
		if planStatus == "completed" && prdStatus != "implemented" {
			t.Errorf("%s completed Plan requires implemented PRD, got %q", docNo, prdStatus)
		}

		assertIndexStatus(t, filepath.Join(docsRoot, "design", "README.md"), docNo, designStatus)
		assertIndexStatus(t, filepath.Join(docsRoot, "prd", "README.md"), docNo, prdStatus)
		assertIndexStatus(t, filepath.Join(docsRoot, "plans", "README.md"), docNo, planStatus)
		assertIndexStatus(t, filepath.Join(docsRoot, "acceptance", "README.md"), docNo, planStatus)

		if planStatus == "completed" {
			acceptancePath := oneNumberedDocument(t, filepath.Join(docsRoot, "acceptance"), docNo)
			if status := frontmatterStatus(t, acceptancePath); status != "passed" {
				t.Errorf("%s completed Plan requires passed Acceptance, got %q", docNo, status)
			}
		}
	}
}

func oneNumberedDocument(t *testing.T, directory, docNo string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, docNo+"-*.md"))
	if err != nil {
		t.Fatalf("glob %s documents: %v", docNo, err)
	}
	if len(matches) != 1 {
		t.Fatalf("%s must contain exactly one %s document, got %v", directory, docNo, matches)
	}
	return matches[0]
}

func frontmatterStatus(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(payload), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		t.Fatalf("%s is missing frontmatter", path)
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if value, found := strings.CutPrefix(strings.TrimSpace(line), "status:"); found {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	t.Fatalf("%s frontmatter is missing status", path)
	return ""
}

func assertLifecycleStatus(t *testing.T, path, status string, allowed ...string) {
	t.Helper()
	for _, candidate := range allowed {
		if status == candidate {
			return
		}
	}
	t.Errorf("%s has unsupported status %q; want one of %v", path, status, allowed)
}

func assertIndexStatus(t *testing.T, indexPath, docNo, expected string) {
	t.Helper()
	payload, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read %s: %v", indexPath, err)
	}
	rowPattern := regexp.MustCompile(`(?m)^\|\s*` + regexp.QuoteMeta(docNo) + `\s*\|.*$`)
	row := rowPattern.FindString(string(payload))
	if row == "" {
		t.Fatalf("%s is missing a parseable %s status row", indexPath, docNo)
	}
	statusPattern := regexp.MustCompile(`\b(?:proposed|accepted|superseded|draft|approved|implemented|cancelled|planned|in_progress|blocked|completed|passed)\b`)
	statuses := statusPattern.FindAllString(row, -1)
	if len(statuses) == 0 {
		t.Fatalf("%s %s row has no lifecycle status", indexPath, docNo)
	}
	actual := statuses[len(statuses)-1]
	if actual != expected {
		t.Errorf("%s lists %s as %q, want %q", indexPath, docNo, actual, expected)
	}
}
