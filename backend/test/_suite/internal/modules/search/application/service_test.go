package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
)

type lexicalReaderStub struct {
	calls      int
	visible    *bool
	candidates []searchdomain.Candidate
	createdAt  map[string]time.Time
	queries    []searchdomain.Query
	err        error
}

func (reader *lexicalReaderStub) Search(_ context.Context, query searchdomain.Query) ([]searchdomain.Candidate, error) {
	reader.calls++
	reader.queries = append(reader.queries, query)
	items := make([]searchdomain.Candidate, 0, len(reader.candidates))
	for _, candidate := range reader.candidates {
		key := string(candidate.Type) + ":" + strconv.FormatInt(candidate.ID, 10)
		if createdAt := reader.createdAt[key]; !query.SnapshotAt.IsZero() && createdAt.After(query.SnapshotAt) {
			continue
		}
		if query.After != nil && !searchCandidateFollows(candidate, *query.After, query.Sort) {
			continue
		}
		items = append(items, candidate)
	}
	if query.CandidateLimit > 0 && len(items) > query.CandidateLimit {
		items = items[:query.CandidateLimit]
	}
	return items, reader.err
}

func (reader *lexicalReaderStub) CanDisplay(_ context.Context, _ searchdomain.Query, _ searchdomain.Candidate) (bool, error) {
	if reader.err != nil {
		return false, reader.err
	}
	return reader.visible == nil || *reader.visible, nil
}

type subjectReaderStub struct {
	subject          Subject
	err              error
	calls            int
	scopeVisible     *bool
	candidateVisible *bool
}

func (reader *subjectReaderStub) SearchScopeVisible(context.Context, Subject, searchdomain.Query) (bool, error) {
	return reader.err == nil && (reader.scopeVisible == nil || *reader.scopeVisible), reader.err
}

func (reader *subjectReaderStub) SearchCandidateVisible(context.Context, Subject, searchdomain.Candidate) (bool, error) {
	return reader.err == nil && (reader.candidateVisible == nil || *reader.candidateVisible), reader.err
}

func (reader *subjectReaderStub) CurrentSearchSubject(context.Context, int64) (Subject, error) {
	reader.calls++
	return reader.subject, reader.err
}

var viewerSubject = Subject{UserID: 7, Role: "viewer"}

func searchCandidateFollows(candidate searchdomain.Candidate, after searchdomain.Position, sortOrder string) bool {
	if sortOrder == searchdomain.SortLatest {
		if !candidate.OccurredAt.Equal(after.OccurredAt) {
			return candidate.OccurredAt.Before(after.OccurredAt)
		}
		if candidate.Type != after.Type {
			return candidate.Type > after.Type
		}
		if candidate.Score != after.Score {
			return candidate.Score < after.Score
		}
		return candidate.ID < after.ID
	}
	if candidate.Score != after.Score {
		return candidate.Score < after.Score
	}
	if !candidate.OccurredAt.Equal(after.OccurredAt) {
		return candidate.OccurredAt.Before(after.OccurredAt)
	}
	if candidate.Type != after.Type {
		return candidate.Type > after.Type
	}
	return candidate.ID < after.ID
}

