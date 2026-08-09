package application

import (
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

// RawResponseHeadersDTO is the allowlisted, protocol-neutral response-header
// projection used by Source Application ports. Pointer fields preserve the
// distinction between an absent scalar header and a present value.
type RawResponseHeadersDTO struct {
	ContentType  *string
	ETag         *string
	LastModified *string
	Date         *string
	Links        []string
	RetryAfter   *string
}

func NewRawResponseHeadersDTO(values map[string][]string) (RawResponseHeadersDTO, error) {
	headers, err := domain.NewRawResponseHeaders(values)
	if err != nil {
		return RawResponseHeadersDTO{}, err
	}
	return rawResponseHeadersDTOFromEntity(headers), nil
}

func (headers RawResponseHeadersDTO) Validate() error {
	_, err := rawResponseHeadersEntityFromDTO(headers)
	return err
}

func (headers RawResponseHeadersDTO) Values() map[string][]string {
	values := make(map[string][]string, 6)
	appendScalarHeaderDTO(values, "Content-Type", headers.ContentType)
	appendScalarHeaderDTO(values, "ETag", headers.ETag)
	appendScalarHeaderDTO(values, "Last-Modified", headers.LastModified)
	appendScalarHeaderDTO(values, "Date", headers.Date)
	if headers.Links != nil {
		values["Link"] = append([]string(nil), headers.Links...)
	}
	appendScalarHeaderDTO(values, "Retry-After", headers.RetryAfter)
	return values
}

func (headers RawResponseHeadersDTO) Equal(other RawResponseHeadersDTO) bool {
	left, right := headers.Values(), other.Values()
	if len(left) != len(right) {
		return false
	}
	for name, values := range left {
		otherValues, found := right[name]
		if !found || len(values) != len(otherValues) {
			return false
		}
		for index := range values {
			if values[index] != otherValues[index] {
				return false
			}
		}
	}
	return true
}

// RawEvidenceSnapshotDTO carries raw bytes only inside the synchronous Source
// archive/selection call path. Persistence ports receive a byte-free command.
type RawEvidenceSnapshotDTO struct {
	EvidenceKey             string
	Payload                 []byte
	CollectorProfileVersion string
	MIMEType                string
	ResponseStatus          int
	RequestedURL            string
	FinalURL                string
	RedirectChain           []string
	ResponseHeaders         RawResponseHeadersDTO
	CapturedAt              time.Time
	PayloadSHA256           string
}

type RawEvidenceReferenceDTO struct {
	EvidenceKey           string
	LocatorType           string
	LocatorValue          string
	ByteStart             *int64
	ByteEnd               *int64
	SelectedPayloadSHA256 string
	SelectorVersion       string
}

func (reference RawEvidenceReferenceDTO) Validate() error {
	_, err := rawEvidenceReferenceEntityFromDTO(reference)
	return err
}

type RawEvidenceAttachmentDTO struct {
	URL       string
	MIMEType  string
	SizeBytes *int64
}

type RawEvidenceMetricsDTO struct {
	ViewCount    *int64
	LikeCount    *int64
	CommentCount *int64
	ShareCount   *int64
}

// RawEvidenceItemDTO is an in-memory archive input. Body must never be copied
// into a repository command, persistence record, durable job, or log field.
type RawEvidenceItemDTO struct {
	SourceCode           string
	ExternalID           string
	ParentExternalID     string
	ContentType          string
	Title                string
	Body                 string
	Language             string
	URL                  string
	Author               string
	PublishedAt          *time.Time
	ObservedAt           time.Time
	EvidenceCompleteness string
	Attachments          []RawEvidenceAttachmentDTO
	Metrics              RawEvidenceMetricsDTO
	SnapshotKey          string
	ItemLocator          string
	EvidenceReferences   []RawEvidenceReferenceDTO
}

type RawEvidenceFetchDTO struct {
	Items     []RawEvidenceItemDTO
	Snapshots []RawEvidenceSnapshotDTO
}

// EvidenceSelectorInputDTO is the validated POJO boundary shared by the
// archive verifier and the evidence reader selector.
type EvidenceSelectorInputDTO struct {
	Snapshot  RawEvidenceSnapshotDTO
	Reference RawEvidenceReferenceDTO
}

func (input EvidenceSelectorInputDTO) Validate() error {
	snapshot, err := rawEvidenceSnapshotEntityFromDTO(input.Snapshot)
	if err != nil {
		return fmt.Errorf("validate selector snapshot: %w", err)
	}
	reference, err := rawEvidenceReferenceEntityFromDTO(input.Reference)
	if err != nil {
		return fmt.Errorf("validate selector reference: %w", err)
	}
	if reference.SnapshotKey != snapshot.Key {
		return fmt.Errorf("selector reference does not identify its snapshot")
	}
	return nil
}

func rawResponseHeadersEntityFromDTO(headers RawResponseHeadersDTO) (domain.RawResponseHeaders, error) {
	return domain.NewRawResponseHeaders(headers.Values())
}

func rawResponseHeadersDTOFromEntity(headers domain.RawResponseHeaders) RawResponseHeadersDTO {
	values := headers.Values()
	return RawResponseHeadersDTO{
		ContentType:  scalarResponseHeaderDTO(values["Content-Type"]),
		ETag:         scalarResponseHeaderDTO(values["ETag"]),
		LastModified: scalarResponseHeaderDTO(values["Last-Modified"]),
		Date:         scalarResponseHeaderDTO(values["Date"]),
		Links:        copyStringSliceOrNil(values["Link"]),
		RetryAfter:   scalarResponseHeaderDTO(values["Retry-After"]),
	}
}

func rawEvidenceSnapshotEntityFromDTO(snapshot RawEvidenceSnapshotDTO) (domain.EvidenceSnapshot, error) {
	profile, err := domain.NewCollectorProfileVersion(snapshot.CollectorProfileVersion)
	if err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	headers, err := rawResponseHeadersEntityFromDTO(snapshot.ResponseHeaders)
	if err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	return domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Key: snapshot.EvidenceKey, Payload: append([]byte(nil), snapshot.Payload...),
		CollectorProfileVersion: profile, MIMEType: snapshot.MIMEType, StatusCode: snapshot.ResponseStatus,
		RequestedURL: snapshot.RequestedURL, FinalURL: snapshot.FinalURL,
		RedirectChain: append([]string(nil), snapshot.RedirectChain...), ResponseHeaders: headers,
		CapturedAt: snapshot.CapturedAt, PayloadSHA256: snapshot.PayloadSHA256,
	})
}

