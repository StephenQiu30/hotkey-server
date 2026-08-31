// Package application implements Search use cases and consumer-owned ports.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"regexp"
	"sort"
	"strings"
	"time"

	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
)

var (
	ErrInvalidQuery      = errors.New("search query is invalid")
	ErrInvalidProjection = errors.New("search projection is invalid")
	ErrUnavailable       = errors.New("search is unavailable")
)

type LexicalReader interface {
	Search(context.Context, searchdomain.Query) ([]searchdomain.Candidate, error)
	CanDisplay(context.Context, searchdomain.Query, searchdomain.Candidate) (bool, error)
}

type Subject struct {
	UserID int64
	Role   string
}

func (subject Subject) valid() bool {
	if subject.UserID <= 0 {
		return false
	}
	switch subject.Role {
	case "viewer", "analyst", "editor", "admin":
		return true
	default:
		return false
	}
}

type SubjectReader interface {
	CurrentSearchSubject(context.Context, int64) (Subject, error)
	SearchScopeVisible(context.Context, Subject, searchdomain.Query) (bool, error)
	SearchCandidateVisible(context.Context, Subject, searchdomain.Candidate) (bool, error)
}

type Request struct {
	Query   searchdomain.Query
	Subject Subject
	Cursor  string
}

type Readers struct {
	Content   LexicalReader
	Event     LexicalReader
	Knowledge LexicalReader
}

type Service struct {
	readers     Readers
	subjects    SubjectReader
	cursorCodec *pagination.Codec
	now         func() time.Time
}

type Item struct {
	Type             searchdomain.ResourceType
	ID               int64
	Title            string
	Snippet          string
	TitleHighlight   string
	SnippetHighlight string
	Status           string
	OccurredAt       time.Time
	Score            float64
}

type Result struct {
	Items      []Item
	NextCursor string
}

type searchResultCursor struct {
	Version           int                   `json:"v"`
	SubjectUserID     int64                 `json:"user_id"`
	SubjectRole       string                `json:"role"`
	FilterFingerprint string                `json:"filter"`
	SnapshotAt        time.Time             `json:"as_of"`
	Position          searchdomain.Position `json:"position"`
}

func NewService(readers Readers, subjects SubjectReader) (*Service, error) {
	return NewServiceWithCursorCodec(readers, subjects, pagination.NewTestCodec("search-application"))
}

