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

func TestPlatformDoesNotImportMonitorSourceOrOperationsImplementations(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := []string{
		"internal/modules/monitor/application",
		"internal/modules/monitor/infrastructure",
		"internal/modules/source/application",
		"internal/modules/source/infrastructure",
		"internal/modules/operations/application",
		"internal/modules/operations/infrastructure",
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
				t.Errorf("%s imports forbidden module implementation %q", path, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceInfrastructureDoesNotReadMonitorOwnedTables(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := []string{"monitor_sources", "monitor_config_versions"}
	err := filepath.WalkDir(filepath.Join(root, "internal", "modules", "source", "infrastructure"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, table := range forbidden {
			if strings.Contains(string(contents), table) {
				t.Errorf("%s reads Monitor-owned table %q; use a Monitor-owned read port", path, table)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
