package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestProjectionServiceReadsDocumentProjectionThroughDeterministicReceipt(t *testing.T) {
	t.Parallel()
	content := []byte("# verified markdown\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	profile := strings.Repeat("b", 64)
	store := &documentProjectionReadStoreStub{content: StoredProjectionContentDTO{
		Content: content, MIMEType: "text/markdown; charset=utf-8", SHA256: digest, SizeBytes: int64(len(content)),
	}}
	service, err := NewProjectionService(store)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.ReadDocumentProjection(context.Background(), DocumentProjectionQueryDTO{
		DocumentID: 11, DocumentVersionID: 41, ArtifactType: "markdown",
		TransformerProfileSHA256: profile, SHA256: digest, SizeBytes: int64(len(content)), MaxBytes: 4 << 20,
	})
	if err != nil {
		t.Fatalf("ReadDocumentProjection() error = %v", err)
	}
	if result.Content != string(content) || result.SHA256 != digest || result.SizeBytes != int64(len(content)) {
		t.Fatalf("ReadDocumentProjection() = %#v", result)
	}
	wantPath := "documents/11/41/markdown/" + profile + ".md"
	if store.receipt.RelativePath != wantPath || store.receipt.DocumentVersionID != 41 || store.maxBytes != 4<<20 {
		t.Fatalf("store receipt/max = %#v/%d, want deterministic %s", store.receipt, store.maxBytes, wantPath)
	}
}

func TestProjectionServiceRejectsInvalidOrInconsistentDocumentProjection(t *testing.T) {
	t.Parallel()
	content := []byte("# verified markdown\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	profile := strings.Repeat("b", 64)
	valid := DocumentProjectionQueryDTO{
		DocumentID: 11, DocumentVersionID: 41, ArtifactType: "markdown",
		TransformerProfileSHA256: profile, SHA256: digest, SizeBytes: int64(len(content)), MaxBytes: 4 << 20,
	}

	store := &documentProjectionReadStoreStub{content: StoredProjectionContentDTO{
		Content: content, MIMEType: "text/markdown; charset=utf-8", SHA256: digest, SizeBytes: int64(len(content)),
	}}
	service, err := NewProjectionService(store)
	if err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.DocumentVersionID = 0
	if _, err := service.ReadDocumentProjection(context.Background(), invalid); err == nil || store.calls != 0 {
		t.Fatalf("invalid query error/calls = %v/%d, want validation before store", err, store.calls)
	}

	store.content.SHA256 = strings.Repeat("c", 64)
	if _, err := service.ReadDocumentProjection(context.Background(), valid); err == nil {
		t.Fatal("ReadDocumentProjection() accepted a store result with a different SHA-256")
	}
	store.content = StoredProjectionContentDTO{
		Content: []byte{0xff, 0xfe}, MIMEType: "text/markdown; charset=utf-8", SHA256: digest, SizeBytes: 2,
	}
	if _, err := service.ReadDocumentProjection(context.Background(), valid); err == nil {
		t.Fatal("ReadDocumentProjection() accepted non-UTF-8 content")
	}
	store.err = fs.ErrNotExist
	if _, err := service.ReadDocumentProjection(context.Background(), valid); !errors.Is(err, ErrProjectionNotFound) {
		t.Fatalf("missing projection error = %v, want ErrProjectionNotFound", err)
	}
}

type documentProjectionReadStoreStub struct {
	projection StoreProjectionCommand
	receipt    ProjectionStoreReceiptDTO
	content    StoredProjectionContentDTO
	err        error
	maxBytes   int64
	calls      int
}

func (store *documentProjectionReadStoreStub) PutIfAbsent(_ context.Context, projection StoreProjectionCommand) (ProjectionStoreReceiptDTO, error) {
	store.projection = projection
	return ProjectionStoreReceiptDTO{}, nil
}

func (store *documentProjectionReadStoreStub) ReadProjection(_ context.Context, command ReadStoredProjectionCommand) (StoredProjectionContentDTO, error) {
	store.calls++
	store.receipt = command.Receipt
	store.maxBytes = command.MaxBytes
	return store.content, store.err
}
