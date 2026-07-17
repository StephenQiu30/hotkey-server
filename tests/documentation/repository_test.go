package documentation_test

import (
	"path/filepath"
	"testing"

	documentation "github.com/StephenQiu30/hotkey-server/internal/documentation"
)

func TestLoadRepositoryAcceptsCurrentFormalDocumentSet(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("Abs(repository root): %v", err)
	}

	repository, issues := documentation.LoadRepository(root, nil)
	if repository == nil {
		t.Fatal("LoadRepository() repository = nil")
	}
	if len(repository.Documents) == 0 {
		t.Fatal("LoadRepository() documents = 0")
	}
	// Current historical documents are normalized by PLAN-018 Task 4. This
	// Task only establishes the parser's ability to enumerate them without
	// treating missing optional lifecycle fields as a parse failure.
	for _, issue := range issues {
		if issue.Rule == "frontmatter.parse" || issue.Rule == "document.read" {
			t.Fatalf("LoadRepository() fatal issue = %#v", issue)
		}
	}
}