func rawEvidenceSnapshotDTOFromEntity(snapshot domain.EvidenceSnapshot) RawEvidenceSnapshotDTO {
	return RawEvidenceSnapshotDTO{
		EvidenceKey: snapshot.Key, Payload: append([]byte(nil), snapshot.Payload...),
		CollectorProfileVersion: snapshot.CollectorProfileVersion.String(), MIMEType: snapshot.MIMEType,
		ResponseStatus: snapshot.StatusCode, RequestedURL: snapshot.RequestedURL, FinalURL: snapshot.FinalURL,
		RedirectChain:   append([]string(nil), snapshot.RedirectChain...),
		ResponseHeaders: rawResponseHeadersDTOFromEntity(snapshot.ResponseHeaders), CapturedAt: snapshot.CapturedAt,
		PayloadSHA256: snapshot.PayloadSHA256,
	}
}

func rawEvidenceReferenceEntityFromDTO(reference RawEvidenceReferenceDTO) (domain.EvidenceReference, error) {
	entity := domain.EvidenceReference{
		SnapshotKey: reference.EvidenceKey, LocatorType: domain.EvidenceLocatorType(reference.LocatorType),
		LocatorValue: reference.LocatorValue, ByteStart: copyInt64(reference.ByteStart), ByteEnd: copyInt64(reference.ByteEnd),
		SelectedPayloadSHA256: reference.SelectedPayloadSHA256, SelectorVersion: reference.SelectorVersion,
	}
	if err := entity.Validate(); err != nil {
		return domain.EvidenceReference{}, err
	}
	return entity, nil
}

func rawEvidenceReferenceDTOFromEntity(reference domain.EvidenceReference) RawEvidenceReferenceDTO {
	return RawEvidenceReferenceDTO{
		EvidenceKey: reference.SnapshotKey, LocatorType: string(reference.LocatorType), LocatorValue: reference.LocatorValue,
		ByteStart: copyInt64(reference.ByteStart), ByteEnd: copyInt64(reference.ByteEnd),
		SelectedPayloadSHA256: reference.SelectedPayloadSHA256, SelectorVersion: reference.SelectorVersion,
	}
}

func rawEvidenceItemEntityFromDTO(item RawEvidenceItemDTO) (domain.SourceItem, error) {
	completeness := domain.EvidenceCompleteness(item.EvidenceCompleteness)
	if !completeness.Valid() {
		return domain.SourceItem{}, fmt.Errorf("source item evidence completeness is invalid")
	}
	attachments := make([]domain.SourceAttachment, len(item.Attachments))
	for index, attachment := range item.Attachments {
		attachments[index] = domain.SourceAttachment{
			URL: attachment.URL, MIMEType: attachment.MIMEType, SizeBytes: copyInt64(attachment.SizeBytes),
		}
	}
	references := make([]domain.EvidenceReference, len(item.EvidenceReferences))
	for index, reference := range item.EvidenceReferences {
		entity, err := rawEvidenceReferenceEntityFromDTO(reference)
		if err != nil {
			return domain.SourceItem{}, fmt.Errorf("map source item evidence reference: %w", err)
		}
		references[index] = entity
	}
	return domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: item.SourceCode, ExternalID: item.ExternalID, ParentExternalID: item.ParentExternalID,
		ContentType: item.ContentType, Title: item.Title, Body: item.Body, Language: item.Language,
		URL: item.URL, Author: item.Author, PublishedAt: copyTime(item.PublishedAt), ObservedAt: item.ObservedAt,
		EvidenceCompleteness: completeness, Attachments: attachments,
		Metrics: domain.SourceMetrics{
			ViewCount: copyInt64(item.Metrics.ViewCount), LikeCount: copyInt64(item.Metrics.LikeCount),
			CommentCount: copyInt64(item.Metrics.CommentCount), ShareCount: copyInt64(item.Metrics.ShareCount),
		},
		SnapshotKey: item.SnapshotKey, ItemLocator: item.ItemLocator, EvidenceReferences: references,
	})
}

