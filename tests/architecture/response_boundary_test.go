package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureValidationRejectsDirectGinResponsesInModuleTransport(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	directory := filepath.Join(repositoryRoot, "internal", "modules", "architecturegate", "transport", "http")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create temporary transport directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(filepath.Join(repositoryRoot, "internal", "modules", "architecturegate")); err != nil {
			t.Errorf("remove temporary transport directory: %v", err)
		}
	})
	path := filepath.Join(directory, "direct_response.go")
	contents := []byte("package http\n\nimport \"github.com/gin-gonic/gin\"\n\nfunc directResponse(c *gin.Context) { c.JSON(200, nil) }\n")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write temporary transport file: %v", err)
	}

	command := exec.Command("sh", "scripts/validate-architecture.sh")
	command.Dir = repositoryRoot
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("architecture validator accepted direct Gin output:\n%s", output)
	}
	if !strings.Contains(string(output), "direct Gin response output") {
		t.Fatalf("validator failure did not identify direct Gin output:\n%s", output)
	}
}
