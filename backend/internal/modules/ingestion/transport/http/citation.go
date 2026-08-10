package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type CitationRequestDTO struct {
	DocumentVersionID int64
}

type DocumentRequestDTO struct {
	DocumentVersionID int64
	IfNoneMatch       string
}

type CitationArtifactResponseDTO struct {
	ArtifactType             string                                `json:"artifact_type" enums:"markdown"`
	TransformerProfileSHA256 string                                `json:"transformer_profile_sha256"`
	MIMEType                 string                                `json:"mime_type" example:"text/markdown; charset=utf-8"`
	SHA256                   string                                `json:"sha256"`
	SizeBytes                int64                                 `json:"size_bytes"`
	ETag                     string                                `json:"etag"`
	AnchorMap                *CitationArtifactAnchorMapResponseDTO `json:"anchor_map" extensions:"x-nullable"`
}

type CitationArtifactAnchorBlockResponseDTO struct {
	Ordinal        int    `json:"ordinal"`
	MarkdownAnchor string `json:"markdown_anchor"`
}

type CitationArtifactAnchorMapResponseDTO struct {
	NormalizationVersion    string                                   `json:"normalization_version"`
	AnchorMapProfileVersion string                                   `json:"anchor_map_profile_version"`
	AnchorMapSHA256         string                                   `json:"anchor_map_sha256"`
	Blocks                  []CitationArtifactAnchorBlockResponseDTO `json:"blocks"`
}

type CitationAnchorMapResponseDTO struct {
	NormalizationVersion string `json:"normalization_version"`
	AnchorMapVersion     string `json:"anchor_map_version"`
	MarkdownAnchor       string `json:"markdown_anchor"`
}

type CitationPartyResponseDTO struct {
	Role              string  `json:"role" enums:"publisher,author,distributor,content_origin"`
	Kind              string  `json:"kind" enums:"organization,person,account"`
	IdentityNamespace string  `json:"identity_namespace"`
	ExternalID        string  `json:"external_id"`
	DisplayName       string  `json:"display_name"`
	HomepageURL       *string `json:"homepage_url" extensions:"x-nullable"`
}

// CitationResponseDTO is an explicit transport allowlist. Internal storage
// and rights-decision identities are intentionally not representable.
type CitationResponseDTO struct {
	DocumentID        int64                      `json:"document_id"`
	DocumentVersionID int64                      `json:"document_version_id"`
	SourceType        string                     `json:"source_type"`
	SourceName        string                     `json:"source_name"`
	Title             string                     `json:"title"`
	Author            *string                    `json:"author" extensions:"x-nullable"`
	Publisher         *string                    `json:"publisher" extensions:"x-nullable"`
	PublisherParty    *CitationPartyResponseDTO  `json:"publisher_party" extensions:"x-nullable"`
	ContentOrigin     *CitationPartyResponseDTO  `json:"content_origin" extensions:"x-nullable"`
	Distributors      []CitationPartyResponseDTO `json:"distributors"`

	PublisherAvailability          string  `json:"publisher_availability" enums:"available,unavailable"`
	PublisherUnavailableReason     *string `json:"publisher_unavailable_reason" extensions:"x-nullable"`
	ContentOriginAvailability      string  `json:"content_origin_availability" enums:"available,unavailable"`
	ContentOriginUnavailableReason *string `json:"content_origin_unavailable_reason" extensions:"x-nullable"`
	SourceRecordURL                *string `json:"source_record_url" extensions:"x-nullable"`
	CanonicalURL                   *string `json:"canonical_url" extensions:"x-nullable"`
	DiscussionURL                  *string `json:"discussion_url" extensions:"x-nullable"`

	BodyOrigin    string     `json:"body_origin"`
	Completeness  string     `json:"completeness"`
	Language      string     `json:"language"`
	PublishedAt   *time.Time `json:"published_at" extensions:"x-nullable"`
	CapturedAt    time.Time  `json:"captured_at"`
	ContentSHA256 *string    `json:"content_sha256" extensions:"x-nullable"`

	Availability      string                       `json:"availability" enums:"full_archive,partial_archive,summary_only,metadata_only,policy_blocked,temporarily_unavailable,quarantined,tombstoned"`
	UnavailableReason *string                      `json:"unavailable_reason" extensions:"x-nullable"`
	Artifact          *CitationArtifactResponseDTO `json:"artifact" extensions:"x-nullable"`

	LocatorAvailability      string                        `json:"locator_availability" enums:"available,unavailable"`
	LocatorUnavailableReason *string                       `json:"locator_unavailable_reason" extensions:"x-nullable"`
	ExactQuote               *string                       `json:"exact_quote" extensions:"x-nullable"`
	UTF8ByteStart            *int64                        `json:"utf8_byte_start" extensions:"x-nullable"`
	UTF8ByteEnd              *int64                        `json:"utf8_byte_end" extensions:"x-nullable"`
	AnchorMap                *CitationAnchorMapResponseDTO `json:"anchor_map" extensions:"x-nullable"`
}