func rawEvidenceItemDTOFromEntity(item domain.SourceItem) RawEvidenceItemDTO {
	attachments := make([]RawEvidenceAttachmentDTO, len(item.Attachments))
	for index, attachment := range item.Attachments {
		attachments[index] = RawEvidenceAttachmentDTO{
			URL: attachment.URL, MIMEType: attachment.MIMEType, SizeBytes: copyInt64(attachment.SizeBytes),
		}
	}
	references := make([]RawEvidenceReferenceDTO, len(item.EvidenceReferences))
	for index, reference := range item.EvidenceReferences {
		references[index] = rawEvidenceReferenceDTOFromEntity(reference)
	}
	return RawEvidenceItemDTO{
		SourceCode: item.SourceCode, ExternalID: item.ExternalID, ParentExternalID: item.ParentExternalID,
		ContentType: item.ContentType, Title: item.Title, Body: item.Body, Language: item.Language,
		URL: item.URL, Author: item.Author, PublishedAt: copyTime(item.PublishedAt), ObservedAt: item.ObservedAt,
		EvidenceCompleteness: string(item.EvidenceCompleteness), Attachments: attachments,
		Metrics: RawEvidenceMetricsDTO{
			ViewCount: copyInt64(item.Metrics.ViewCount), LikeCount: copyInt64(item.Metrics.LikeCount),
			CommentCount: copyInt64(item.Metrics.CommentCount), ShareCount: copyInt64(item.Metrics.ShareCount),
		},
		SnapshotKey: item.SnapshotKey, ItemLocator: item.ItemLocator, EvidenceReferences: references,
	}
}

func rawEvidenceFetchEntityFromDTO(fetch RawEvidenceFetchDTO) (domain.FetchResult, error) {
	items := make([]domain.SourceItem, len(fetch.Items))
	for index, item := range fetch.Items {
		entity, err := rawEvidenceItemEntityFromDTO(item)
		if err != nil {
			return domain.FetchResult{}, fmt.Errorf("map raw evidence item: %w", err)
		}
		items[index] = entity
	}
	snapshots := make([]domain.EvidenceSnapshot, len(fetch.Snapshots))
	for index, snapshot := range fetch.Snapshots {
		entity, err := rawEvidenceSnapshotEntityFromDTO(snapshot)
		if err != nil {
			return domain.FetchResult{}, fmt.Errorf("map raw evidence snapshot: %w", err)
		}
		snapshots[index] = entity
	}
	return domain.FetchResult{Items: items, Snapshots: snapshots}, nil
}

func rawEvidenceFetchDTOFromEntity(fetch domain.FetchResult) RawEvidenceFetchDTO {
	items := make([]RawEvidenceItemDTO, len(fetch.Items))
	for index, item := range fetch.Items {
		items[index] = rawEvidenceItemDTOFromEntity(item)
	}
	snapshots := make([]RawEvidenceSnapshotDTO, len(fetch.Snapshots))
	for index, snapshot := range fetch.Snapshots {
		snapshots[index] = rawEvidenceSnapshotDTOFromEntity(snapshot)
	}
	return RawEvidenceFetchDTO{Items: items, Snapshots: snapshots}
}

func evidenceSelectorInputDTOFromEntities(snapshot domain.EvidenceSnapshot, reference domain.EvidenceReference) EvidenceSelectorInputDTO {
	return EvidenceSelectorInputDTO{
		Snapshot: rawEvidenceSnapshotDTOFromEntity(snapshot), Reference: rawEvidenceReferenceDTOFromEntity(reference),
	}
}

func rawEvidenceRightsDecisionEntitiesFromDTO(decision RawEvidenceRightsDecisionDTO) (domain.RightsAction, domain.RightsState, error) {
	action := domain.RightsAction(decision.Action)
	state := domain.RightsState(decision.Decision)
	if !action.Valid() || !state.Valid() {
		return "", "", fmt.Errorf("raw evidence rights action or decision is invalid")
	}
	return action, state, nil
}

func evidenceLifecycleEntityFromString(value string) (domain.EvidenceLifecycleState, error) {
	state := domain.EvidenceLifecycleState(value)
	if !state.Valid() {
		return "", fmt.Errorf("raw evidence lifecycle state is invalid")
	}
	return state, nil
}

func appendScalarHeaderDTO(values map[string][]string, name string, value *string) {
	if value != nil {
		values[name] = []string{*value}
	}
}

func scalarResponseHeaderDTO(values []string) *string {
	if len(values) != 1 {
		return nil
	}
	value := values[0]
	return &value
}

func copyStringSliceOrNil(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
