package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
)

type Writer struct {
	root string
	mu   sync.Mutex
}

func NewWriter(root string) *Writer {
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		absolute = filepath.Clean(root)
	}
	return &Writer{root: absolute}
}

func (writer *Writer) Write(kind, key, content string) (string, error) {
	if writer == nil {
		return "", fmt.Errorf("vault writer is required")
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	path, err := writer.safePath(kind, key)
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

func (writer *Writer) WriteAutomatic(kind, key, generated string) (string, error) {
	if writer == nil {
		return "", fmt.Errorf("vault writer is required")
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	path, err := writer.safePath(kind, key)
	if err != nil {
		return "", err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	merged, err := domain.MergeAutomaticRegion(string(existing), generated)
	if err != nil {
		return "", err
	}
	return writer.writeAtomic(path, merged)
}

func (writer *Writer) Read(kind, key string) ([]byte, string, error) {
	if writer == nil {
		return nil, "", fmt.Errorf("vault writer is required")
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	path, err := writer.safePath(kind, key)
	if err != nil {
		return nil, "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	return content, path, nil
}

// CleanupTemporary removes only writer-owned dot-files below the configured
// root. It never follows symlinked directories and returns the number removed.
func (writer *Writer) CleanupTemporary() (int, error) {
	if writer == nil {
		return 0, fmt.Errorf("vault writer is required")
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := writer.ensureRoot(); err != nil {
		return 0, err
	}
	removed := 0
	err := filepath.WalkDir(writer.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == writer.root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".hotkey-") && strings.HasSuffix(entry.Name(), ".tmp") {
			if err := os.Remove(path); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	return removed, err
}

func (writer *Writer) ListFiles() ([]domain.VaultFile, error) {
	if writer == nil {
		return nil, fmt.Errorf("vault writer is required")
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := writer.ensureRoot(); err != nil {
		return nil, err
	}
	files := make([]domain.VaultFile, 0)
	err := filepath.WalkDir(writer.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == writer.root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".hotkey-") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(writer.root, path)
		if err != nil {
			return err
		}
		files = append(files, domain.VaultFile{Path: filepath.ToSlash(relative), Hash: domain.HashContent("", string(content))})
		return nil
	})
	return files, err
}

func (writer *Writer) safePath(kind, key string) (string, error) {
	if err := writer.ensureRoot(); err != nil {
		return "", err
	}
	path, err := domain.StablePath(writer.root, kind, key)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkComponents(writer.root, filepath.Dir(path)); err != nil {
		return "", err
	}
	return path, nil
}

func (writer *Writer) ensureRoot() error {
	if strings.TrimSpace(writer.root) == "" {
		return fmt.Errorf("vault root is required")
	}
	if info, err := os.Lstat(writer.root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("vault root must not be a symlink")
		}
		if !info.IsDir() {
			return fmt.Errorf("vault root is not a directory")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(writer.root, 0o755)
}

func rejectSymlinkComponents(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("vault path escapes root")
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("vault path contains symlink")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (writer *Writer) writeAtomic(path, content string) (string, error) {
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
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
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
