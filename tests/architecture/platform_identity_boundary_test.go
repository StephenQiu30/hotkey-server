package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformDoesNotImportIdentityApplicationOrDomain(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := []string{
		"internal/modules/identity/application",
		"internal/modules/identity/domain",
	}
	err := filepath.WalkDir(filepath.Join(root, "internal", "platform"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, value := range forbidden {
			if strings.Contains(string(contents), value) {
				t.Errorf("%s imports forbidden identity implementation %q", path, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
