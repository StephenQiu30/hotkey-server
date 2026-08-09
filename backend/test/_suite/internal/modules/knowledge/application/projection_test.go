package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type projectionStoreStub struct {
	put      ProjectionStoreReceiptDTO
	read     StoredProjectionContentDTO
	lastPut  StoreProjectionCommand
	lastRead ReadStoredProjectionCommand
	err      error
}

func (store *projectionStoreStub) PutIfAbsent(_ context.Context, projection StoreProjectionCommand) (ProjectionStoreReceiptDTO, error) {
	store.lastPut = projection
	return store.put, store.err
}

func (store *projectionStoreStub) ReadProjection(_ context.Context, command ReadStoredProjectionCommand) (StoredProjectionContentDTO, error) {
	store.lastRead = command
	return store.read, store.err
}

func TestProjectionServiceValidatesBeforeCallingStore(t *testing.T) {
	store := &projectionStoreStub{}
	service, err := NewProjectionService(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Publish(context.Background(), PublishProjectionCommand{}); err == nil {
		t.Fatal("Publish() accepted an invalid projection")
	}

	content := []byte("document")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	profile := fmt.Sprintf("%064d", 1)
	store.put = ProjectionStoreReceiptDTO{
		DocumentID: 1, DocumentVersionID: 2, Format: "plaintext",
		TransformerProfileSHA256: profile, RelativePath: "documents/1/2/plaintext/" + profile + ".txt",
		MIMEType: "text/plain; charset=utf-8", SHA256: digest, SizeBytes: int64(len(content)),
	}
	got, err := service.Publish(context.Background(), PublishProjectionCommand{
		DocumentID: 1, DocumentVersionID: 2, Format: ProjectionFormatPlaintext,
		TransformerProfileSHA256: profile, Content: content, SHA256: digest,
	})
	if err != nil || got.DocumentID != store.put.DocumentID || got.RelativePath != store.put.RelativePath || got.SHA256 != digest || got.SizeBytes != int64(len(content)) {
		t.Fatalf("Publish() = %#v, %v", got, err)
	}
	if store.lastPut.Format != "plaintext" || store.lastPut.RelativePath != store.put.RelativePath || string(store.lastPut.Content) != string(content) {
		t.Fatalf("Publish() store mapping = %#v", store.lastPut)
	}

	store.put.RelativePath = "documents/1/2/plaintext/" + fmt.Sprintf("%064d", 9) + ".txt"
	if _, err := service.Publish(context.Background(), PublishProjectionCommand{
		DocumentID: 1, DocumentVersionID: 2, Format: ProjectionFormatPlaintext,
		TransformerProfileSHA256: profile, Content: content, SHA256: digest,
	}); !errors.Is(err, ErrProjectionIntegrity) {
		t.Fatalf("Publish() mismatched receipt error = %v, want integrity failure", err)
	}
	store.put.RelativePath = "documents/1/2/plaintext/" + profile + ".txt"
	store.err = errors.New("private projection body and /absolute/vault/path")
	_, err = service.Publish(context.Background(), PublishProjectionCommand{
		DocumentID: 1, DocumentVersionID: 2, Format: ProjectionFormatPlaintext,
		TransformerProfileSHA256: profile, Content: content, SHA256: digest,
	})
	if !errors.Is(err, ErrProjectionUnavailable) || strings.Contains(err.Error(), "private projection body") || strings.Contains(err.Error(), "/absolute/vault/path") {
		t.Fatalf("Publish() unsanitized store error = %v", err)
	}
}

func TestProjectionServiceBoundsReads(t *testing.T) {
	service, err := NewProjectionService(&projectionStoreStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Read(context.Background(), ReadProjectionQuery{MaxBytes: 1024}); err == nil {
		t.Fatal("Read() accepted an invalid receipt")
	}
	readContent := []byte("# archived")
	readDigest := fmt.Sprintf("%x", sha256.Sum256(readContent))
	valid := ReadProjectionQuery{
		DocumentID: 1, DocumentVersionID: 2, Format: ProjectionFormatMarkdown,
		TransformerProfileSHA256: fmt.Sprintf("%064d", 1),
		RelativePath:             "documents/1/2/markdown/" + fmt.Sprintf("%064d", 1) + ".md",
		SHA256:                   readDigest, SizeBytes: int64(len(readContent)), MaxBytes: 1024,
	}
	valid.MaxBytes = 0
	if _, err := service.Read(context.Background(), valid); err == nil {
		t.Fatal("Read() accepted a non-positive byte ceiling")
	}
	valid.MaxBytes = 1024
	store := &projectionStoreStub{read: StoredProjectionContentDTO{Content: readContent, MIMEType: "text/markdown; charset=utf-8", SHA256: valid.SHA256, SizeBytes: int64(len(readContent))}}
	service, err = NewProjectionService(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Read(context.Background(), valid)
	if err != nil || string(result.Content) != "# archived" || store.lastRead.Receipt.Format != "markdown" || store.lastRead.MaxBytes != 1024 {
		t.Fatalf("Read() = %#v/%v; receipt=%#v", result, err, store.lastRead)
	}
	store.read.Content = []byte("# changed!")
	if _, err := service.Read(context.Background(), valid); !errors.Is(err, ErrProjectionIntegrity) {
		t.Fatalf("Read() mismatched bytes error = %v, want integrity failure", err)
	}
}
