package architecture_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestAnalystRoleGapRemainsExplicitAcrossPublishedContracts(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"backend/internal/modules/identity/domain/user.go": {
			`RoleAdmin  Role = "admin"`,
			`RoleEditor Role = "editor"`,
			`RoleViewer Role = "viewer"`,
		},
		"backend/internal/platform/http/auth.go": {
			`RoleViewer Role = "viewer"`,
			`RoleEditor Role = "editor"`,
			`RoleAdmin  Role = "admin"`,
		},
		"backend/internal/modules/identity/transport/http/dto.go": {
			`enums:"admin,editor,viewer"`,
		},
		"backend/db/schema.sql": {
			"role IN ('admin', 'editor', 'viewer')",
		},
		"docs/openapi/swagger.json": {
			`"enum": [`,
			`"admin"`,
			`"editor"`,
			`"viewer"`,
		},
		"frontend/src/lib/domainEnums.ts": {
			`Admin = "admin"`,
			`Editor = "editor"`,
			`Viewer = "viewer"`,
		},
		"frontend/src/services/hotkey/hotkey-server/typings.d.ts": {
			`role?: "admin" | "editor" | "viewer"`,
		},
	}

	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		lower := strings.ToLower(payload)
		if strings.Contains(lower, `"analyst"`) || strings.Contains(lower, `'analyst'`) {
			t.Errorf("%s publishes analyst before the coordinated role migration is complete", relative)
		}
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing current-role fact %q", relative, fragment)
			}
		}
	}

	prd := readRepositoryFile(t, repository, "docs/prd/001-HotKey产品需求分析与总体架构.md")
	for _, statement := range []string{
		"当前权限只有 `viewer`、`editor`、`admin`，尚无独立 `analyst`",
		"在 Analyst 正式落地前，不得仅在前端伪造该角色",
	} {
		if !strings.Contains(prd, statement) {
			t.Errorf("PRD 001 no longer declares the current Analyst gap: missing %q", statement)
		}
	}
}

func TestP0RuntimeRejectsForbiddenDistributedInfrastructure(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	if violations := forbiddenInfrastructureViolations(t, repository); len(violations) > 0 {
		t.Fatalf("P0 runtime contains forbidden infrastructure:\n%s", strings.Join(violations, "\n"))
	}
}

func TestForbiddenInfrastructureDetectorCatchesErroneousIntroductions(t *testing.T) {
	repository := t.TempDir()
	writeFixture := func(relative, contents string) {
		t.Helper()
		path := filepath.Join(repository, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeFixture("backend/go.mod", "module fixture\nrequire github.com/segmentio/kafka-go v0.4.49\n")
	writeFixture("docker-compose.yml", "services:\n  temporal:\n    image: temporalio/auto-setup:latest\n")
	writeFixture("backend/pyproject.toml", "[project]\nname = 'second-backend'\n")
	if err := os.MkdirAll(filepath.Join(repository, "backend", "db", "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := strings.Join(forbiddenInfrastructureViolations(t, repository), "\n")
	for _, expected := range []string{
		"backend/go.mod: forbidden Go dependency github.com/segmentio/kafka-go",
		"docker-compose.yml: forbidden service temporal",
		"docker-compose.yml: forbidden image temporalio/",
		"backend/pyproject.toml: Python business-service manifest is forbidden",
		"backend/db/migrations: incremental migrations directory is forbidden",
	} {
		if !strings.Contains(violations, expected) {
			t.Errorf("detector did not report %q; got:\n%s", expected, violations)
		}
	}
}

func forbiddenInfrastructureViolations(t *testing.T, repository string) []string {
	t.Helper()
	goDependencies := []string{
		"github.com/segmentio/kafka-go",
		"github.com/twmb/franz-go",
		"github.com/ibm/sarama",
		"go.temporal.io/sdk",
		"github.com/elastic/go-elasticsearch",
		"github.com/nerzal/gocloak",
	}
	composeService := regexp.MustCompile(`(?m)^\s{2}(kafka|temporal|elasticsearch|keycloak):\s*$`)
	composeImage := regexp.MustCompile(`(?mi)^\s*image:\s*[^#\n]*(apache/kafka|confluentinc/|bitnami/kafka|temporalio/|elasticsearch:|docker\.elastic\.co/elasticsearch|keycloak/)`)
	pythonManifests := map[string]struct{}{
		"pyproject.toml":   {},
		"requirements.txt": {},
		"pipfile":          {},
		"poetry.lock":      {},
	}
	ignoredDirectories := map[string]struct{}{
		".git": {}, ".next": {}, "node_modules": {}, "vendor": {},
	}

	var violations []string
	err := filepath.WalkDir(repository, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(repository, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if _, ignored := ignoredDirectories[entry.Name()]; ignored {
				return filepath.SkipDir
			}
			if relative == "backend/db/migrations" {
				violations = append(violations, relative+": incremental migrations directory is forbidden")
				return filepath.SkipDir
			}
			return nil
		}

		name := strings.ToLower(entry.Name())
		if name == "go.mod" || name == "go.sum" {
			payload, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			lower := strings.ToLower(string(payload))
			for _, dependency := range goDependencies {
				if strings.Contains(lower, dependency) {
					violations = append(violations, fmt.Sprintf("%s: forbidden Go dependency %s", relative, dependency))
				}
			}
		}

		if isComposeFilename(name) {
			payload, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			contents := string(payload)
			for _, match := range composeService.FindAllStringSubmatch(contents, -1) {
				violations = append(violations, fmt.Sprintf("%s: forbidden service %s", relative, strings.ToLower(match[1])))
			}
			for _, match := range composeImage.FindAllStringSubmatch(contents, -1) {
				violations = append(violations, fmt.Sprintf("%s: forbidden image %s", relative, strings.ToLower(match[1])))
			}
		}

		if _, isPythonManifest := pythonManifests[name]; isPythonManifest && isProductionServicePath(relative) {
			violations = append(violations, relative+": Python business-service manifest is forbidden")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository for forbidden infrastructure: %v", err)
	}
	sort.Strings(violations)
	return violations
}

func readRepositoryFile(t *testing.T, repository, relative string) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(payload)
}

func isComposeFilename(name string) bool {
	return (strings.HasPrefix(name, "docker-compose") || strings.HasPrefix(name, "compose")) &&
		(strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"))
}

func isProductionServicePath(relative string) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) == 1 {
		return true
	}
	switch parts[0] {
	case "backend", "services", "apps", "python", "src":
		return true
	default:
		return false
	}
}
