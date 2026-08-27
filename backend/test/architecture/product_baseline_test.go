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
	writeFixture("agent/pyproject.toml", "[project]\nname = 'hotkey-agent'\n")
	writeFixture("backend/pyproject.toml", "[project]\nname = 'second-backend'\n")
	if err := os.MkdirAll(filepath.Join(repository, "backend", "db", "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations := strings.Join(forbiddenInfrastructureViolations(t, repository), "\n")
	for _, expected := range []string{
		"backend/go.mod: forbidden Go dependency github.com/segmentio/kafka-go",
		"docker-compose.yml: forbidden service temporal",
		"docker-compose.yml: forbidden image temporalio/",
		"backend/pyproject.toml: Python manifest is only allowed under root agent/",
		"backend/db/migrations: incremental migrations directory is forbidden",
	} {
		if !strings.Contains(violations, expected) {
			t.Errorf("detector did not report %q; got:\n%s", expected, violations)
		}
	}
}

func TestPythonAgentIsTheOnlyApprovedAnalysisService(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	required := []string{
		"agent/pyproject.toml",
		"agent/Dockerfile",
		"agent/src/hotkey_agent/main.py",
		"agent/tests/test_api.py",
	}
	for _, relative := range required {
		info, err := os.Stat(filepath.Join(repository, filepath.FromSlash(relative)))
		if err != nil || info.IsDir() {
			t.Errorf("approved Python Agent artifact %s is missing", relative)
		}
	}

	compose := readRepositoryFile(t, repository, "docker-compose.yml")
	if !strings.Contains(compose, "  hotkey-agent:") {
		t.Error("development Compose does not declare the approved hotkey-agent service")
	}
	agentBlock := composeServiceBlock(compose, "hotkey-agent")
	if strings.Contains(agentBlock, "\n    ports:") {
		t.Error("hotkey-agent must not publish a host port")
	}
	for _, forbidden := range []string{
		"HOTKEY_DATABASE_URL",
		"HOTKEY_REDIS_URL",
		"HOTKEY_MINIO_SECRET_KEY",
		"HOTKEY_VAULT_PATH",
		"HOTKEY_JWT_SECRET",
	} {
		if strings.Contains(agentBlock, forbidden) {
			t.Errorf("hotkey-agent receives forbidden business credential %s", forbidden)
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

		if _, isPythonManifest := pythonManifests[name]; isPythonManifest && isProductionServicePath(relative) && !isAllowedPythonAgentManifest(relative) {
			violations = append(violations, relative+": Python manifest is only allowed under root agent/")
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

func composeServiceBlock(contents, service string) string {
	var block []string
	started := false
	for _, line := range strings.Split(contents, "\n") {
		if line == "  "+service+":" {
			started = true
			block = append(block, line)
			continue
		}
		if started && strings.HasPrefix(line, "  ") && len(line) > 2 && line[2] != ' ' && line[2] != '\t' {
			break
		}
		if started {
			block = append(block, line)
		}
	}
	return strings.Join(block, "\n")
}

func isProductionServicePath(relative string) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) == 1 {
		return true
	}
	switch parts[0] {
	case "agent", "backend", "services", "apps", "python", "src":
		return true
	default:
		return false
	}
}

func isAllowedPythonAgentManifest(relative string) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	return len(parts) == 2 && parts[0] == "agent"
}