func TestServiceCursorIsSignedBoundExpiringAndSnapshotStableAcrossOwners(t *testing.T) {
	snapshotAt := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	content := &lexicalReaderStub{createdAt: map[string]time.Time{}, candidates: []searchdomain.Candidate{
		{Type: searchdomain.ResourceContent, ID: 9, Title: "content first", OccurredAt: snapshotAt.Add(-time.Minute), Score: 0.8},
		{Type: searchdomain.ResourceContent, ID: 8, Title: "content second", OccurredAt: snapshotAt.Add(-time.Minute), Score: 0.6},
	}}
	event := &lexicalReaderStub{createdAt: map[string]time.Time{}, candidates: []searchdomain.Candidate{
		{Type: searchdomain.ResourceEvent, ID: 3, Title: "event first", OccurredAt: snapshotAt.Add(-time.Minute), Score: 0.9},
		{Type: searchdomain.ResourceEvent, ID: 2, Title: "event second", OccurredAt: snapshotAt.Add(-time.Minute), Score: 0.7},
	}}
	knowledge := &lexicalReaderStub{createdAt: map[string]time.Time{}}
	for _, reader := range []*lexicalReaderStub{content, event} {
		for _, candidate := range reader.candidates {
			reader.createdAt[string(candidate.Type)+":"+strconv.FormatInt(candidate.ID, 10)] = snapshotAt.Add(-time.Hour)
		}
	}
	codec, err := pagination.NewCodec("search-cursor-application-test-secret-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	subjects := &subjectReaderStub{subject: viewerSubject}
	service, err := NewServiceWithCursorCodec(Readers{Content: content, Event: event, Knowledge: knowledge}, subjects, codec)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return snapshotAt }
	request := Request{Subject: viewerSubject, Query: searchdomain.Query{Keyword: "release", Limit: 2}}
	first, err := service.Search(context.Background(), request)
	if err != nil || len(first.Items) != 2 || first.Items[0].ID != 3 || first.Items[1].ID != 9 || first.NextCursor == "" {
		t.Fatalf("first search page = %#v / %v", first, err)
	}
	if _, err := strconv.ParseInt(first.NextCursor, 10, 64); err == nil {
		t.Fatalf("search cursor is a naked integer: %q", first.NextCursor)
	}
	knowledge.candidates = []searchdomain.Candidate{{
		Type: searchdomain.ResourceKnowledge, ID: 11, Title: "concurrent", OccurredAt: snapshotAt, Score: 0.85,
	}}
	knowledge.createdAt["knowledge:11"] = snapshotAt.Add(time.Second)
	request.Cursor = first.NextCursor
	second, err := service.Search(context.Background(), request)
	if err != nil || len(second.Items) != 2 || second.Items[0].ID != 2 || second.Items[1].ID != 8 || second.NextCursor != "" {
		t.Fatalf("second search page = %#v / %v", second, err)
	}
	if len(content.queries) < 2 || content.queries[1].After == nil || !content.queries[1].SnapshotAt.Equal(snapshotAt) || content.queries[1].CandidateLimit != 3 {
		t.Fatalf("decoded page query = %#v", content.queries)
	}

	tampered := "A" + first.NextCursor[1:]
	if tampered == first.NextCursor {
		tampered = "B" + first.NextCursor[1:]
	}
	request.Cursor = tampered
	if _, err := service.Search(context.Background(), request); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("tampered cursor error = %v, want invalid query", err)
	}
	request.Cursor = first.NextCursor
	request.Query.Status = "active"
	if _, err := service.Search(context.Background(), request); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("cross-filter cursor error = %v, want invalid query", err)
	}
	request.Query.Status = ""
	request.Subject = Subject{UserID: 8, Role: "admin"}
	subjects.subject = request.Subject
	if _, err := service.Search(context.Background(), request); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("cross-subject cursor error = %v, want invalid query", err)
	}

	expiringCodec, err := pagination.NewCodec("expiring-search-cursor-test-secret-32-bytes", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	subjects.subject = viewerSubject
	expiringService, err := NewServiceWithCursorCodec(Readers{Content: content, Event: event, Knowledge: knowledge}, subjects, expiringCodec)
	if err != nil {
		t.Fatal(err)
	}
	expiringService.now = func() time.Time { return snapshotAt }
	expiringFirst, err := expiringService.Search(context.Background(), Request{
		Subject: viewerSubject, Query: searchdomain.Query{Keyword: "release", Limit: 1},
	})
	if err != nil || expiringFirst.NextCursor == "" {
		t.Fatalf("expiring first page = %#v / %v", expiringFirst, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := expiringService.Search(context.Background(), Request{
		Subject: viewerSubject, Query: searchdomain.Query{Keyword: "release", Limit: 1}, Cursor: expiringFirst.NextCursor,
	}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("expired cursor error = %v, want invalid query", err)
	}
}

func TestServiceQueriesOnlySelectedOwnersAndReturnsStableSafeHighlights(t *testing.T) {
	now := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	content := &lexicalReaderStub{candidates: []searchdomain.Candidate{
		{Type: searchdomain.ResourceContent, ID: 9, Title: `<img src=x onerror=alert(1)> Release`, Snippet: `[x](javascript:alert(1)) release`, Status: "active", OccurredAt: now, Score: 0.7},
	}}
	event := &lexicalReaderStub{candidates: []searchdomain.Candidate{
		{Type: searchdomain.ResourceEvent, ID: 3, Title: "release 发布", Snippet: "芯片 release", Status: "active", OccurredAt: now, Score: 0.9},
	}}
	knowledge := &lexicalReaderStub{}
	service, err := NewService(Readers{Content: content, Event: event, Knowledge: knowledge}, &subjectReaderStub{subject: viewerSubject})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Search(context.Background(), Request{Subject: viewerSubject, Query: searchdomain.Query{
		Keyword: "release", Types: []searchdomain.ResourceType{searchdomain.ResourceContent, searchdomain.ResourceEvent}, Limit: 10,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if content.calls != 1 || event.calls != 1 || knowledge.calls != 0 || len(result.Items) != 2 || result.Items[0].Type != searchdomain.ResourceEvent || result.Items[1].Type != searchdomain.ResourceContent {
		t.Fatalf("search result/calls = %#v content:%d event:%d knowledge:%d", result, content.calls, event.calls, knowledge.calls)
	}
	unsafe := result.Items[1]
	if strings.Contains(unsafe.TitleHighlight, "<img") || strings.Contains(unsafe.SnippetHighlight, "javascript:") && strings.Contains(unsafe.SnippetHighlight, "href=") || strings.Count(unsafe.TitleHighlight, "<mark>") != 1 || !strings.Contains(unsafe.TitleHighlight, "&lt;img") {
		t.Fatalf("unsafe highlight = %#v", unsafe)
	}
	for _, markup := range []string{unsafe.TitleHighlight, unsafe.SnippetHighlight} {
		if strings.Contains(markup, "<script") || strings.Contains(markup, "onerror=") && strings.Contains(markup, "<img") {
			t.Fatalf("active markup escaped incompletely: %q", markup)
		}
	}
}

func TestServiceRejectsInvalidReaderCandidatesAndDoesNotReturnPartialResults(t *testing.T) {
	reader := &lexicalReaderStub{candidates: []searchdomain.Candidate{{Type: searchdomain.ResourceContent, ID: 1, Title: "invalid missing time"}}}
	service, err := NewService(Readers{Content: reader, Event: &lexicalReaderStub{}, Knowledge: &lexicalReaderStub{}}, &subjectReaderStub{subject: viewerSubject})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Search(context.Background(), Request{Subject: viewerSubject, Query: searchdomain.Query{Keyword: "invalid", Types: []searchdomain.ResourceType{searchdomain.ResourceContent}}}); !errors.Is(err, ErrInvalidProjection) {
		t.Fatalf("invalid projection error = %v", err)
	}
	reader.candidates = nil
	reader.err = errors.New("private body sentinel")
	if _, err := service.Search(context.Background(), Request{Subject: viewerSubject, Query: searchdomain.Query{Keyword: "invalid", Types: []searchdomain.ResourceType{searchdomain.ResourceContent}}}); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("reader error = %v", err)
	}
}

func TestServiceSupportsLatestSortAcrossOwners(t *testing.T) {
	now := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	content := &lexicalReaderStub{candidates: []searchdomain.Candidate{{Type: searchdomain.ResourceContent, ID: 1, Title: "new", OccurredAt: now, Score: 0.1}}}
	event := &lexicalReaderStub{candidates: []searchdomain.Candidate{{Type: searchdomain.ResourceEvent, ID: 2, Title: "old", OccurredAt: now.Add(-time.Hour), Score: 0.9}}}
	service, err := NewService(Readers{Content: content, Event: event, Knowledge: &lexicalReaderStub{}}, &subjectReaderStub{subject: viewerSubject})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Search(context.Background(), Request{Subject: viewerSubject, Query: searchdomain.Query{Keyword: "release", Sort: searchdomain.SortLatest}})
	if err != nil || len(result.Items) != 2 || result.Items[0].ID != 1 {
		t.Fatalf("latest result = %#v/%v", result, err)
	}
}

func TestServiceRechecksCurrentSubjectAndOwnerVisibilityBeforeDTOAssembly(t *testing.T) {
	now := time.Now().UTC()
	visible := false
	content := &lexicalReaderStub{visible: &visible, candidates: []searchdomain.Candidate{{Type: searchdomain.ResourceContent, ID: 1, Title: "hidden", OccurredAt: now, Score: 0.8}}}
	subjects := &subjectReaderStub{subject: viewerSubject}
	service, err := NewService(Readers{Content: content, Event: &lexicalReaderStub{}, Knowledge: &lexicalReaderStub{}}, subjects)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Search(context.Background(), Request{Subject: viewerSubject, Query: searchdomain.Query{Keyword: "hidden"}})
	if err != nil || len(result.Items) != 0 || subjects.calls != 1 {
		t.Fatalf("hidden result/subject checks = %#v/%v/%d", result, err, subjects.calls)
	}
	content.visible = nil
	subjects.subject.Role = "admin"
	if _, err := service.Search(context.Background(), Request{Subject: viewerSubject, Query: searchdomain.Query{Keyword: "hidden"}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("changed subject error = %v", err)
	}
}

func TestHighlightEscapesEveryUntrustedByteBeforeAddingControlledMarks(t *testing.T) {
	got := highlight(`A <svg onload=alert(1)> & "release" javascript:`, "release")
	want := `A &lt;svg onload=alert(1)&gt; &amp; &#34;<mark>release</mark>&#34; javascript:`
	if got != want {
		t.Fatalf("highlight() = %q, want %q", got, want)
	}
}