type VersionedDocumentResponseDTO struct {
	Citation CitationResponseDTO `json:"citation"`
	Markdown string              `json:"markdown"`
	ETag     string              `json:"etag"`
}

type versionedCitationHTTPService interface {
	GetCitation(context.Context, ingestionapplication.CitationQuery) (ingestionapplication.CitationResult, error)
	GetDocument(context.Context, ingestionapplication.DocumentQuery) (ingestionapplication.DocumentResult, error)
}

type VersionedCitationHandler struct{ service versionedCitationHTTPService }

func NewVersionedCitationHandler(service versionedCitationHTTPService) *VersionedCitationHandler {
	return &VersionedCitationHandler{service: service}
}

func RegisterCitationRoutes(router *gin.Engine, service versionedCitationHTTPService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewVersionedCitationHandler(service)
	versions := router.Group("/api/v1/document-versions", httptransport.RequireAuthentication(authenticator))
	versions.GET("/:id/citation", httptransport.Wrap(handler.Citation))
	versions.GET("/:id/document", httptransport.Wrap(handler.Document))
}

// Citation returns provenance and an explicit availability state for one
// immutable DocumentVersion. Unavailable publisher/locator facts remain null.
// @Summary Get an exact document-version citation
// @Tags document-versions
// @Produce json
// @Security BearerAuth
// @Param id path int true "document version ID"
// @Success 200 {object} ContentResult[CitationResponseDTO]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 502 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/document-versions/{id}/citation [get]
func (handler *VersionedCitationHandler) Citation(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	request, err := citationRequestDTO(c)
	if err != nil {
		return err
	}
	result, err := handler.service.GetCitation(c.Request.Context(), ingestionapplication.CitationQuery{DocumentVersionID: request.DocumentVersionID})
	if err != nil {
		return versionedCitationHTTPError(err)
	}
	c.Header("Cache-Control", "private, no-store")
	httptransport.OK(c, citationResponseDTO(result.Citation))
	return nil
}

// Document returns verified Markdown for one immutable DocumentVersion. The
// strong ETag is the active Markdown artifact SHA-256 and is checked only after
// rights and Vault integrity have been revalidated.
// @Summary Get an exact document-version Markdown projection
// @Tags document-versions
// @Produce json
// @Security BearerAuth
// @Param id path int true "document version ID"
// @Param If-None-Match header string false "strong artifact SHA-256 ETag"
// @Success 200 {object} ContentResult[VersionedDocumentResponseDTO]
// @Success 304
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 409 {object} ContentResult[EmptyResponse]
// @Failure 502 {object} ContentResult[EmptyResponse]
// @Failure 503 {object} ContentResult[EmptyResponse]
// @Router /api/v1/document-versions/{id}/document [get]
func (handler *VersionedCitationHandler) Document(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	request, err := documentRequestDTO(c)
	if err != nil {
		return err
	}
	result, err := handler.service.GetDocument(c.Request.Context(), ingestionapplication.DocumentQuery{
		DocumentVersionID: request.DocumentVersionID, IfNoneMatch: request.IfNoneMatch,
	})
	if err != nil {
		return versionedCitationHTTPError(err)
	}
	c.Header("Cache-Control", "private, no-cache")
	c.Header("ETag", result.ETag)
	if result.NotModified {
		c.Status(stdhttp.StatusNotModified)
		return nil
	}
	httptransport.OK(c, VersionedDocumentResponseDTO{
		Citation: citationResponseDTO(result.Citation), Markdown: result.Markdown, ETag: result.ETag,
	})
	return nil
}

func citationRequestDTO(c *gin.Context) (CitationRequestDTO, error) {
	id, err := versionedDocumentID(c)
	if err != nil {
		return CitationRequestDTO{}, err
	}
	return CitationRequestDTO{DocumentVersionID: id}, nil
}

func documentRequestDTO(c *gin.Context) (DocumentRequestDTO, error) {
	id, err := versionedDocumentID(c)
	if err != nil {
		return DocumentRequestDTO{}, err
	}
	ifNoneMatch := c.GetHeader("If-None-Match")
	if !validStrongArtifactETag(ifNoneMatch) {
		return DocumentRequestDTO{}, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid If-None-Match")
	}
	return DocumentRequestDTO{DocumentVersionID: id, IfNoneMatch: ifNoneMatch}, nil
}

func versionedDocumentID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid document version id")
	}
	return id, nil
}

