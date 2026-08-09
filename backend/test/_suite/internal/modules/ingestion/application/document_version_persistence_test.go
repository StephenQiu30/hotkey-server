package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestDocumentVersionServicePersistsNormalizedMetadataWithoutPassingBodyToPostgres(t *testing.T) {
	t.Parallel()
	capturedAt := time.Date(2026, time.August, 9, 8, 30, 0, 0, time.UTC)
	reader := &documentObservationReaderStub{observation: DocumentObservationDTO{
		ID: 41, SourceConnectionID: 7, ExternalWorkID: "  Cafe\u0301-article  ",
		BodyOrigin: BodyOriginFeedContent, Completeness: BodyCompletenessFull,
		Body: "  first line\r\nsecond line  ", Language: "EN", CapturedAt: capturedAt,
	}}
	store := &documentVersionStoreStub{documentID: 19}
	qualityScore := 87.5
	service, err := NewDocumentVersionService(DocumentVersionDependencies{Observations: reader, Versions: store})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}

	result, err := service.PersistSourceObservation(context.Background(), PersistDocumentVersionCommand{
		SourceObservationID: 41,
		ExtractorVersion:    " rss-entry-v2 ", ExtractorProfileVersion: " rss-profile-v3 ",
		ExtractorProfileSHA256: strings.Repeat("A", 64), QualityScore: &qualityScore,
		Truncated: true, QualityWarnings: []string{" Missing author ", "missing author", "SHORT_BODY"},
	})
	if err != nil {
		t.Fatalf("PersistSourceObservation() error = %v", err)
	}
	if !result.DocumentCreated || !result.DocumentVersionCreated || result.Document.ID != 19 || result.DocumentVersion.ID != 29 {
		t.Fatalf("PersistSourceObservation() result = %#v, want newly persisted document/version", result)
	}
	if reader.lastID != 41 {
		t.Fatalf("ReadDocumentObservation() id = %d, want 41", reader.lastID)
	}
	if store.documentIdentity.ExternalWorkID == nil || *store.documentIdentity.ExternalWorkID != "Café-article" || len(store.documentIdentity.DocumentKey) != 64 {
		t.Fatalf("document identity = %#v, want normalized external-work identity", store.documentIdentity)
	}
	draft := store.appended
	wantBody := "first line\nsecond line"
	wantDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(wantBody)))
	if draft.DocumentID != 19 || draft.SourceObservationID != 41 || draft.BodyOrigin != BodyOriginFeedContent ||
		draft.Completeness != BodyCompletenessFull || draft.WordCount != 4 || draft.Language != "en" ||
		draft.ContentSHA256 != wantDigest || draft.ExtractorVersion != "rss-entry-v2" ||
		draft.ExtractorProfileVersion != "rss-profile-v3" || draft.ExtractorProfileSHA256 != strings.Repeat("a", 64) ||
		draft.QualityScore == nil || *draft.QualityScore != 87.5 || !draft.Truncated || !reflect.DeepEqual(draft.QualityWarnings, []string{"missing author", "short_body"}) ||
		draft.LifecycleState != DocumentPolicyPending || !draft.CapturedAt.Equal(capturedAt) {
		t.Fatalf("AppendDocumentVersion() input = %#v, want normalized immutable metadata", draft)
	}
	if _, exposed := reflect.TypeOf(draft).FieldByName("Body"); exposed {
		t.Fatal("DocumentVersionDraftDTO exposes Body; the persistence port must never receive document body bytes")
	}
}

