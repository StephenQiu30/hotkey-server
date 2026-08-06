package architecture

import (
	"os"
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
	contents := []byte("package http\n\nimport \"github.com/gin-gonic/gin\"\n\nfunc directJSON(ctx *gin.Context) { ctx.JSON(200, nil) }\nfunc directAbort(ctx *gin.Context) { ctx.AbortWithStatusJSON(400, nil) }\nfunc directString(ctx *gin.Context) { alias := ctx; alias.String(200, \"forbidden\") }\n")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write temporary transport file: %v", err)
	}

	err = ValidateHTTPTransport(filepath.Join(repositoryRoot, "internal", "modules"))
	if err == nil {
		t.Fatal("architecture validator accepted direct Gin output")
	}
	if !strings.Contains(err.Error(), "direct Gin response output") {
		t.Fatalf("validator failure did not identify direct Gin output:\n%s", err)
	}
}
