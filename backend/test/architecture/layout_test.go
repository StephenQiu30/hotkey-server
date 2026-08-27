package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestSharedPackagesDoNotImportInfrastructure(t *testing.T) {
	root := repositoryRoot(t)
	sharedRoot := filepath.Join(root, "internal", "shared")
	forbidden := []string{
		"github.com/StephenQiu30/hotkey-server/backend/internal/platform",
		"gorm.io/gorm",
		"github.com/jackc/pgx",
		"github.com/gin-gonic/gin",
		"github.com/riverqueue/river",
		"github.com/minio/minio-go",
	}
	var violations []string
	err := filepath.WalkDir(sharedRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		imports, err := goImportPaths(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, importPath := range imports {
			for _, prefix := range forbidden {
				if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
					violations = append(violations, relative+" -> "+importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("shared packages import infrastructure: %s", strings.Join(violations, ", "))
	}
}

func TestProductionSourceCodeDoesNotCallRetiredBingSearchAPI(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "internal")
	violations := productionGoFilesContaining(t, root, []string{
		"api.bing.microsoft.com",
		"/v7.0/search",
	})
	if len(violations) > 0 {
		t.Fatalf("production code calls retired Bing Search API: %s", strings.Join(violations, ", "))
	}
}

func TestProductionSourceCodeDoesNotScrapeSogouSearchPages(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := os.Stat(filepath.Join(root, "internal", "modules", "source", "infrastructure", "sogou")); err == nil {
		t.Fatal("Sogou connector must not exist before an authorized read contract is accepted")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	violations := productionGoFilesContaining(t, filepath.Join(root, "internal"), []string{
		"sogou.com/web",
		"weixin.sogou.com",
	})
	if len(violations) > 0 {
		t.Fatalf("production code scrapes Sogou search pages: %s", strings.Join(violations, ", "))
	}

	schema, err := os.ReadFile(filepath.Join(root, "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(schema)), "'sogou'") {
		t.Fatal("database source enum must not register Sogou before authorization")
	}
}

func TestWeiboConnectorDoesNotCallWebOrMobilePrivateEndpoints(t *testing.T) {
	violations := productionGoFilesContaining(t, filepath.Join(repositoryRoot(t), "internal"), []string{
		"weibo.com/ajax/",
		"m.weibo.cn/api/",
		"weibo.com/hot/search",
		"weibo.com/newlogin",
	})
	if len(violations) > 0 {
		t.Fatalf("production code calls a Weibo web/mobile private endpoint: %s", strings.Join(violations, ", "))
	}
}

func TestGoogleAgentSearchDoesNotCallWebPagesOrRetiredCustomSearch(t *testing.T) {
	violations := productionGoFilesContaining(t, filepath.Join(repositoryRoot(t), "internal"), []string{
		"google.com/search?",
		"www.google.com/search",
		"customsearch.googleapis.com",
		"googleapis.com/customsearch",
	})
	if len(violations) > 0 {
		t.Fatalf("production code calls Google web pages or retired Custom Search: %s", strings.Join(violations, ", "))
	}
}

func TestDuckDuckGoRemainsANonExecutableCapabilityBoundary(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := os.Stat(filepath.Join(root, "internal", "modules", "source", "infrastructure", "duckduckgo")); err == nil {
		t.Fatal("DuckDuckGo connector must not exist without a documented production API contract")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	violations := productionGoFilesContaining(t, filepath.Join(root, "internal"), []string{
		"api.duckduckgo.com",
		"html.duckduckgo.com",
		"lite.duckduckgo.com",
		"duckduckgo.com/html",
		"duckduckgo.com/?q=",
	})
	if len(violations) > 0 {
		t.Fatalf("production code calls an undocumented DuckDuckGo endpoint: %s", strings.Join(violations, ", "))
	}

	schema, err := os.ReadFile(filepath.Join(root, "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	normalizedSchema := strings.ToLower(string(schema))
	for _, sourceType := range []string{"'duckduckgo'", "'duckduckgo_instant_answer'"} {
		if strings.Contains(normalizedSchema, sourceType) {
			t.Fatalf("database source enum must not register %s without a production API contract", sourceType)
		}
	}
}

func productionGoFilesContaining(t *testing.T, root string, forbidden []string) []string {
	t.Helper()
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := strings.ToLower(string(payload))
		for _, value := range forbidden {
			if strings.Contains(content, strings.ToLower(value)) {
				relative, _ := filepath.Rel(repositoryRoot(t), path)
				violations = append(violations, relative)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return violations
}

func TestImportPathLiteralSupportsQuotedAndRawStrings(t *testing.T) {
	for _, literal := range []string{`"gorm.io/gorm"`, "`gorm.io/gorm`"} {
		t.Run(literal, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "imports.go")
			source := "package fixture\nimport " + literal + "\n"
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			imports, err := goImportPaths(path)
			if err != nil {
				t.Fatalf("goImportPaths(): %v", err)
			}
			if len(imports) != 1 || imports[0] != "gorm.io/gorm" {
				t.Errorf("goImportPaths() = %#v, want gorm.io/gorm", imports)
			}
		})
	}
}

func goImportPaths(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		imports = append(imports, importPath)
	}
	return imports, nil
}

func TestGreenfieldLayout(t *testing.T) {
	if os.Getenv("HOTKEY_TEST_SUITE_ACTIVE") == "1" {
		return
	}
	root := repositoryRoot(t)
	required := []string{
		"internal/bootstrap",
		"internal/platform",
		"internal/shared",
		"internal/modules",
	}
	for _, relative := range required {
		info, err := os.Stat(filepath.Join(root, relative))
		if err != nil || !info.IsDir() {
			t.Errorf("required directory %s is missing", relative)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "db", "schema.sql")); err != nil || info.IsDir() {
		t.Errorf("required complete schema db/schema.sql is missing")
	}
	if _, err := os.Stat(filepath.Join(root, "db", "schema")); err == nil {
		t.Error("legacy split schema directory db/schema must not exist")
	}
	if _, err := os.Stat(filepath.Join(root, "scripts")); err == nil {
		t.Error("root scripts directory must not exist; test tools belong under test/")
	}

	forbidden := []string{
		"internal/controller",
		"internal/service",
		"internal/repository",
		"internal/model",
		"internal/queue",
		"internal/worker",
		"internal/fxapp",
	}
	for _, relative := range forbidden {
		if _, err := os.Stat(filepath.Join(root, relative)); err == nil {
			t.Errorf("legacy directory %s must not exist", relative)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "test", "_suite")); err != nil || !info.IsDir() {
		t.Error("centralized test suite test/_suite is missing")
	}
	var mixedTests []string
	var misplacedTestdata []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "test") {
			return filepath.SkipDir
		}
		if entry.IsDir() && entry.Name() == "testdata" {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			misplacedTestdata = append(misplacedTestdata, relative)
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			mixedTests = append(mixedTests, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan test placement: %v", err)
	}
	if len(mixedTests) > 0 {
		t.Errorf("test files must be kept under test/: %s", strings.Join(mixedTests, ", "))
	}
	if len(misplacedTestdata) > 0 {
		t.Errorf("testdata directories must be kept under test/: %s", strings.Join(misplacedTestdata, ", "))
	}
}

func TestRepositoryUsesSingleAgentRules(t *testing.T) {
	root := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	want := filepath.Join(root, "AGENTS.md")
	var found []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == ".next" || entry.Name() == ".venv" || entry.Name() == ".pytest_cache" || entry.Name() == ".mypy_cache" || entry.Name() == ".ruff_cache") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "AGENTS.md" {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != want {
		relative := make([]string, 0, len(found))
		for _, path := range found {
			value, relErr := filepath.Rel(root, path)
			if relErr != nil {
				t.Fatal(relErr)
			}
			relative = append(relative, filepath.ToSlash(value))
		}
		t.Errorf("repository must keep one root AGENTS.md, found: %s", strings.Join(relative, ", "))
	}
}

func TestBackendMakefileIsCanonicalAcceptanceEntryPoint(t *testing.T) {
	backendRoot := repositoryRoot(t)
	root := filepath.Clean(filepath.Join(backendRoot, ".."))

	makefile, err := os.ReadFile(filepath.Join(backendRoot, "Makefile"))
	if err != nil {
		t.Fatalf("read backend Makefile: %v", err)
	}
	makefileText := string(makefile)
	for _, fragment := range []string{
		"test-env:",
		"openapi:",
		"openapi-check: openapi",
		"vet:",
		"test: test-env",
		"build:",
		"architecture:",
		"repository:",
		"database-runtime: test-env",
		"schema: test-env",
		"vulnerability:",
		"ci-static: openapi-check vet build architecture repository",
		"ci-runtime: database-runtime schema test",
		"ci-vulnerability: vulnerability",
		"ci: ci-static ci-runtime ci-vulnerability",
	} {
		if !strings.Contains(makefileText, fragment) {
			t.Errorf("backend Makefile must contain %q", fragment)
		}
	}

	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflowText := string(workflow)
	for _, command := range []string{"run: make ci-static", "run: make ci-runtime", "run: make ci-vulnerability"} {
		if !strings.Contains(workflowText, command) {
			t.Errorf("backend CI must execute canonical parallel entry point %q", command)
		}
	}
	if strings.Contains(workflowText, "run: make ci\n") {
		t.Error("backend CI must keep independent acceptance gates parallel")
	}
	for _, duplicate := range []string{
		"go run ./test/runner vet ./...",
		"go run ./test/runner test ./... -count=1",
		"sh test/tools/verify-database-runtime.sh",
	} {
		if strings.Contains(workflowText, duplicate) {
			t.Errorf("backend CI duplicates Makefile command %q", duplicate)
		}
	}

	for _, relative := range []string{"README.md", "README_EN.md"} {
		readme, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, command := range []string{"make openapi", "make ci"} {
			if !strings.Contains(string(readme), command) {
				t.Errorf("%s must document %q", relative, command)
			}
		}
	}
}

func TestRepositoryKeepsCommonConfigurationAtRoot(t *testing.T) {
	root := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	rootOnlyFiles := map[string]struct{}{
		".editorconfig":   {},
		".gitattributes":  {},
		".gitignore":      {},
		"CONTRIBUTING.md": {},
		"LICENSE":         {},
		"SECURITY.md":     {},
	}
	rootOnlyDirectories := map[string]struct{}{
		".codex":  {},
		".github": {},
	}

	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == ".next" || entry.Name() == ".venv" || entry.Name() == ".pytest_cache" || entry.Name() == ".mypy_cache" || entry.Name() == ".ruff_cache") {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}

		parent := filepath.Dir(path)
		if entry.IsDir() {
			if _, rootOnly := rootOnlyDirectories[entry.Name()]; rootOnly && parent != root {
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				violations = append(violations, filepath.ToSlash(relative)+"/")
				return filepath.SkipDir
			}
			return nil
		}

		_, rootOnly := rootOnlyFiles[entry.Name()]
		isCompose := strings.HasPrefix(entry.Name(), "docker-compose") &&
			(strings.HasSuffix(entry.Name(), ".yml") || strings.HasSuffix(entry.Name(), ".yaml"))
		if (rootOnly || isCompose) && parent != root {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			violations = append(violations, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("common repository configuration must stay at root: %s", strings.Join(violations, ", "))
	}
}

func TestForbiddenRuntimeDependenciesAreRemoved(t *testing.T) {
	root := repositoryRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dependency := range []string{"github.com/segmentio/kafka-go"} {
		if strings.Contains(string(content), dependency) {
			t.Errorf("legacy dependency %s must be removed", dependency)
		}
	}
}

func TestBootstrapAdminConfigurationAndCommandAreRemoved(t *testing.T) {
	root := repositoryRoot(t)
	forbiddenFile := filepath.Join(root, "internal", "bootstrap", "user_command.go")
	if _, err := os.Stat(forbiddenFile); err == nil {
		t.Errorf("bootstrap administrator command must be removed: %s", forbiddenFile)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	var violations []string
	forbiddenTokens := []string{
		"BOOTSTRAP_ADMIN",
		"bootstrap_admin",
		"bootstrap-admin",
		"BootstrapAdmin",
		"ErrBootstrapUnavailable",
	}
	paths := []string{
		filepath.Join(root, "cmd"),
		filepath.Join(root, "internal"),
		filepath.Join(root, ".env.example"),
		filepath.Join(root, ".env.prod"),
		filepath.Join(root, "README.md"),
		filepath.Join(root, "README_EN.md"),
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		inspect := func(candidate string) error {
			content, err := os.ReadFile(candidate)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, candidate)
			if err != nil {
				return err
			}
			for _, token := range forbiddenTokens {
				if strings.Contains(string(content), token) {
					violations = append(violations, filepath.ToSlash(relative)+" -> "+token)
				}
			}
			return nil
		}
		if !info.IsDir() {
			if err := inspect(path); err != nil {
				t.Fatal(err)
			}
			continue
		}
		err = filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			return inspect(candidate)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) > 0 {
		t.Errorf("bootstrap administrator configuration or command remains: %s", strings.Join(violations, ", "))
	}
}

func TestLangChainGoStaysInsideIntelligenceProviderInfrastructure(t *testing.T) {
	root := repositoryRoot(t)
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "test") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "github.com/tmc/langchaingo") {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			if !strings.HasPrefix(filepath.ToSlash(relative), "internal/modules/intelligence/infrastructure/provider/") {
				violations = append(violations, relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("LangChainGo imports escape provider infrastructure: %s", strings.Join(violations, ", "))
	}
}
