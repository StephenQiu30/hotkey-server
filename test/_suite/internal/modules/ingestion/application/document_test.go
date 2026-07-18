package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

const testMarkdownMIME = "text/markdown; charset=utf-8"

func TestContentDocumentSelectsNewestAvailableMarkdownAsset(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	content := queryTestContent(7, 3)
	readyMarkdown := "# 标题\n"
	readySHA := fmt.Sprintf("%x", sha256.Sum256([]byte(readyMarkdown)))
	assets := []ingestiondomain.ContentAsset{
		{ID: 8, ContentID: 7, AssetType: "text", ObjectKey: "evidence/v1/3/aa/plain.txt", MIMEType: "text/plain; charset=utf-8", SHA256: strings.Repeat("a", 64), SizeBytes: 5, CapturedAt: now.Add(time.Minute), Status: ingestiondomain.AssetStatusAvailable},
		{ID: 9, ContentID: 7, AssetType: "text", ObjectKey: "evidence/v1/3/bb/pending.txt", MIMEType: testMarkdownMIME, SHA256: strings.Repeat("b", 64), SizeBytes: 7, CapturedAt: now.Add(2 * time.Minute), Status: ingestiondomain.AssetStatusDeletePending},
		{ID: 10, ContentID: 7, AssetType: "text", ObjectKey: "evidence/v1/3/cc/older.txt", MIMEType: testMarkdownMIME, SHA256: strings.Repeat("c", 64), SizeBytes: 7, CapturedAt: now, Status: ingestiondomain.AssetStatusAvailable},
		{ID: 11, ContentID: 7, AssetType: "text", ObjectKey: "evidence/v1/3/dd/newer.txt", MIMEType: testMarkdownMIME, SHA256: readySHA, SizeBytes: int64(len(readyMarkdown)), CapturedAt: now, Status: ingestiondomain.AssetStatusAvailable},
	}
	store := &documentEvidenceStoreStub{documents: map[string]ingestiondomain.EvidenceText{
		assets[3].ObjectKey: {Text: readyMarkdown, MIMEType: testMarkdownMIME, SHA256: assets[3].SHA256, SizeBytes: assets[3].SizeBytes},
	}}
	service, err := NewContentQueryService(ContentQueryDependencies{
		Contents: &contentQueryRepositoryStub{content: content, assets: assets},
		Sources: &contentSourceReaderStub{references: map[int64]sourcedomain.ContentSourceReference{
			3: {Name: "RSS feed", SourceType: sourcedomain.SourceTypeRSS},
		}},
		Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewContentQueryService() error = %v", err)
	}

	document, err := service.GetDocument(context.Background(), content.ID)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if document.Availability != ingestiondomain.ContentDocumentReady || document.Markdown != "# 标题\n" || document.SourceName != "RSS feed" || document.SHA256 != assets[3].SHA256 {
		t.Fatalf("GetDocument() = %#v, want newest ready Markdown", document)
	}
	if store.lastKey != assets[3].ObjectKey || store.lastMaxBytes <= 0 {
		t.Fatalf("EvidenceStore.ReadText() args = %q/%d, want selected key and positive limit", store.lastKey, store.lastMaxBytes)
	}
}

func TestContentDocumentReturnsNotCapturedForLegacyOrMissingAsset(t *testing.T) {
	t.Parallel()
	content := queryTestContent(7, 3)
	service, err := NewContentQueryService(ContentQueryDependencies{
		Contents: &contentQueryRepositoryStub{content: content, assets: []ingestiondomain.ContentAsset{{
			ID: 8, ContentID: 7, AssetType: "text", ObjectKey: "evidence/v1/3/aa/plain.txt", MIMEType: "text/plain; charset=utf-8", Status: ingestiondomain.AssetStatusAvailable,
		}}},
		Sources:  &contentSourceReaderStub{references: map[int64]sourcedomain.ContentSourceReference{3: {Name: "RSS feed", SourceType: sourcedomain.SourceTypeRSS}}},
		Evidence: &documentEvidenceStoreStub{},
	})
	if err != nil {
		t.Fatalf("NewContentQueryService() error = %v", err)
	}
	document, err := service.GetDocument(context.Background(), content.ID)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if document.Availability != ingestiondomain.ContentDocumentNotCaptured || document.Markdown != "" || document.SHA256 != "" {
		t.Fatalf("GetDocument() = %#v, want not_captured empty state", document)
	}
}

func TestContentDocumentRejectsStoreAndIntegrityFailuresAsUnavailable(t *testing.T) {
	t.Parallel()
	content := queryTestContent(7, 3)
	asset := ingestiondomain.ContentAsset{ID: 8, ContentID: 7, AssetType: "text", ObjectKey: "evidence/v1/3/aa/markdown.txt", MIMEType: testMarkdownMIME, SHA256: strings.Repeat("a", 64), SizeBytes: 5, CapturedAt: time.Now().UTC(), Status: ingestiondomain.AssetStatusAvailable}
	tests := []struct {
		name     string
		document ingestiondomain.EvidenceText
		err      error
	}{
		{name: "store unavailable", err: errors.New("minio.internal private diagnostic")},
		{name: "sha mismatch", document: ingestiondomain.EvidenceText{Text: "body", MIMEType: testMarkdownMIME, SHA256: strings.Repeat("b", 64), SizeBytes: 5}},
		{name: "size mismatch", document: ingestiondomain.EvidenceText{Text: "body", MIMEType: testMarkdownMIME, SHA256: asset.SHA256, SizeBytes: 4}},
		{name: "mime mismatch", document: ingestiondomain.EvidenceText{Text: "body", MIMEType: "text/plain", SHA256: asset.SHA256, SizeBytes: 5}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewContentQueryService(ContentQueryDependencies{
				Contents: &contentQueryRepositoryStub{content: content, assets: []ingestiondomain.ContentAsset{asset}},
				Sources:  &contentSourceReaderStub{references: map[int64]sourcedomain.ContentSourceReference{3: {Name: "RSS feed", SourceType: sourcedomain.SourceTypeRSS}}},
				Evidence: &documentEvidenceStoreStub{document: test.document, err: test.err},
			})
			if err != nil {
				t.Fatalf("NewContentQueryService() error = %v", err)
			}
			if _, err := service.GetDocument(context.Background(), content.ID); !isAppCode(err, sharederrors.CodeUnavailable) {
				t.Fatalf("GetDocument() error = %v, want unavailable", err)
			}
		})
	}
}

type documentEvidenceStoreStub struct {
	documents    map[string]ingestiondomain.EvidenceText
	document     ingestiondomain.EvidenceText
	err          error
	lastKey      string
	lastMaxBytes int64
}

func (*documentEvidenceStoreStub) PutText(context.Context, ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	return ingestiondomain.EvidenceReceipt{}, errors.New("not used")
}
func (*documentEvidenceStoreStub) Delete(context.Context, string) error {
	return errors.New("not used")
}
func (*documentEvidenceStoreStub) ListPrefix(context.Context, string) ([]ingestiondomain.EvidenceReceipt, error) {
	return nil, errors.New("not used")
}
func (store *documentEvidenceStoreStub) ReadText(_ context.Context, key string, maxBytes int64) (ingestiondomain.EvidenceText, error) {
	store.lastKey, store.lastMaxBytes = key, maxBytes
	if store.err != nil {
		return ingestiondomain.EvidenceText{}, store.err
	}
	if document, ok := store.documents[key]; ok {
		return document, nil
	}
	return store.document, nil
}