func NewServiceWithCursorCodec(readers Readers, subjects SubjectReader, cursorCodec *pagination.Codec) (*Service, error) {
	if readers.Content == nil || readers.Event == nil || readers.Knowledge == nil || subjects == nil || cursorCodec == nil {
		return nil, fmt.Errorf("all lexical search readers and current subject reader are required")
	}
	return &Service{readers: readers, subjects: subjects, cursorCodec: cursorCodec, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (service *Service) Search(ctx context.Context, request Request) (Result, error) {
	if service == nil || service.cursorCodec == nil || service.now == nil {
		return Result{}, ErrUnavailable
	}
	request.Cursor = strings.TrimSpace(request.Cursor)
	if len(request.Cursor) > 8192 || strings.ContainsAny(request.Cursor, "\r\n") {
		return Result{}, ErrInvalidQuery
	}
	if !request.Subject.valid() {
		return Result{}, ErrInvalidQuery
	}
	currentSubject, err := service.subjects.CurrentSearchSubject(ctx, request.Subject.UserID)
	if err != nil || currentSubject != request.Subject || !currentSubject.valid() {
		return Result{}, ErrUnavailable
	}
	query := request.Query
	query = query.Normalized()
	if err := query.Validate(); err != nil {
		return Result{}, fmt.Errorf("%w", ErrInvalidQuery)
	}
	scopeVisible, err := service.subjects.SearchScopeVisible(ctx, currentSubject, query)
	if err != nil {
		return Result{}, fmt.Errorf("%w", ErrUnavailable)
	}
	if !scopeVisible {
		return Result{Items: []Item{}}, nil
	}
	fingerprint, err := searchFilterFingerprint(query)
	if err != nil {
		return Result{}, ErrInvalidQuery
	}
	cursor := searchResultCursor{
		Version: 1, SubjectUserID: currentSubject.UserID, SubjectRole: currentSubject.Role,
		FilterFingerprint: fingerprint, SnapshotAt: service.now().UTC(),
	}
	if request.Cursor != "" {
		cursor, err = decodeSearchResultCursor(service.cursorCodec, request.Cursor, currentSubject, fingerprint)
		if err != nil {
			return Result{}, ErrInvalidQuery
		}
		query.After = &cursor.Position
	}
	query.SnapshotAt = cursor.SnapshotAt
	query.CandidateLimit = query.Limit + 1
	owners := []struct {
		resourceType searchdomain.ResourceType
		reader       LexicalReader
	}{
		{resourceType: searchdomain.ResourceContent, reader: service.readers.Content},
		{resourceType: searchdomain.ResourceEvent, reader: service.readers.Event},
		{resourceType: searchdomain.ResourceKnowledge, reader: service.readers.Knowledge},
	}
	candidates := make([]searchdomain.Candidate, 0, query.Limit*len(owners))
	seen := make(map[string]int, query.Limit*len(owners))
	for _, owner := range owners {
		if !query.Includes(owner.resourceType) {
			continue
		}
		items, err := owner.reader.Search(ctx, query)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{}, ctxErr
			}
			return Result{}, fmt.Errorf("%w", ErrUnavailable)
		}
		if len(items) > query.CandidateLimit {
			return Result{}, fmt.Errorf("%w", ErrInvalidProjection)
		}
		for _, candidate := range items {
			if candidate.Type != owner.resourceType || candidate.Validate() != nil {
				return Result{}, fmt.Errorf("%w", ErrInvalidProjection)
			}
			if query.After != nil && !candidateFollowsPosition(candidate, *query.After, query.Sort) {
				return Result{}, fmt.Errorf("%w", ErrInvalidProjection)
			}
			key := fmt.Sprintf("%s:%d", candidate.Type, candidate.ID)
			if index, found := seen[key]; found {
				if candidate.Score > candidates[index].Score {
					candidates[index] = candidate
				}
				continue
			}
			seen[key] = len(candidates)
			candidates = append(candidates, candidate)
		}
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if query.Sort == searchdomain.SortRelevance && candidates[left].Score != candidates[right].Score {
			return candidates[left].Score > candidates[right].Score
		}
		if !candidates[left].OccurredAt.Equal(candidates[right].OccurredAt) {
			return candidates[left].OccurredAt.After(candidates[right].OccurredAt)
		}
		if candidates[left].Type != candidates[right].Type {
			return candidates[left].Type < candidates[right].Type
		}
		if query.Sort == searchdomain.SortLatest && candidates[left].Score != candidates[right].Score {
			return candidates[left].Score > candidates[right].Score
		}
		return candidates[left].ID > candidates[right].ID
	})
	result := Result{Items: make([]Item, 0, min(query.Limit+1, len(candidates)))}
	visiblePositions := make([]searchdomain.Position, 0, min(query.Limit+1, len(candidates)))
	for _, candidate := range candidates {
		visible, err := readerForType(service.readers, candidate.Type).CanDisplay(ctx, query, candidate)
		if err != nil {
			return Result{}, fmt.Errorf("%w", ErrUnavailable)
		}
		if !visible {
			continue
		}
		currentSubject, err = service.subjects.CurrentSearchSubject(ctx, request.Subject.UserID)
		if err != nil || currentSubject != request.Subject || !currentSubject.valid() {
			return Result{}, fmt.Errorf("%w", ErrUnavailable)
		}
		scopeVisible, err = service.subjects.SearchScopeVisible(ctx, currentSubject, query)
		if err != nil {
			return Result{}, fmt.Errorf("%w", ErrUnavailable)
		}
		candidateVisible, candidateErr := service.subjects.SearchCandidateVisible(ctx, currentSubject, candidate)
		if candidateErr != nil {
			return Result{}, fmt.Errorf("%w", ErrUnavailable)
		}
		if !scopeVisible || !candidateVisible {
			continue
		}
		result.Items = append(result.Items, Item{
			Type: candidate.Type, ID: candidate.ID, Title: candidate.Title, Snippet: candidate.Snippet,
			TitleHighlight: highlight(candidate.Title, query.Keyword), SnippetHighlight: highlight(candidate.Snippet, query.Keyword),
			Status: candidate.Status, OccurredAt: candidate.OccurredAt, Score: candidate.Score,
		})
		visiblePositions = append(visiblePositions, searchdomain.Position{
			Type: candidate.Type, ID: candidate.ID, OccurredAt: candidate.OccurredAt, Score: candidate.Score,
		})
		if len(result.Items) > query.Limit {
			break
		}
	}
	if len(result.Items) > query.Limit {
		cursor.Position = visiblePositions[query.Limit-1]
		result.NextCursor, err = encodeSearchResultCursor(service.cursorCodec, cursor)
		if err != nil {
			return Result{}, fmt.Errorf("%w", ErrUnavailable)
		}
		result.Items = result.Items[:query.Limit]
	}
	return result, nil
}

