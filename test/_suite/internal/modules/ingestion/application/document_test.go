package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

const testMarkdownMIME = "text/markdown; charset=utf-8"

func TestContentDocumentAssetSelection(t *testing.T) {
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

func TestContentDocumentConcurrentReads(t *testing.T) {
	t.Parallel()
	const readers = 32
	content := queryTestContent(7, 3)
	markdown := "# 并发读取\n"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(markdown)))
	asset := ingestiondomain.ContentAsset{ID: 8, ContentID: content.ID, AssetType: "text", ObjectKey: "evidence/v1/3/concurrent.txt", MIMEType: testMarkdownMIME, SHA256: digest, SizeBytes: int64(len(markdown)), CapturedAt: time.Now().UTC(), Status: ingestiondomain.AssetStatusAvailable}
	store := &concurrentDocumentEvidenceStore{document: ingestiondomain.EvidenceText{Text: markdown, MIMEType: testMarkdownMIME, SHA256: digest, SizeBytes: int64(len(markdown))}}
	service := newDocumentQueryService(t, content, []ingestiondomain.ContentAsset{asset}, store)

	var wait sync.WaitGroup
	errorsCh := make(chan error, readers)
	for range readers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			document, err := service.GetDocument(context.Background(), content.ID)
			if err != nil {
				errorsCh <- err
				return
			}
			if document.Availability != ingestiondomain.ContentDocumentReady || document.Markdown != markdown || document.SHA256 != digest {
				errorsCh <- fmt.Errorf("inconsistent document: %#v", document)
			}
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
	if store.reads != readers {
		t.Fatalf("EvidenceStore.ReadText calls = %d, want %d", store.reads, readers)
	}
}

func TestContentDocumentStoreRecovery(t *testing.T) {
	t.Parallel()
	content := queryTestContent(7, 3)
	markdown := "# 恢复\n"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(markdown)))
	asset := ingestiondomain.ContentAsset{ID: 8, ContentID: content.ID, AssetType: "text", ObjectKey: "evidence/v1/3/recovery.txt", MIMEType: testMarkdownMIME, SHA256: digest, SizeBytes: int64(len(markdown)), CapturedAt: time.Now().UTC(), Status: ingestiondomain.AssetStatusAvailable}
	store := &recoveringDocumentEvidenceStore{document: ingestiondomain.EvidenceText{Text: markdown, MIMEType: testMarkdownMIME, SHA256: digest, SizeBytes: int64(len(markdown))}, remainingFailures: 1}
	service := newDocumentQueryService(t, content, []ingestiondomain.ContentAsset{asset}, store)

	if _, err := service.GetDocument(context.Background(), content.ID); !isAppCode(err, sharederrors.CodeUnavailable) {
		t.Fatalf("GetDocument(first) error = %v, want unavailable", err)
	}
	document, err := service.GetDocument(context.Background(), content.ID)
	if err != nil || document.Availability != ingestiondomain.ContentDocumentReady || document.Markdown != markdown {
		t.Fatalf("GetDocument(recovered) = %#v/%v, want ready", document, err)
	}
}

func TestContentDocumentDeleted(t *testing.T) {
	t.Parallel()
	service, err := NewContentQueryService(ContentQueryDependencies{
		Contents: &contentQueryRepositoryStub{getError: sharedrepository.ErrNotFound},
		Sources:  &contentSourceReaderStub{}, Evidence: &documentEvidenceStoreStub{},
	})
	if err != nil {
		t.Fatalf("NewContentQueryService() error = %v", err)
	}
	if _, err := service.GetDocument(context.Background(), 7); !isAppCode(err, sharederrors.CodeNotFound) {
		t.Fatalf("GetDocument(deleted) error = %v, want not found", err)
	}
}

func TestContentDocumentDeletePending(t *testing.T) {
	t.Parallel()
	content := queryTestContent(7, 3)
	asset := ingestiondomain.ContentAsset{ID: 8, ContentID: content.ID, AssetType: "text", ObjectKey: "evidence/v1/3/pending.txt", MIMEType: testMarkdownMIME, SHA256: strings.Repeat("a", 64), SizeBytes: 5, CapturedAt: time.Now().UTC(), Status: ingestiondomain.AssetStatusDeletePending}
	store := &concurrentDocumentEvidenceStore{}
	service := newDocumentQueryService(t, content, []ingestiondomain.ContentAsset{asset}, store)
	document, err := service.GetDocument(context.Background(), content.ID)
	if err != nil || document.Availability != ingestiondomain.ContentDocumentNotCaptured || store.reads != 0 {
		t.Fatalf("GetDocument(delete_pending) = %#v/%v reads=%d, want not_captured without store read", document, err, store.reads)
	}
}

func newDocumentQueryService(t *testing.T, content ingestiondomain.Content, assets []ingestiondomain.ContentAsset, store ingestiondomain.EvidenceStore) *ContentQueryService {
	t.Helper()
	service, err := NewContentQueryService(ContentQueryDependencies{
		Contents: &contentQueryRepositoryStub{content: content, assets: assets},
		Sources:  &contentSourceReaderStub{references: map[int64]sourcedomain.ContentSourceReference{3: {Name: "RSS feed", SourceType: sourcedomain.SourceTypeRSS}}},
		Evidence: store,
	})
	if err != nil {
		t.Fatalf("NewContentQueryService() error = %v", err)
	}
	return service
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

type concurrentDocumentEvidenceStore struct {
	mu       sync.Mutex
	document ingestiondomain.EvidenceText
	reads    int
}

func (*concurrentDocumentEvidenceStore) PutText(context.Context, ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	return ingestiondomain.EvidenceReceipt{}, errors.New("not used")
}
func (*concurrentDocumentEvidenceStore) Delete(context.Context, string) error {
	return errors.New("not used")
}
func (*concurrentDocumentEvidenceStore) ListPrefix(context.Context, string) ([]ingestiondomain.EvidenceReceipt, error) {
	return nil, errors.New("not used")
}
func (store *concurrentDocumentEvidenceStore) ReadText(context.Context, string, int64) (ingestiondomain.EvidenceText, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.reads++
	return store.document, nil
}

type recoveringDocumentEvidenceStore struct {
	mu                sync.Mutex
	document          ingestiondomain.EvidenceText
	remainingFailures int
}

func (*recoveringDocumentEvidenceStore) PutText(context.Context, ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	return ingestiondomain.EvidenceReceipt{}, errors.New("not used")
}
func (*recoveringDocumentEvidenceStore) Delete(context.Context, string) error {
	return errors.New("not used")
}
func (*recoveringDocumentEvidenceStore) ListPrefix(context.Context, string) ([]ingestiondomain.EvidenceReceipt, error) {
	return nil, errors.New("not used")
}
func (store *recoveringDocumentEvidenceStore) ReadText(context.Context, string, int64) (ingestiondomain.EvidenceText, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.remainingFailures > 0 {
		store.remainingFailures--
		return ingestiondomain.EvidenceText{}, errors.New("temporary object store failure")
	}
	return store.document, nil
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
