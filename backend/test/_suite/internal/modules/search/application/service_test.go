package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
)

type lexicalReaderStub struct {
	calls      int
	visible    *bool
	candidates []searchdomain.Candidate
	err        error
}

func (reader *lexicalReaderStub) Search(_ context.Context, _ searchdomain.Query) ([]searchdomain.Candidate, error) {
	reader.calls++
	return append([]searchdomain.Candidate(nil), reader.candidates...), reader.err
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
