package fakejobs

import (
	"context"
	"fmt"
	"sync"
)

// ExportRecorder is a fake implementing jobs.TopicExporter.
// It tracks per-topic-date export status in memory.
type ExportRecorder struct {
	mu      sync.Mutex
	Exported map[string]bool   // "topicID:date" → true
	Failed   map[string]string // "topicID:date" → reason
	Err      error             // injected error for IsExported
}

func NewExportRecorder() *ExportRecorder {
	return &ExportRecorder{
		Exported: make(map[string]bool),
		Failed:   make(map[string]string),
	}
}

func key(topicID int64, date string) string {
	return fmt.Sprintf("%d:%s", topicID, date)
}

func (r *ExportRecorder) IsExported(_ context.Context, topicID int64, date string) (bool, error) {
	if r.Err != nil {
		return false, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Exported[key(topicID, date)], nil
}

func (r *ExportRecorder) MarkExported(_ context.Context, topicID int64, date string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Exported[key(topicID, date)] = true
	delete(r.Failed, key(topicID, date))
	return nil
}

func (r *ExportRecorder) MarkFailed(_ context.Context, topicID int64, date string, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Failed[key(topicID, date)] = reason
	return nil
}

// VaultWriterFake is a fake implementing jobs.VaultWriter.
// It writes to an in-memory map and can simulate permission errors.
type VaultWriterFake struct {
	mu       sync.Mutex
	Files    map[string]string // path → content
	ErrPaths map[string]error  // path → error to return
}

func NewVaultWriterFake() *VaultWriterFake {
	return &VaultWriterFake{
		Files:    make(map[string]string),
		ErrPaths: make(map[string]error),
	}
}

func (w *VaultWriterFake) WriteAtomic(path, content string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err, ok := w.ErrPaths[path]; ok {
		return err
	}
	w.Files[path] = content
	return nil
}

// VaultWriterAllFail is a fake that always returns a permission error.
type VaultWriterAllFail struct {
	Err error
}

func (w *VaultWriterAllFail) WriteAtomic(_, _ string) error {
	return w.Err
}
