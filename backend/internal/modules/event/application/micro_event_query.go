package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidMicroEventQuery = errors.New("micro-event query is invalid")
var ErrEvidenceSummaryUnavailable = errors.New("micro-event evidence summary is unavailable")

type MicroEventListQuery struct {
	CursorID int64
	Limit    int
	Statuses []string
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
	LatestEvidenceState                 *EvidenceStateSnapshotDTO
	LatestSummary                       *EvidenceSummaryDTO
	ContentFamilyCount                  int
	DocumentCount                       int
}

type MicroEventPageDTO struct {
	Items        []MicroEventProjectionDTO
	NextCursorID int64
}

type MicroEventEvidenceQuery struct {
	MicroEventID int64
	CursorID     int64
	Limit        int
	AsOf         time.Time
}

type ClaimEvidenceProjectionDTO struct {
	ID, Version, ClaimID, DocumentVersionID, TextQuoteSelectorID int64
	ContentFamilyID, LineageRootID                               int64
	LineageDecisionID, ContentFamilyMemberVersion                *int64
	ClaimSubject, ClaimPredicate, ClaimObject                    string
	Relation, Availability                                       string
	ExactQuote, Prefix, Suffix                                   *string
	UTF8ByteStart, UTF8ByteEnd                                   *int64
	QuoteSHA256, PlaintextSHA256, SelectorVersion                *string
	MarkdownAnchor                                               *string
	SourceRecordURL, CanonicalURL                                *string
	PublisherName, ContentOriginName                             *string
	PublishedAt                                                  *time.Time
	CapturedAt                                                   time.Time
	ExtractionSchemaVersion, DecisionOrigin                      string
	CreatedAt                                                    time.Time
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
	if service == nil || service.repository == nil || query.CursorID < 0 || query.Limit < 0 || query.Limit > 100 {
		return MicroEventPageDTO{}, ErrInvalidMicroEventQuery
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	for _, status := range query.Statuses {
		if status != "active" && status != "review_pending" && status != "closed" && status != "merged" {
			return MicroEventPageDTO{}, ErrInvalidMicroEventQuery
		}
	}
	return service.repository.ListMicroEvents(ctx, query)
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