func validStrongArtifactETag(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 66 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	for _, character := range value[1 : len(value)-1] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func citationResponseDTO(value ingestionapplication.CitationDTO) CitationResponseDTO {
	response := CitationResponseDTO{
		DocumentID: value.DocumentID, DocumentVersionID: value.DocumentVersionID,
		SourceType: value.SourceType, SourceName: value.SourceName, Title: value.Title,
		Author: value.Author, Publisher: value.Publisher,
		PublisherParty: citationPartyResponseDTO(value.PublisherParty), ContentOrigin: citationPartyResponseDTO(value.ContentOrigin),
		Distributors:                   citationPartyResponseDTOs(value.Distributors),
		PublisherAvailability:          string(value.PublisherAvailability),
		PublisherUnavailableReason:     citationReasonPointer(value.PublisherUnavailableReason),
		ContentOriginAvailability:      string(value.ContentOriginAvailability),
		ContentOriginUnavailableReason: citationReasonPointer(value.ContentOriginUnavailableReason),
		SourceRecordURL:                value.SourceRecordURL, CanonicalURL: value.CanonicalURL, DiscussionURL: value.DiscussionURL,
		BodyOrigin: string(value.BodyOrigin), Completeness: string(value.Completeness), Language: value.Language,
		PublishedAt: value.PublishedAt, CapturedAt: value.CapturedAt, ContentSHA256: value.ContentSHA256,
		Availability: string(value.Availability), UnavailableReason: citationReasonPointer(value.UnavailableReason),
		LocatorAvailability:      string(value.LocatorAvailability),
		LocatorUnavailableReason: citationReasonPointer(value.LocatorUnavailableReason),
		ExactQuote:               value.ExactQuote, UTF8ByteStart: value.UTF8ByteStart, UTF8ByteEnd: value.UTF8ByteEnd,
	}
	if value.Artifact != nil {
		response.Artifact = &CitationArtifactResponseDTO{
			ArtifactType: value.Artifact.ArtifactType, TransformerProfileSHA256: value.Artifact.TransformerProfileSHA256,
			MIMEType: value.Artifact.MIMEType, SHA256: value.Artifact.SHA256,
			SizeBytes: value.Artifact.SizeBytes, ETag: value.Artifact.ETag,
		}
		if value.Artifact.AnchorMap != nil {
			response.Artifact.AnchorMap = &CitationArtifactAnchorMapResponseDTO{
				NormalizationVersion:    value.Artifact.AnchorMap.NormalizationVersion,
				AnchorMapProfileVersion: value.Artifact.AnchorMap.AnchorMapProfileVersion,
				AnchorMapSHA256:         value.Artifact.AnchorMap.AnchorMapSHA256,
				Blocks:                  make([]CitationArtifactAnchorBlockResponseDTO, len(value.Artifact.AnchorMap.Blocks)),
			}
			for index, block := range value.Artifact.AnchorMap.Blocks {
				response.Artifact.AnchorMap.Blocks[index] = CitationArtifactAnchorBlockResponseDTO{Ordinal: block.Ordinal, MarkdownAnchor: block.MarkdownAnchor}
			}
		}
	}
	if value.AnchorMap != nil {
		response.AnchorMap = &CitationAnchorMapResponseDTO{
			NormalizationVersion: value.AnchorMap.NormalizationVersion,
			AnchorMapVersion:     value.AnchorMap.AnchorMapVersion, MarkdownAnchor: value.AnchorMap.MarkdownAnchor,
		}
	}
	return response
}

func citationPartyResponseDTO(value *ingestionapplication.CitationPartyDTO) *CitationPartyResponseDTO {
	if value == nil {
		return nil
	}
	return &CitationPartyResponseDTO{
		Role: value.Role, Kind: value.Kind, IdentityNamespace: value.IdentityNamespace,
		ExternalID: value.ExternalID, DisplayName: value.DisplayName, HomepageURL: value.HomepageURL,
	}
}

func citationPartyResponseDTOs(values []ingestionapplication.CitationPartyDTO) []CitationPartyResponseDTO {
	result := make([]CitationPartyResponseDTO, len(values))
	for index := range values {
		result[index] = *citationPartyResponseDTO(&values[index])
	}
	return result
}

func citationReasonPointer(value ingestionapplication.CitationUnavailableReason) *string {
	if value == "" {
		return nil
	}
	result := string(value)
	return &result
}

func versionedCitationHTTPError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	var readError *ingestionapplication.DocumentReadError
	if errors.As(err, &readError) {
		switch readError.Kind {
		case ingestionapplication.DocumentReadFailureNotReadable:
			return sharederrors.Wrap(sharederrors.CodeConflict, stdhttp.StatusConflict, "document is not readable", err)
		case ingestionapplication.DocumentReadFailurePolicy:
			return sharederrors.Wrap(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "document policy blocked", err)
		case ingestionapplication.DocumentReadFailurePermission:
			return sharederrors.Wrap(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "document permission denied", err)
		case ingestionapplication.DocumentReadFailureRetention:
			return sharederrors.Wrap(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "document retention unavailable", err)
		case ingestionapplication.DocumentReadFailureMissing:
			return sharederrors.Wrap(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "document artifact not found", err)
		case ingestionapplication.DocumentReadFailureIntegrity:
			return sharederrors.Wrap(sharederrors.CodeBadGateway, stdhttp.StatusBadGateway, "document projection integrity failure", err)
		case ingestionapplication.DocumentReadFailureUnavailable:
			return sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "document projection unavailable", err)
		}
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid document version request", err)
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.Wrap(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "document version not found", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "document version service unavailable", err)
	default:
		return fmt.Errorf("read versioned citation: %w", err)
	}
}