func TestDocumentVersionServicePersistsAlreadyVerifiedObservationWithoutAnotherSourceRead(t *testing.T) {
	t.Parallel()

	reader := &documentObservationReaderStub{err: errors.New("unexpected source read")}
	store := &documentVersionStoreStub{documentID: 19}
	service, err := NewDocumentVersionService(DocumentVersionDependencies{Observations: reader, Versions: store})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}
	command := PersistDocumentObservationCommand{
		Observation: DocumentObservationDTO{
			ID: 41, SourceConnectionID: 7, ExternalWorkID: "article-41",
			BodyOrigin: BodyOriginFeedContent, Completeness: BodyCompletenessFull,
			Body: "verified body", Language: "en", CapturedAt: time.Date(2026, time.August, 9, 8, 30, 0, 0, time.UTC),
		},
		ExtractorVersion: "feed-body-extractor-v1", ExtractorProfileVersion: "rss-atom-rdf-body-v1",
		ExtractorProfileSHA256: strings.Repeat("a", 64), QualityWarnings: []string{"captured_markup_sanitized"},
	}

	result, err := service.PersistDocumentObservation(context.Background(), command)
	if err != nil {
		t.Fatalf("PersistDocumentObservation() error = %v", err)
	}
	if reader.lastID != 0 {
		t.Fatalf("PersistDocumentObservation() reread Source observation %d", reader.lastID)
	}
	if !result.DocumentVersionCreated || store.appended.SourceObservationID != 41 ||
		store.appended.ContentSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte("verified body"))) {
		t.Fatalf("PersistDocumentObservation() result/draft = %#v/%#v", result, store.appended)
	}
}

func TestDocumentObservationPersistenceConstructorRequiresOnlyRepository(t *testing.T) {
	t.Parallel()

	store := &documentVersionStoreStub{documentID: 19}
	service, err := NewDocumentObservationPersistenceService(store)
	if err != nil {
		t.Fatalf("NewDocumentObservationPersistenceService() error = %v", err)
	}
	command := PersistDocumentObservationCommand{
		Observation: DocumentObservationDTO{
			ID: 41, SourceConnectionID: 7, ExternalWorkID: "article-41",
			BodyOrigin: BodyOriginFeedContent, Completeness: BodyCompletenessFull,
			Body: "verified body", Language: "en", CapturedAt: time.Date(2026, time.August, 9, 8, 30, 0, 0, time.UTC),
		},
		ExtractorVersion: "feed-body-extractor-v1", ExtractorProfileVersion: "rss-atom-rdf-body-v1",
		ExtractorProfileSHA256: strings.Repeat("a", 64),
	}
	if _, err := service.PersistDocumentObservation(context.Background(), command); err != nil {
		t.Fatalf("PersistDocumentObservation() error = %v", err)
	}
	if _, err := service.PersistSourceObservation(context.Background(), validPersistDocumentVersionCommand()); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("repository-only PersistSourceObservation() error = %v, want invalid input", err)
	}
	if _, err := NewDocumentObservationPersistenceService(nil); err == nil {
		t.Fatal("NewDocumentObservationPersistenceService(nil) error = nil")
	}
}

func TestDocumentVersionServiceRejectsInvalidBodyBeforeCreatingDocument(t *testing.T) {
	t.Parallel()
	reader := &documentObservationReaderStub{observation: DocumentObservationDTO{
		ID: 41, SourceConnectionID: 7, ExternalWorkID: "article-1",
		BodyOrigin: BodyOriginFeedSummary, Completeness: BodyCompletenessFull,
		Body: "summary", Language: "en", CapturedAt: time.Now().UTC(),
	}}
	store := &documentVersionStoreStub{documentID: 19}
	service, err := NewDocumentVersionService(DocumentVersionDependencies{Observations: reader, Versions: store})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}

	_, err = service.PersistSourceObservation(context.Background(), validPersistDocumentVersionCommand())
	if !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("PersistSourceObservation() error = %v, want invalid input", err)
	}
	if store.resolveCalls != 0 || store.appendCalls != 0 {
		t.Fatalf("persistence calls = resolve %d / append %d, want none", store.resolveCalls, store.appendCalls)
	}
}

