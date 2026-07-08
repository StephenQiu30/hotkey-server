package obsidian

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

func WriteAtomicNoOverwrite(path string, content []byte) (dto.WriteResult, error) {
	if _, err := os.Stat(path); err == nil {
		return dto.WriteResult{Path: path, Status: dto.WriteStatusSkipped, Skipped: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return dto.WriteResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return dto.WriteResult{}, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return dto.WriteResult{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return dto.WriteResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return dto.WriteResult{}, err
	}

	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return dto.WriteResult{Path: path, Status: dto.WriteStatusSkipped, Skipped: true}, nil
		}
		return dto.WriteResult{}, err
	}
	return dto.WriteResult{Path: path, Status: dto.WriteStatusPublished}, nil
}
