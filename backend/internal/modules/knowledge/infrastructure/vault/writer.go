package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	"github.com/google/uuid"
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

// PutIfAbsent writes an immutable document projection without exposing an
// absolute Vault path. A repeated identical write is idempotent; a different
// payload for the same document version is a conflict and never overwrites
// the existing projection.
func (writer *Writer) PutIfAbsent(ctx context.Context, command knowledgeapplication.StoreProjectionCommand) (knowledgeapplication.ProjectionStoreReceiptDTO, error) {
	if writer == nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, fmt.Errorf("vault writer is required")
	}
	projection, err := projectionManifestFromStoreCommand(command)
	if err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	if err := ctx.Err(); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()

	relativePath, err := validateProjectionRelativePath(projection.relativePath)
	if err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	root, err := writer.openRoot()
	if err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	defer root.Close()
	if receipt, found, err := projectionReceiptAt(root, relativePath, projection); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	} else if found {
		return receipt, nil
	}
	directory := filepath.Dir(relativePath)
	if err := root.MkdirAll(directory, 0o755); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	if err := rejectRootSymlinkComponents(root, directory); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	temporaryPath := filepath.Join(directory, ".hotkey-projection-"+uuid.NewString()+".tmp")
	temporary, err := root.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	defer root.Remove(temporaryPath)
	if _, err := temporary.Write(projection.content); err != nil {
		_ = temporary.Close()
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	if err := temporary.Close(); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	if err := ctx.Err(); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	// A hard link publishes the complete temporary file atomically and fails
	// with EEXIST instead of replacing an immutable target.
	if err := root.Link(temporaryPath, relativePath); err != nil {
		if !os.IsExist(err) {
			return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
		}
		if receipt, found, verifyErr := projectionReceiptAt(root, relativePath, projection); verifyErr != nil {
			return knowledgeapplication.ProjectionStoreReceiptDTO{}, verifyErr
		} else if found {
			return receipt, nil
		}
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, knowledgeapplication.ErrProjectionConflict
	}
	if err := syncRootDirectory(root, directory); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, err
	}
	return projectionReceipt(projection), nil
}

// ReadProjection verifies the caller's immutable receipt against both the
// deterministic path and the bytes on disk before returning content.
func (writer *Writer) ReadProjection(ctx context.Context, command knowledgeapplication.ReadStoredProjectionCommand) (knowledgeapplication.StoredProjectionContentDTO, error) {
	if writer == nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, fmt.Errorf("vault writer is required")
	}
	receipt, err := projectionReceiptRecordFromDTO(command.Receipt)
	if err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	if command.MaxBytes <= 0 {
		return knowledgeapplication.StoredProjectionContentDTO{}, knowledgeapplication.ErrProjectionInvalid
	}
	if receipt.sizeBytes > command.MaxBytes {
		return knowledgeapplication.StoredProjectionContentDTO{}, knowledgeapplication.ErrProjectionTooLarge
	}
	if err := ctx.Err(); err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	relativePath, err := validateProjectionRelativePath(receipt.relativePath)
	if err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	root, err := writer.openRoot()
	if err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	defer root.Close()
	if err := rejectRootSymlinkComponents(root, filepath.Dir(relativePath)); err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	info, err := root.Lstat(relativePath)
	if err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return knowledgeapplication.StoredProjectionContentDTO{}, fmt.Errorf("%w: projection is not a regular file", knowledgeapplication.ErrProjectionIntegrity)
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return knowledgeapplication.StoredProjectionContentDTO{}, fmt.Errorf("%w: projection changed during read", knowledgeapplication.ErrProjectionIntegrity)
	}
	content, err := io.ReadAll(io.LimitReader(file, command.MaxBytes+1))
	if err != nil {
		return knowledgeapplication.StoredProjectionContentDTO{}, err
	}
	if int64(len(content)) > command.MaxBytes {
		return knowledgeapplication.StoredProjectionContentDTO{}, knowledgeapplication.ErrProjectionTooLarge
	}
	digest := sha256.Sum256(content)
	actualSHA := hex.EncodeToString(digest[:])
	if int64(len(content)) != receipt.sizeBytes || actualSHA != receipt.sha256 {
		return knowledgeapplication.StoredProjectionContentDTO{}, knowledgeapplication.ErrProjectionIntegrity
	}
	return knowledgeapplication.StoredProjectionContentDTO{
		Content: content, MIMEType: receipt.mimeType, SHA256: actualSHA, SizeBytes: int64(len(content)),
	}, nil
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

