package obsidian

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes content to path atomically by writing to a temporary
// file first, then renaming. Parent directories are created if needed.
// This prevents sync tools (iCloud, Dropbox) from reading partial files.
func WriteAtomic(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("obsidian: mkdir %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("obsidian: write tmp %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("obsidian: rename %s -> %s: %w", tmp, path, err)
	}

	return nil
}
