package obsidian

import (
	"errors"
	"os"
	"path/filepath"
)

func WriteAtomicNoOverwrite(path string, content []byte) (WriteResult, error) {
	if _, err := os.Stat(path); err == nil {
		return WriteResult{Path: path, Status: WriteStatusSkipped, Skipped: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return WriteResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WriteResult{}, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return WriteResult{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return WriteResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return WriteResult{}, err
	}

	if _, err := os.Stat(path); err == nil {
		return WriteResult{Path: path, Status: WriteStatusSkipped, Skipped: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return WriteResult{}, err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Path: path, Status: WriteStatusPublished}, nil
}