func encodeSearchResultCursor(cursorCodec *pagination.Codec, cursor searchResultCursor) (string, error) {
	cursor.Version = 1
	return cursorCodec.Seal("search_result_list", cursor)
}

func decodeSearchResultCursor(cursorCodec *pagination.Codec, value string, subject Subject, fingerprint string) (searchResultCursor, error) {
	var cursor searchResultCursor
	if err := cursorCodec.Open(value, "search_result_list", &cursor); err != nil {
		return searchResultCursor{}, err
	}
	if cursor.Version != 1 || cursor.SubjectUserID != subject.UserID || cursor.SubjectRole != subject.Role ||
		cursor.FilterFingerprint != fingerprint || cursor.SnapshotAt.IsZero() || cursor.Position.Validate() != nil {
		return searchResultCursor{}, ErrInvalidQuery
	}
	cursor.SnapshotAt = cursor.SnapshotAt.UTC()
	cursor.Position.OccurredAt = cursor.Position.OccurredAt.UTC()
	return cursor, nil
}

func searchFilterFingerprint(query searchdomain.Query) (string, error) {
	query = query.Normalized()
	type fingerprint struct {
		Keyword            string                      `json:"q"`
		Types              []searchdomain.ResourceType `json:"types"`
		SourceConnectionID *int64                      `json:"source_id,omitempty"`
		MonitorID          *int64                      `json:"monitor_id,omitempty"`
		Entity             string                      `json:"entity,omitempty"`
		Status             string                      `json:"status,omitempty"`
		Sort               string                      `json:"sort"`
		From               *time.Time                  `json:"from,omitempty"`
		To                 *time.Time                  `json:"to,omitempty"`
	}
	payload, err := json.Marshal(fingerprint{
		Keyword: query.Keyword, Types: query.Types, SourceConnectionID: query.SourceConnectionID, MonitorID: query.MonitorID,
		Entity: query.Entity, Status: query.Status, Sort: query.Sort, From: query.From, To: query.To,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest), nil
}

func candidateFollowsPosition(candidate searchdomain.Candidate, after searchdomain.Position, sortOrder string) bool {
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

func readerForType(readers Readers, resourceType searchdomain.ResourceType) LexicalReader {
	switch resourceType {
	case searchdomain.ResourceContent:
		return readers.Content
	case searchdomain.ResourceEvent:
		return readers.Event
	default:
		return readers.Knowledge
	}
}

type highlightSpan struct{ start, end int }

// highlight escapes the complete untrusted display value before inserting
// the only two permitted markup tokens around query matches.
func highlight(value, keyword string) string {
	if value == "" {
		return ""
	}
	terms := highlightTerms(keyword)
	spans := make([]highlightSpan, 0)
	for _, term := range terms {
		pattern, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(term))
		if err != nil {
			continue
		}
		for _, match := range pattern.FindAllStringIndex(value, -1) {
			spans = append(spans, highlightSpan{start: match[0], end: match[1]})
		}
	}
	if len(spans) == 0 {
		return stdhtml.EscapeString(value)
	}
	sort.Slice(spans, func(left, right int) bool {
		if spans[left].start == spans[right].start {
			return spans[left].end > spans[right].end
		}
		return spans[left].start < spans[right].start
	})
	merged := spans[:0]
	for _, span := range spans {
		if len(merged) == 0 || span.start > merged[len(merged)-1].end {
			merged = append(merged, span)
			continue
		}
		if span.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = span.end
		}
	}
	var result strings.Builder
	position := 0
	for _, span := range merged {
		result.WriteString(stdhtml.EscapeString(value[position:span.start]))
		result.WriteString("<mark>")
		result.WriteString(stdhtml.EscapeString(value[span.start:span.end]))
		result.WriteString("</mark>")
		position = span.end
	}
	result.WriteString(stdhtml.EscapeString(value[position:]))
	return result.String()
}

func highlightTerms(keyword string) []string {
	values := append([]string{strings.TrimSpace(keyword)}, strings.Fields(keyword)...)
	terms := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, found := seen[key]; found {
			continue
		}
		seen[key] = struct{}{}
		terms = append(terms, value)
	}
	return terms
}
