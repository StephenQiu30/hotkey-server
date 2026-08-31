package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const backendModulePath = "github.com/StephenQiu30/hotkey-server/backend"

// TestModuleLayersKeepInwardDependencies protects the modular-monolith
// dependency direction. Application and Domain code may define ports, while
// concrete adapters remain behind Infrastructure and Transport boundaries.
func TestModuleLayersKeepInwardDependencies(t *testing.T) {
	root := repositoryRoot(t)
	modulesRoot := filepath.Join(root, "internal", "modules")
	forbiddenByLayer := map[string][]string{
		"application": {
			backendModulePath + "/internal/modules/*/infrastructure",
			backendModulePath + "/internal/modules/*/transport",
			backendModulePath + "/internal/platform",
		},
		"domain": {
			backendModulePath + "/internal/modules/*/application",
			backendModulePath + "/internal/modules/*/infrastructure",
			backendModulePath + "/internal/modules/*/transport",
			backendModulePath + "/internal/platform",
			"github.com/gin-gonic/gin",
			"github.com/jackc/pgx",
			"github.com/minio/minio-go",
			"gorm.io/gorm",
		},
		"transport": {
			backendModulePath + "/internal/modules/*/infrastructure",
		},
	}

	var violations []string
	err := filepath.WalkDir(modulesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(modulesRoot, path)
		if err != nil {
			return err
		}
		segments := strings.Split(filepath.ToSlash(relative), "/")
		if len(segments) < 3 {
			return nil
		}
		layer := segments[1]
		forbidden, protected := forbiddenByLayer[layer]
		if !protected {
			return nil
		}
		imports, err := goImportPaths(path)
		if err != nil {
			return err
		}
		for _, importPath := range imports {
			for _, pattern := range forbidden {
				if importMatchesLayerPattern(importPath, pattern) &&
					!legacyApplicationTransactionDependency(filepath.ToSlash(relative), importPath) {
					violations = append(violations, filepath.ToSlash(relative)+" -> "+importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("module layer dependencies point outward:\n%s", strings.Join(violations, "\n"))
	}
}

// These services predate the inward-only gate and use the shared transaction
// coordinator directly. The finite baseline prevents the exception from
// spreading while each service is migrated to a module-owned unit-of-work
// port in a behavior-preserving change.
func legacyApplicationTransactionDependency(relative, importPath string) bool {
	if importPath != backendModulePath+"/internal/platform/database" {
		return false
	}
	legacy := map[string]bool{
		"agentaccess/application/service.go":       true,
		"delivery/application/subscription.go":     true,
		"identity/application/service.go":          true,
		"ingestion/application/lifecycle.go":       true,
		"ingestion/application/service.go":         true,
		"monitor/application/service.go":           true,
		"operations/application/governance.go":     true,
		"source/application/collection_control.go": true,
		"source/application/collection_service.go": true,
		"source/application/metric_capability.go":  true,
		"source/application/service.go":            true,
	}
	return legacy[relative]
}

func importMatchesLayerPattern(importPath, pattern string) bool {
	wildcard := strings.IndexByte(pattern, '*')
	if wildcard < 0 {
		return importPath == pattern || strings.HasPrefix(importPath, pattern+"/")
	}
	prefix := pattern[:wildcard]
	suffix := pattern[wildcard+1:]
	if !strings.HasPrefix(importPath, prefix) {
		return false
	}
	remainder := strings.TrimPrefix(importPath, prefix)
	separator := strings.IndexByte(remainder, '/')
	if separator <= 0 {
		return false
	}
	matched := prefix + remainder[:separator] + suffix
	return importPath == matched || strings.HasPrefix(importPath, matched+"/")
}