func TestDocumentVersionServiceUsesObservationScopedIdentityWhenExternalWorkIsUnknown(t *testing.T) {
	t.Parallel()
	store := &documentVersionStoreStub{documentID: 19}
	reader := &documentObservationReaderStub{observation: DocumentObservationDTO{
		ID: 41, SourceConnectionID: 7,
		BodyOrigin: BodyOriginAPIContent, Completeness: BodyCompletenessFull,
		Body: "body", Language: "und", CapturedAt: time.Now().UTC(),
	}}
	service, err := NewDocumentVersionService(DocumentVersionDependencies{Observations: reader, Versions: store})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}

	if _, err := service.PersistSourceObservation(context.Background(), validPersistDocumentVersionCommand()); err != nil {
		t.Fatalf("PersistSourceObservation() error = %v", err)
	}
	firstKey := store.documentIdentity.DocumentKey
	if store.documentIdentity.ExternalWorkID != nil {
		t.Fatalf("external work id = %#v, want nil for conservative observation identity", store.documentIdentity.ExternalWorkID)
	}
	reader.observation.ID = 42
	store.documentID = 20
	input := validPersistDocumentVersionCommand()
	input.SourceObservationID = 42
	if _, err := service.PersistSourceObservation(context.Background(), input); err != nil {
		t.Fatalf("PersistSourceObservation(second) error = %v", err)
	}
	if store.documentIdentity.DocumentKey == firstKey {
		t.Fatal("unknown external work reused a Document across observations")
	}
}

func TestDocumentVersionLifecycleServiceValidatesAndUsesCAS(t *testing.T) {
	t.Parallel()
	store := &documentVersionStoreStub{current: lifecycleDocumentVersionDTO(DocumentPolicyPending, 1)}
	service, err := NewDocumentVersionService(DocumentVersionDependencies{Observations: &documentObservationReaderStub{}, Versions: store})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}

	unchanged, err := service.TransitionDocumentVersion(context.Background(), TransitionDocumentVersionCommand{
		DocumentVersionID: 29, ExpectedVersion: 1, To: DocumentPolicyPending,
	})
	if err != nil || unchanged.DocumentVersion.Version != 1 || store.casCalls != 0 {
		t.Fatalf("same-state transition = %#v/%v CAS calls=%d, want no-op", unchanged, err, store.casCalls)
	}

	transitioned, err := service.TransitionDocumentVersion(context.Background(), TransitionDocumentVersionCommand{
		DocumentVersionID: 29, ExpectedVersion: 1, To: DocumentDerivedPending,
	})
	if err != nil || transitioned.DocumentVersion.Version != 2 || transitioned.DocumentVersion.LifecycleState != DocumentDerivedPending {
		t.Fatalf("valid transition = %#v/%v, want version 2 derive_pending", transitioned, err)
	}
	if store.casID != 29 || store.casExpected != 1 || store.casTo != DocumentDerivedPending || store.casCalls != 1 {
		t.Fatalf("CAS args = %d/%d/%s calls=%d", store.casID, store.casExpected, store.casTo, store.casCalls)
	}

	store.current.Version = 3
	_, err = service.TransitionDocumentVersion(context.Background(), TransitionDocumentVersionCommand{
		DocumentVersionID: 29, ExpectedVersion: 2, To: DocumentDerivedAvailable,
	})
	if !errors.Is(err, sharedrepository.ErrConflict) || store.casCalls != 1 {
		t.Fatalf("stale transition error/CAS calls = %v/%d, want conflict/no repository call", err, store.casCalls)
	}

	store.current.Version = 3
	store.current.LifecycleState = DocumentReadable
	_, err = service.TransitionDocumentVersion(context.Background(), TransitionDocumentVersionCommand{
		DocumentVersionID: 29, ExpectedVersion: 3, To: DocumentDerivedPending,
	})
	if !errors.Is(err, sharedrepository.ErrConflict) || store.casCalls != 1 {
		t.Fatalf("invalid transition error/CAS calls = %v/%d, want conflict/no repository call", err, store.casCalls)
	}

	_, err = service.TransitionDocumentVersion(context.Background(), TransitionDocumentVersionCommand{
		DocumentVersionID: 29, ExpectedVersion: 3, To: DocumentRawAvailable,
	})
	if !errors.Is(err, sharedrepository.ErrInvalidInput) || store.casCalls != 1 {
		t.Fatalf("non-persisted state error/CAS calls = %v/%d, want invalid/no repository call", err, store.casCalls)
	}
}

