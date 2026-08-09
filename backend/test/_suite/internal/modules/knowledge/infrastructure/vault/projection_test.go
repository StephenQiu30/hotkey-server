package vault

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
)

func TestProjectionWriterIsImmutableAndIdempotent(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root)
	content := []byte("# Archived\n\nBody\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	profile := strings.Repeat("a", 64)
	projection := projectionStoreCommand(5, 8, "markdown", profile, content)

	first, err := writer.PutIfAbsent(context.Background(), projection)
	if err != nil {
		t.Fatalf("PutIfAbsent(): %v", err)
	}
	second, err := writer.PutIfAbsent(context.Background(), projection)
	if err != nil || second != first {
		t.Fatalf("idempotent PutIfAbsent() = %#v, %v; want %#v", second, err, first)
	}
	if first.RelativePath != "documents/5/8/markdown/"+profile+".md" || strings.HasPrefix(first.RelativePath, root) {
		t.Fatalf("receipt path = %q; want opaque relative path", first.RelativePath)
	}

	read, err := writer.ReadProjection(context.Background(), knowledgeapplication.ReadStoredProjectionCommand{Receipt: first, MaxBytes: 1 << 20})
	if err != nil || string(read.Content) != string(content) || read.SHA256 != digest {
		t.Fatalf("Read() = %#v, %v", read, err)
	}

	changed := []byte("changed")
	projection.Content = changed
	projection.SHA256 = fmt.Sprintf("%x", sha256.Sum256(changed))
	if _, err := writer.PutIfAbsent(context.Background(), projection); !errors.Is(err, knowledgeapplication.ErrProjectionConflict) {
		t.Fatalf("conflicting PutIfAbsent() error = %v", err)
	}
	if _, err := writer.Write("documents/5/8/markdown", profile, "legacy overwrite"); err == nil {
		t.Fatal("legacy mutable writer entered the immutable documents namespace")
	}
}

func TestProjectionWriterRejectsSymlinksAndIntegrityMismatch(t *testing.T) {
	root := t.TempDir()
	escape := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escape, filepath.Join(root, "documents", "9")); err != nil {
		t.Fatal(err)
	}
	writer := NewWriter(root)
	content := []byte("body")
	projection := projectionStoreCommand(9, 1, "plaintext", strings.Repeat("b", 64), content)
	if _, err := writer.PutIfAbsent(context.Background(), projection); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink error = %v", err)
	}

	cleanWriter := NewWriter(t.TempDir())
	receipt, err := cleanWriter.PutIfAbsent(context.Background(), projectionStoreCommand(2, 3, "plaintext", strings.Repeat("c", 64), content))
	if err != nil {
		t.Fatal(err)
	}
	receipt.SHA256 = strings.Repeat("0", 64)
	if _, err := cleanWriter.ReadProjection(context.Background(), knowledgeapplication.ReadStoredProjectionCommand{Receipt: receipt, MaxBytes: 1 << 20}); !errors.Is(err, knowledgeapplication.ErrProjectionIntegrity) {
		t.Fatalf("integrity error = %v", err)
	}
}

func TestProjectionWriterHonorsContextAndReadLimit(t *testing.T) {
	writer := NewWriter(t.TempDir())
	content := []byte("body")
	projection := projectionStoreCommand(1, 1, "plaintext", strings.Repeat("d", 64), content)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writer.PutIfAbsent(cancelled, projection); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled PutIfAbsent() error = %v", err)
	}
	receipt, err := writer.PutIfAbsent(context.Background(), projection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ReadProjection(context.Background(), knowledgeapplication.ReadStoredProjectionCommand{Receipt: receipt, MaxBytes: 2}); !errors.Is(err, knowledgeapplication.ErrProjectionTooLarge) {
		t.Fatalf("bounded Read() error = %v", err)
	}
}

func projectionStoreCommand(documentID, documentVersionID int64, format, profile string, content []byte) knowledgeapplication.StoreProjectionCommand {
	extension := "txt"
	mimeType := "text/plain; charset=utf-8"
	if format == "markdown" {
		extension = "md"
		mimeType = "text/markdown; charset=utf-8"
	}
	return knowledgeapplication.StoreProjectionCommand{
		DocumentID: documentID, DocumentVersionID: documentVersionID, Format: format,
		TransformerProfileSHA256: profile,
		RelativePath:             fmt.Sprintf("documents/%d/%d/%s/%s.%s", documentID, documentVersionID, format, profile, extension),
		MIMEType:                 mimeType, Content: append([]byte(nil), content...), SHA256: fmt.Sprintf("%x", sha256.Sum256(content)),
	}
}
