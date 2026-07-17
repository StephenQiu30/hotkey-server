package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
)

type Writer struct{ root string }

func NewWriter(root string) *Writer { return &Writer{root: filepath.Clean(root)} }

func (writer *Writer) Write(kind, key, content string) (string, error) {
	if writer == nil {
		return "", fmt.Errorf("vault writer is required")
	}
	path, err := domain.StablePath(writer.root, kind, key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".hotkey-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", err
	}
	return path, nil
}