func TestDocumentVersionLifecycleServiceRequiresDisplayReceiptForReadable(t *testing.T) {
	t.Parallel()
	store := &documentVersionStoreStub{current: lifecycleDocumentVersionDTO(DocumentDerivedAvailable, 3)}
	service, err := NewDocumentVersionService(DocumentVersionDependencies{Observations: &documentObservationReaderStub{}, Versions: store})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}

	_, err = service.TransitionDocumentVersion(context.Background(), TransitionDocumentVersionCommand{
		DocumentVersionID: 29, ExpectedVersion: 3, To: DocumentReadable,
	})
	if !errors.Is(err, sharedrepository.ErrInvalidInput) || store.casCalls != 0 {
		t.Fatalf("readable without display receipt error/CAS = %v/%d, want invalid/no repository call", err, store.casCalls)
	}

	displayDecisionID := int64(77)
	readable, err := service.TransitionDocumentVersion(context.Background(), TransitionDocumentVersionCommand{
		DocumentVersionID: 29, ExpectedVersion: 3, To: DocumentReadable,
		DisplayPrivateRightsDecisionID: &displayDecisionID,
	})
	if err != nil || readable.DocumentVersion.LifecycleState != DocumentReadable || readable.DocumentVersion.DisplayPrivateRightsDecisionID == nil || *readable.DocumentVersion.DisplayPrivateRightsDecisionID != displayDecisionID {
		t.Fatalf("readable with display receipt = %#v/%v", readable, err)
	}
	if store.casDisplayDecisionID == nil || *store.casDisplayDecisionID != displayDecisionID {
		t.Fatalf("CAS display decision = %#v, want %d", store.casDisplayDecisionID, displayDecisionID)
	}
}

func validPersistDocumentVersionCommand() PersistDocumentVersionCommand {
	qualityScore := 90.0
	return PersistDocumentVersionCommand{
		SourceObservationID: 41, ExtractorVersion: "extract-v1", ExtractorProfileVersion: "profile-v1",
		ExtractorProfileSHA256: strings.Repeat("a", 64), QualityScore: &qualityScore,
	}
}

type documentObservationReaderStub struct {
	observation DocumentObservationDTO
	err         error
	lastID      int64
}

func (reader *documentObservationReaderStub) ReadDocumentObservation(_ context.Context, id int64) (DocumentObservationDTO, error) {
	reader.lastID = id
	return reader.observation, reader.err
}

type documentVersionStoreStub struct {
	documentID           int64
	documentIdentity     DocumentIdentityDTO
	appended             DocumentVersionDraftDTO
	current              DocumentVersionDTO
	resolveCalls         int
	appendCalls          int
	casCalls             int
	casID                int64
	casExpected          int64
	casTo                string
	casDisplayDecisionID *int64
	resolveErr           error
	appendErr            error
	getErr               error
	casErr               error
}

func (store *documentVersionStoreStub) ResolveDocument(_ context.Context, identity DocumentIdentityDTO) (DocumentDTO, bool, error) {
	store.resolveCalls++
	store.documentIdentity = identity
	if store.resolveErr != nil {
		return DocumentDTO{}, false, store.resolveErr
	}
	externalWorkID := cloneOptionalString(identity.ExternalWorkID)
	createdAt := time.Date(2026, time.August, 9, 8, 0, 0, 0, time.UTC)
	return DocumentDTO{
		ID: store.documentID, Version: 1, SourceConnectionID: identity.SourceConnectionID,
		DocumentKey: identity.DocumentKey, ExternalWorkID: externalWorkID, State: DocumentStateActive,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}, true, nil
}

