package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
)

func TestWriterUsesAtomicStablePath(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root)
	path, err := writer.Write("events", "evt-1", "# event")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "# event" {
		t.Fatalf("written content = %q/%v", contents, err)
	}
	if _, err := writer.Write("events", "../escape", "bad"); err == nil || !strings.Contains(err.Error(), "vault path") {
		t.Fatalf("traversal error = %v", err)
	}
}

func TestWriterAutomaticUpdateAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root)
	if _, err := writer.Write("events", "evt-1", "Human"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteAutomatic("events", "evt-1", "Generated v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteAutomatic("events", "evt-1", "Generated v2"); err != nil {
		t.Fatal(err)
	}
	content, _, err := writer.Read("events", "evt-1")
	if err != nil || !strings.Contains(string(content), "Human") || !strings.Contains(string(content), "Generated v2") || strings.Contains(string(content), "Generated v1") {
		t.Fatalf("automatic update = %q/%v", content, err)
	}
	escape := filepath.Join(root, "escape")
	if err := os.MkdirAll(escape, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "topics")
	if err := os.Symlink(escape, link); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write("topics", "escaped", "bad"); !errors.Is(err, domain.ErrVaultPathSymlink) || domain.VaultRejectionReason(err) != domain.VaultReasonPathSymlink || strings.Contains(err.Error(), root) {
		t.Fatalf("symlink escape error = %v", err)
	}
}

func TestWriterRejectsPathAndContentBeforeCreatingVaultRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "not-created", "vault")
	writer := NewWriter(root)
	if _, err := writer.Write("events", "%2e%2e", "safe"); !errors.Is(err, domain.ErrVaultPathInvalid) {
		t.Fatalf("encoded traversal error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "not-created")); !os.IsNotExist(err) {
		t.Fatalf("invalid path performed file I/O: %v", err)
	}
	if _, err := writer.CompareAndSwap("reports", "17", domain.HashContent("", ""), `<script>alert(1)</script>`); !errors.Is(err, domain.ErrVaultContentUnsafe) {
		t.Fatalf("unsafe content error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "not-created")); !os.IsNotExist(err) {
		t.Fatalf("unsafe content performed file I/O: %v", err)
	}
}

func TestWriterAutomaticRejectsUnsafeExistingHumanContentWithoutPublishing(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "events", "evt-1.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	unsafe := "Human\n\n<img src=x onerror=alert(1)>\n"
	if err := os.WriteFile(path, []byte(unsafe), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(root)
	if _, err := writer.WriteAutomatic("events", "evt-1", "Generated"); !errors.Is(err, domain.ErrVaultContentUnsafe) {
		t.Fatalf("unsafe existing content error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != unsafe {
		t.Fatalf("Vault content after rejected automatic update = %q/%v", content, err)
	}
}

func TestWriterCompareAndSwapRejectsStalePublisherWithoutChangingVault(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root)
	initial, err := domain.RenderVaultDocument(domain.VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 1, Type: domain.DocumentReport, SourceID: 91,
		Title: "日报 v1", Generated: "generated v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write("reports", "17", initial); err != nil {
		t.Fatal(err)
	}
	replacement, err := domain.UpdateVaultDocument(initial, domain.VaultDocumentRenderInput{
		DocumentID: 17, RevisionNo: 2, Type: domain.DocumentReport, SourceID: 91,
		Title: "日报 v2", Generated: "generated v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	initialHash := domain.HashContent("", initial)
	replacementHash, err := writer.CompareAndSwap("reports", "17", initialHash, replacement)
	if err != nil || replacementHash != domain.HashContent("", replacement) {
		t.Fatalf("CompareAndSwap() = %q/%v", replacementHash, err)
	}

	staleReplacement := strings.ReplaceAll(replacement, "generated v2", "stale overwrite")
	if _, err := writer.CompareAndSwap("reports", "17", initialHash, staleReplacement); !errors.Is(err, domain.ErrVaultConflict) {
		t.Fatalf("stale CompareAndSwap() error = %v", err)
	}
	content, _, err := writer.Read("reports", "17")
	if err != nil || string(content) != replacement {
		t.Fatalf("Vault content after stale writer = %q/%v", content, err)
	}
	removed, err := writer.CleanupTemporary()
	if err != nil || removed != 0 {
		t.Fatalf("temporary cleanup = %d/%v", removed, err)
	}
}

func TestWriterCompareAndSwapKeepsOriginalOnAtomicWriteFailure(t *testing.T) {
	tests := []struct {
		name   string
		inject func(*Writer)
		want   error
	}{
		{
			name: "disk full while writing temporary file",
			inject: func(writer *Writer) {
				writer.writeString = func(*os.File, string) (int, error) { return 0, syscall.ENOSPC }
			},
			want: syscall.ENOSPC,
		},
		{
			name: "rename interrupted before publish",
			inject: func(writer *Writer) {
				writer.renameFile = func(string, string) error { return syscall.EIO }
			},
			want: syscall.EIO,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writer := NewWriter(root)
			initial := "original Vault bytes"
			if _, err := writer.Write("reports", "17", initial); err != nil {
				t.Fatal(err)
			}
			test.inject(writer)

			if _, err := writer.CompareAndSwap("reports", "17", domain.HashContent("", initial), "replacement"); !errors.Is(err, test.want) {
				t.Fatalf("CompareAndSwap() error = %v, want %v", err, test.want)
			}
			content, _, err := writer.Read("reports", "17")
			if err != nil || string(content) != initial {
				t.Fatalf("Vault content after failed publish = %q/%v", content, err)
			}
			matches, err := filepath.Glob(filepath.Join(root, "reports", ".hotkey-*.tmp"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("temporary files after failed publish = %v/%v", matches, err)
			}
		})
	}
}
