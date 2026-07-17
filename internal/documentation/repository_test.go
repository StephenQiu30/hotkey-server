package docgovernance_test

import (
	"os"
	"path/filepath"
	"testing"

	documentation "github.com/StephenQiu30/hotkey-server/internal/documentation"
)

func TestLoadRepositoryParsesFormalDocumentsAndDecodedLocalLinks(t *testing.T) {
	root := t.TempDir()
	writeDocument(t, root, "docs/design/001-设计.md", "Design", "001", "docs/design/001-设计.md", "accepted", "[目标](目标%20文件.md)")
	writeDocument(t, root, "docs/design/目标 文件.md", "Design", "002", "docs/design/目标 文件.md", "review", "")

	repository, issues := documentation.LoadRepository(root, nil)
	if len(issues) != 0 {
		t.Fatalf("LoadRepository() issues = %#v, want none", issues)
	}
	if len(repository.Documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(repository.Documents))
	}
	if got := repository.Documents[0].Links; len(got) != 1 || got[0].Path != "docs/design/目标 文件.md" {
		t.Fatalf("decoded links = %#v, want docs/design/目标 文件.md", got)
	}
}

func TestLoadRepositoryReportsFrontmatterAndPathViolations(t *testing.T) {
	root := t.TempDir()
	writeDocument(t, root, "docs/design/001-first.md", "Design", "001", "docs/design/not-first.md", "unknown", "[escape](../../../secret.md)")
	writeDocument(t, root, "docs/design/001-second.md", "Design", "001", "docs/design/not-first.md", "review", "")
	writeRawDocument(t, root, "docs/prd/002-missing-layer.md", "---\ndoc_no: \"002\"\n---\n")

	_, issues := documentation.LoadRepository(root, nil)
	for _, rule := range []string{"frontmatter.required", "frontmatter.status", "document.doc_no_unique", "document.canonical_path_unique", "document.canonical_path", "link.outside_root"} {
		if !hasRule(issues, rule) {
			t.Errorf("issues = %#v, want rule %q", issues, rule)
		}
	}
}

func writeDocument(t *testing.T, root, path, layer, docNo, canonicalPath, status, body string) {
	t.Helper()
	writeRawDocument(t, root, path, "---\n"+
		"layer: "+layer+"\n"+
		"doc_no: \""+docNo+"\"\n"+
		"audience: [Dev]\n"+
		"feature_area: test\n"+
		"purpose: test document\n"+
		"canonical_path: "+canonicalPath+"\n"+
		"status: "+status+"\n"+
		"version: v1\n"+
		"owner: test\n"+
		"inputs: []\n"+
		"outputs: []\n"+
		"triggers: []\n"+
		"downstream: []\n"+
		"---\n\n"+body+"\n")
}

func writeRawDocument(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func hasRule(issues []documentation.Issue, rule string) bool {
	for _, issue := range issues {
		if issue.Rule == rule {
			return true
		}
	}
	return false
}
