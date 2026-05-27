package governance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestP0AcceptanceEvidenceIsComplete(t *testing.T) {
	for i := 2; i <= 13; i++ {
		pattern := filepath.Join("..", "..", "docs", "acceptance", fmt.Sprintf("%d-*.md", i))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		if len(matches) != 1 {
			t.Fatalf("acceptance doc %d matches = %d, want 1: %#v", i, len(matches), matches)
		}
	}
}

func TestPRDAndPlanNumberingIsContinuous(t *testing.T) {
	assertContinuousDocs(t, filepath.Join("..", "..", "docs", "product", "prd"), 25)
	assertContinuousDocs(t, filepath.Join("..", "..", "docs", "plans"), 30)
	assertDocNumberMetadataMatchesFilename(t, filepath.Join("..", "..", "docs", "product", "prd"))
	assertDocNumberMetadataMatchesFilename(t, filepath.Join("..", "..", "docs", "plans"))
}

func TestLegacyFastAPIRuntimeIsAbsent(t *testing.T) {
	for _, path := range []string{
		"server",
		"tests",
		"scripts",
		"sql",
		"packages",
		"deploy",
		"openspec/changes",
		"openspec/specs",
		"package.json",
		"pyproject.toml",
		"Dockerfile.api",
		"docker-compose.yml",
		"docker-compose.prod.yml",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", path)); err == nil {
			t.Fatalf("legacy runtime path still exists: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func TestNoSuperpowersProcessDocsRemainInRepository(t *testing.T) {
	err := filepath.WalkDir(filepath.Join("..", "..", "docs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if regexp.MustCompile(`(?i)superpowers`).Match(content) {
			t.Fatalf("process-only superpowers reference found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
}

func assertContinuousDocs(t *testing.T, dir string, max int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	seen := map[string]bool{}
	re := regexp.MustCompile(`^([0-9]+)-.*\.md$`)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(entry.Name())
		if len(matches) == 0 {
			continue
		}
		seen[matches[1]] = true
	}
	for i := 1; i <= max; i++ {
		number := fmt.Sprintf("%d", i)
		if !seen[number] {
			t.Fatalf("%s missing numbered document %s", dir, number)
		}
	}
	if len(seen) != max {
		t.Fatalf("%s numbered document count = %d, want %d", dir, len(seen), max)
	}
}

func assertDocNumberMetadataMatchesFilename(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	re := regexp.MustCompile(`^([0-9]+)-.*\.md$`)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(entry.Name())
		if len(matches) == 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		number := matches[1]
		docNoPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^doc_no:\s+"%s"$`, regexp.QuoteMeta(number)))
		if !docNoPattern.Match(content) {
			t.Fatalf("%s doc_no does not match filename number %s", path, number)
		}
		titlePattern := regexp.MustCompile(fmt.Sprintf(`(?m)^# %s-`, regexp.QuoteMeta(number)))
		if !titlePattern.Match(content) {
			t.Fatalf("%s title does not start with document number %s", path, number)
		}
	}
}
