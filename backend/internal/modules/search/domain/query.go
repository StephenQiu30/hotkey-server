package domain

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ResourceType string

const (
	ResourceContent   ResourceType = "content"
	ResourceEvent     ResourceType = "event"
	ResourceKnowledge ResourceType = "knowledge"

	DefaultLimit        = 20
	MaximumLimit        = 100
	MaximumKeywordRunes = 100
	MaximumEntityRunes  = 128
	MaximumTitleBytes   = 4 << 10
	MaximumSnippetBytes = 8 << 10
	SortRelevance       = "relevance"
	SortLatest          = "latest"
)

var stableStatusPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func (resourceType ResourceType) Valid() bool {
	return resourceType == ResourceContent || resourceType == ResourceEvent || resourceType == ResourceKnowledge
}

type Query struct {
	Keyword            string
	Types              []ResourceType
	SourceConnectionID *int64
	MonitorID          *int64
	Entity             string
	Status             string
	Sort               string
	From               *time.Time
	To                 *time.Time
	Limit              int
	// CandidateLimit, SnapshotAt and After are application-owned paging
	// controls. They are never accepted directly from HTTP clients.
	CandidateLimit int
	SnapshotAt     time.Time
	After          *Position
}

type Position struct {
	Type       ResourceType `json:"type"`
	ID         int64        `json:"id"`
	OccurredAt time.Time    `json:"occurred_at"`
	Score      float64      `json:"score"`
}

func (position Position) Validate() error {
	if !position.Type.Valid() || position.ID <= 0 || position.OccurredAt.IsZero() || position.OccurredAt.Location() != time.UTC ||
		math.IsNaN(position.Score) || math.IsInf(position.Score, 0) || position.Score < 0 || position.Score > 100 {
		return fmt.Errorf("invalid search position")
	}
	return nil
}

func (query Query) Normalized() Query {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Entity = strings.TrimSpace(query.Entity)
	query.Status = strings.TrimSpace(query.Status)
	query.Sort = strings.TrimSpace(query.Sort)
	if query.Sort == "" {
		query.Sort = SortRelevance
	}
	if query.Limit == 0 {
		query.Limit = DefaultLimit
	}
	if query.CandidateLimit == 0 {
		query.CandidateLimit = query.Limit
	}
	selected := make(map[ResourceType]struct{}, len(query.Types))
	for _, resourceType := range query.Types {
		selected[resourceType] = struct{}{}
	}
	if len(selected) == 0 {
		selected[ResourceContent] = struct{}{}
		selected[ResourceEvent] = struct{}{}
		selected[ResourceKnowledge] = struct{}{}
	}
	query.Types = make([]ResourceType, 0, len(selected))
	for resourceType := range selected {
		query.Types = append(query.Types, resourceType)
	}
	sort.Slice(query.Types, func(left, right int) bool { return query.Types[left] < query.Types[right] })
	if query.From != nil {
		value := query.From.UTC()
		query.From = &value
	}
	if query.To != nil {
		value := query.To.UTC()
		query.To = &value
	}
	if !query.SnapshotAt.IsZero() {
		query.SnapshotAt = query.SnapshotAt.UTC()
	}
	if query.After != nil {
		position := *query.After
		position.OccurredAt = position.OccurredAt.UTC()
		query.After = &position
	}
	return query
}

func (query Query) Validate() error {
	query = query.Normalized()
	if query.Keyword == "" || utf8.RuneCountInString(query.Keyword) > MaximumKeywordRunes || containsControl(query.Keyword) {
		return fmt.Errorf("invalid search keyword")
	}
	if query.Limit < 1 || query.Limit > MaximumLimit {
		return fmt.Errorf("invalid search limit")
	}
	if query.CandidateLimit < query.Limit || query.CandidateLimit > MaximumLimit+1 {
		return fmt.Errorf("invalid search candidate limit")
	}
	for _, resourceType := range query.Types {
		if !resourceType.Valid() {
			return fmt.Errorf("invalid search resource type")
		}
	}
	if query.SourceConnectionID != nil && *query.SourceConnectionID <= 0 || query.MonitorID != nil && *query.MonitorID <= 0 {
		return fmt.Errorf("invalid search reference")
	}
	if utf8.RuneCountInString(query.Entity) > MaximumEntityRunes || containsControl(query.Entity) {
		return fmt.Errorf("invalid search entity")
	}
	if query.Status != "" && !stableStatusPattern.MatchString(query.Status) {
		return fmt.Errorf("invalid search status")
	}
	if query.Sort != SortRelevance && query.Sort != SortLatest {
		return fmt.Errorf("invalid search sort")
	}
	if query.From != nil && query.From.IsZero() || query.To != nil && query.To.IsZero() ||
		query.From != nil && query.To != nil && query.From.After(*query.To) {
		return fmt.Errorf("invalid search time range")
	}
	if query.After != nil {
		if query.SnapshotAt.IsZero() || query.After.Validate() != nil {
			return fmt.Errorf("invalid search page")
		}
	}
	return nil
}

func (query Query) Includes(resourceType ResourceType) bool {
	query = query.Normalized()
	index := sort.Search(len(query.Types), func(index int) bool { return query.Types[index] >= resourceType })
	return index < len(query.Types) && query.Types[index] == resourceType
}

// Candidate is the bounded, displayable projection returned by one owning
// module. It contains no raw evidence, object key, Vault path or provider data.
type Candidate struct {
	Type ResourceType
	ID   int64
	// SourceConnectionID is an internal authorization fact. Transport DTOs
	// deliberately omit it.
	SourceConnectionID int64
	Title              string
	Snippet            string
	Status             string
	OccurredAt         time.Time
	Score              float64
}

func (candidate Candidate) Validate() error {
	if !candidate.Type.Valid() || candidate.ID <= 0 || candidate.SourceConnectionID < 0 || strings.TrimSpace(candidate.Title) == "" || len(candidate.Title) > MaximumTitleBytes ||
		len(candidate.Snippet) > MaximumSnippetBytes || candidate.OccurredAt.IsZero() || candidate.OccurredAt.Location() != time.UTC ||
		math.IsNaN(candidate.Score) || math.IsInf(candidate.Score, 0) || candidate.Score < 0 || candidate.Score > 100 ||
		candidate.Status != "" && !stableStatusPattern.MatchString(candidate.Status) {
		return fmt.Errorf("invalid search candidate")
	}
	return nil
}

func containsControl(value string) bool {
	return strings.IndexByte(value, 0) >= 0 || strings.IndexFunc(value, unicode.IsControl) >= 0
}