func (store *documentVersionStoreStub) AppendDocumentVersion(_ context.Context, draft DocumentVersionDraftDTO) (DocumentVersionDTO, bool, error) {
	store.appendCalls++
	store.appended = draft
	if store.appendErr != nil {
		return DocumentVersionDTO{}, false, store.appendErr
	}
	stored := documentVersionFromDraft(29, 1, 1, draft)
	store.current = stored
	return stored, true, nil
}

func (store *documentVersionStoreStub) GetDocumentVersion(_ context.Context, id int64) (DocumentVersionDTO, error) {
	if store.getErr != nil {
		return DocumentVersionDTO{}, store.getErr
	}
	if store.current.ID == 0 {
		return DocumentVersionDTO{}, fmt.Errorf("%w: document version %d", sharedrepository.ErrNotFound, id)
	}
	return store.current, nil
}

func (store *documentVersionStoreStub) CompareAndSwapDocumentVersionLifecycle(_ context.Context, command TransitionDocumentVersionCommand) (DocumentVersionDTO, error) {
	store.casCalls++
	store.casID, store.casExpected, store.casTo = command.DocumentVersionID, command.ExpectedVersion, command.To
	store.casDisplayDecisionID = cloneOptionalInt64(command.DisplayPrivateRightsDecisionID)
	if store.casErr != nil {
		return DocumentVersionDTO{}, store.casErr
	}
	store.current.Version = command.ExpectedVersion + 1
	store.current.LifecycleState = command.To
	store.current.DisplayPrivateRightsDecisionID = cloneOptionalInt64(command.DisplayPrivateRightsDecisionID)
	return store.current, nil
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func documentVersionFromDraft(id, version, revisionNo int64, draft DocumentVersionDraftDTO) DocumentVersionDTO {
	createdAt := time.Date(2026, time.August, 9, 8, 0, 0, 0, time.UTC)
	return DocumentVersionDTO{
		ID: id, Version: version, DocumentID: draft.DocumentID, SourceObservationID: draft.SourceObservationID,
		RevisionNo: revisionNo, VersionKey: draft.VersionKey, BodyOrigin: draft.BodyOrigin,
		Completeness: draft.Completeness, WordCount: draft.WordCount, Language: draft.Language,
		Truncated: draft.Truncated, QualityScore: cloneOptionalFloat64(draft.QualityScore),
		QualityWarnings: append([]string(nil), draft.QualityWarnings...), ContentSHA256: draft.ContentSHA256,
		ExtractorVersion: draft.ExtractorVersion, ExtractorProfileVersion: draft.ExtractorProfileVersion,
		ExtractorProfileSHA256: draft.ExtractorProfileSHA256, LifecycleState: draft.LifecycleState,
		CapturedAt: draft.CapturedAt, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func lifecycleDocumentVersionDTO(state string, version int64) DocumentVersionDTO {
	capturedAt := time.Date(2026, time.August, 9, 8, 0, 0, 0, time.UTC)
	return DocumentVersionDTO{
		ID: 29, Version: version, DocumentID: 19, SourceObservationID: 41, RevisionNo: 1,
		VersionKey: strings.Repeat("b", 64), BodyOrigin: BodyOriginFeedContent,
		Completeness: BodyCompletenessFull, WordCount: 1, Language: "en",
		ContentSHA256: strings.Repeat("c", 64), ExtractorVersion: "extract-v1",
		ExtractorProfileVersion: "profile-v1", ExtractorProfileSHA256: strings.Repeat("a", 64),
		LifecycleState: state, CapturedAt: capturedAt, CreatedAt: capturedAt, UpdatedAt: capturedAt,
	}
}

func cloneOptionalFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneOptionalInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
