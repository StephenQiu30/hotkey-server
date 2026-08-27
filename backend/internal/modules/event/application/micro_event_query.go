package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrInvalidMicroEventQuery = errors.New("micro-event query is invalid")
var ErrEvidenceSummaryUnavailable = errors.New("micro-event evidence summary is unavailable")

type MicroEventListQuery struct {
	Cursor         string
	Limit          int
	Statuses       []string
	Sort           string
	MonitorID      int64
	SourceTypes    []string
	EvidenceStates []string
	StartedFrom    *time.Time
	StartedTo      *time.Time
}

type MicroEventProjectionDTO struct {
	ID, Version                         int64
	EventKey, Status                    string
	PrimarySubjectKey, PrimaryActionKey string
	LocationKeys, IdentifierKeys        []string
	EventStartedAt                      time.Time
	EventEndedAt                        *time.Time
	ClusteringProfileVersion            string
	Storyline                           *StorylineDTO
	LatestHeat                          *EventHeatSnapshotDTO
	RelevanceScore                      *float64
	LatestEvidenceState                 *EvidenceStateSnapshotDTO
	LatestSummary                       *EvidenceSummaryDTO
	ContentFamilyCount                  int
	DocumentCount                       int
	Members                             []MicroEventMemberProjectionDTO
}

type MicroEventMemberProjectionDTO struct {
	ID, Version, ContentFamilyID, MembershipDecisionID int64
	ClusteringProfileVersion                           string
}

type MicroEventPageDTO struct {
	Items      []MicroEventProjectionDTO
	NextCursor string
}

type MicroEventEvidenceQuery struct {
	MicroEventID int64
	CursorID     int64
	Limit        int
	AsOf         time.Time
}

type ClaimEvidenceProjectionDTO struct {
	ID, Version, ClaimID, ClaimVersion, DocumentVersionID, TextQuoteSelectorID int64
	ContentFamilyID, LineageRootID                                             int64
	LineageDecisionID, ContentFamilyMemberVersion                              *int64
	ClaimSubject, ClaimPredicate, ClaimObject                                  string
	Relation, Availability                                                     string
	ExactQuote, Prefix, Suffix                                                 *string
	UTF8ByteStart, UTF8ByteEnd                                                 *int64
	QuoteSHA256, PlaintextSHA256, SelectorVersion                              *string
	MarkdownAnchor                                                             *string
	SourceRecordURL, CanonicalURL                                              *string
	PublisherName, ContentOriginName                                           *string
	PublishedAt                                                                *time.Time
	CapturedAt                                                                 time.Time
	ExtractionSchemaVersion, DecisionOrigin                                    string
	CreatedAt                                                                  time.Time
}

type MicroEventEvidencePageDTO struct {
	Items        []ClaimEvidenceProjectionDTO
	NextCursorID int64
}

type MicroEventQueryRepository interface {
	ListMicroEvents(context.Context, MicroEventListQuery) (MicroEventPageDTO, error)
	GetMicroEvent(context.Context, int64) (MicroEventProjectionDTO, error)
	ListMicroEventEvidence(context.Context, MicroEventEvidenceQuery) (MicroEventEvidencePageDTO, error)
	GetMicroEventSummary(context.Context, int64, int64) (*EvidenceSummaryDTO, error)
}

type MicroEventQueryService struct{ repository MicroEventQueryRepository }

func NewMicroEventQueryService(repository MicroEventQueryRepository) (*MicroEventQueryService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidMicroEventQuery)
	}
	return &MicroEventQueryService{repository: repository}, nil
}

func (service *MicroEventQueryService) List(ctx context.Context, query MicroEventListQuery) (MicroEventPageDTO, error) {
	query.Cursor = strings.TrimSpace(query.Cursor)
	query.Sort = strings.TrimSpace(query.Sort)
	if query.Sort == "" {
		query.Sort = "heat"
	}
	if service == nil || service.repository == nil || len(query.Cursor) > 8192 || strings.ContainsAny(query.Cursor, "\r\n") ||
		(query.Sort != "heat" && query.Sort != "relevance" && query.Sort != "latest") || query.Limit < 0 || query.Limit > 100 ||
		query.MonitorID < 0 {
		return MicroEventPageDTO{}, ErrInvalidMicroEventQuery
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	var err error
	query.Statuses, err = normalizeMicroEventFilterValues(query.Statuses, map[string]struct{}{
		"active": {}, "review_pending": {}, "closed": {}, "merged": {},
	})
	if err != nil {
		return MicroEventPageDTO{}, err
	}
	query.SourceTypes, err = normalizeMicroEventFilterValues(query.SourceTypes, map[string]struct{}{
		"rss": {}, "hacker_news": {}, "x": {}, "bing_grounding": {}, "bilibili": {}, "weibo": {}, "google_agent_search": {},
	})
	if err != nil {
		return MicroEventPageDTO{}, err
	}
	query.EvidenceStates, err = normalizeMicroEventFilterValues(query.EvidenceStates, map[string]struct{}{
		"no_citable_body": {}, "single_origin": {}, "multiple_origins": {}, "conflicting_reports": {},
		"publisher_corrected": {}, "publisher_withdrawn": {},
	})
	if err != nil {
		return MicroEventPageDTO{}, err
	}
	if query.StartedFrom != nil {
		value := query.StartedFrom.UTC()
		query.StartedFrom = &value
	}
	if query.StartedTo != nil {
		value := query.StartedTo.UTC()
		query.StartedTo = &value
	}
	if query.StartedFrom != nil && query.StartedTo != nil && query.StartedFrom.After(*query.StartedTo) {
		return MicroEventPageDTO{}, ErrInvalidMicroEventQuery
	}
	return service.repository.ListMicroEvents(ctx, query)
}

func normalizeMicroEventFilterValues(values []string, allowed map[string]struct{}) ([]string, error) {
	if len(values) > len(allowed) {
		return nil, ErrInvalidMicroEventQuery
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if _, ok := allowed[value]; !ok {
			return nil, ErrInvalidMicroEventQuery
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func (service *MicroEventQueryService) Get(ctx context.Context, id int64) (MicroEventProjectionDTO, error) {
	if service == nil || service.repository == nil || id <= 0 {
		return MicroEventProjectionDTO{}, ErrInvalidMicroEventQuery
	}
	result, err := service.repository.GetMicroEvent(ctx, id)
	if err != nil {
		return MicroEventProjectionDTO{}, err
	}
	summary, err := service.repository.GetMicroEventSummary(ctx, result.ID, result.Version)
	if err != nil && !errors.Is(err, ErrEvidenceSummaryUnavailable) {
		return MicroEventProjectionDTO{}, err
	}
	result.LatestSummary = summary
	return result, nil
}

func (service *MicroEventQueryService) Evidence(ctx context.Context, query MicroEventEvidenceQuery) (MicroEventEvidencePageDTO, error) {
	if service == nil || service.repository == nil || query.MicroEventID <= 0 || query.CursorID < 0 || query.Limit < 0 || query.Limit > 100 {
		return MicroEventEvidencePageDTO{}, ErrInvalidMicroEventQuery
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.AsOf.IsZero() {
		query.AsOf = time.Now().UTC()
	} else {
		query.AsOf = query.AsOf.UTC()
	}
	return service.repository.ListMicroEventEvidence(ctx, query)
}