func (writer *Writer) openRoot() (*os.Root, error) {
	if err := writer.ensureRoot(); err != nil {
		return nil, err
	}
	return os.OpenRoot(writer.root)
}

func validateProjectionRelativePath(relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) || filepath.ToSlash(filepath.Clean(relativePath)) != relativePath || !strings.HasPrefix(relativePath, "documents/") {
		return "", fmt.Errorf("invalid projection path")
	}
	return filepath.FromSlash(relativePath), nil
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

type projectionManifest struct {
	documentID, documentVersionID int64
	format                        string
	transformerProfileSHA256      string
	relativePath, mimeType        string
	content                       []byte
	sha256                        string
}

type projectionReceiptRecord struct {
	documentID, documentVersionID int64
	format                        string
	transformerProfileSHA256      string
	relativePath, mimeType        string
	sha256                        string
	sizeBytes                     int64
}

func projectionManifestFromStoreCommand(command knowledgeapplication.StoreProjectionCommand) (projectionManifest, error) {
	if err := knowledgeapplication.ValidateStoreProjectionCommand(command); err != nil {
		return projectionManifest{}, err
	}
	return projectionManifest{
		documentID: command.DocumentID, documentVersionID: command.DocumentVersionID,
		format: command.Format, transformerProfileSHA256: command.TransformerProfileSHA256,
		relativePath: command.RelativePath, mimeType: command.MIMEType,
		content: append([]byte(nil), command.Content...), sha256: command.SHA256,
	}, nil
}

func projectionReceiptRecordFromDTO(receipt knowledgeapplication.ProjectionStoreReceiptDTO) (projectionReceiptRecord, error) {
	if err := knowledgeapplication.ValidateProjectionStoreReceiptDTO(receipt); err != nil {
		return projectionReceiptRecord{}, err
	}
	return projectionReceiptRecord{
		documentID: receipt.DocumentID, documentVersionID: receipt.DocumentVersionID,
		format: receipt.Format, transformerProfileSHA256: receipt.TransformerProfileSHA256,
		relativePath: receipt.RelativePath, mimeType: receipt.MIMEType,
		sha256: receipt.SHA256, sizeBytes: receipt.SizeBytes,
	}, nil
}

func projectionReceipt(projection projectionManifest) knowledgeapplication.ProjectionStoreReceiptDTO {
	return knowledgeapplication.ProjectionStoreReceiptDTO{
		DocumentID: projection.documentID, DocumentVersionID: projection.documentVersionID,
		Format: projection.format, TransformerProfileSHA256: projection.transformerProfileSHA256,
		RelativePath: projection.relativePath, MIMEType: projection.mimeType, SHA256: projection.sha256,
		SizeBytes: int64(len(projection.content)),
	}
}

func projectionReceiptAt(root *os.Root, relativePath string, projection projectionManifest) (knowledgeapplication.ProjectionStoreReceiptDTO, bool, error) {
	if err := rejectRootSymlinkComponents(root, filepath.Dir(relativePath)); err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, err
	}
	info, err := root.Lstat(relativePath)
	if os.IsNotExist(err) {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, nil
	}
	if err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, fmt.Errorf("%w: projection is not a regular file", knowledgeapplication.ErrProjectionIntegrity)
	}
	if info.Size() != int64(len(projection.content)) {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, knowledgeapplication.ErrProjectionConflict
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, fmt.Errorf("%w: projection changed during read", knowledgeapplication.ErrProjectionIntegrity)
	}
	content, err := io.ReadAll(io.LimitReader(file, info.Size()+1))
	if err != nil {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, err
	}
	digest := sha256.Sum256(content)
	if int64(len(content)) != int64(len(projection.content)) || hex.EncodeToString(digest[:]) != projection.sha256 {
		return knowledgeapplication.ProjectionStoreReceiptDTO{}, false, knowledgeapplication.ErrProjectionConflict
	}
	return projectionReceipt(projection), true, nil
}

func syncRootDirectory(root *os.Root, directory string) error {
	handle, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func rejectRootSymlinkComponents(root *os.Root, target string) error {
	current := "."
	for _, component := range strings.Split(filepath.Clean(target), string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("vault path contains symlink")
		}
		if !info.IsDir() {
			return fmt.Errorf("vault path component is not a directory")
		}
	}
	return nil
}

var _ knowledgeapplication.ProjectionStore = (*Writer)(nil)
