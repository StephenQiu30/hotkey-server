package http

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type LocateTextQuoteSelectorRequestDTO struct {
	ExactQuote           string `json:"exact_quote" binding:"required,max=4096"`
	PlaintextSHA256      string `json:"plaintext_sha256" binding:"required,len=64"`
	NormalizationVersion string `json:"normalization_version" binding:"required"`
}

// TextQuoteSelectorResponseDTO deliberately excludes rights decisions and
// artifact locators. It exposes only the immutable citation selector required
// by an editor to attach ClaimEvidence.
type TextQuoteSelectorResponseDTO struct {
	ID                   int64     `json:"id"`
	Version              int64     `json:"version"`
	DocumentVersionID    int64     `json:"document_version_id"`
	ExactQuote           string    `json:"exact_quote"`
	Prefix               string    `json:"prefix"`
	Suffix               string    `json:"suffix"`
	UTF8ByteStart        int64     `json:"utf8_byte_start"`
	UTF8ByteEnd          int64     `json:"utf8_byte_end"`
	QuoteSHA256          string    `json:"quote_sha256"`
	PlaintextSHA256      string    `json:"plaintext_sha256"`
	NormalizationVersion string    `json:"normalization_version"`
	SelectorVersion      string    `json:"selector_version"`
	MarkdownAnchor       *string   `json:"markdown_anchor" extensions:"x-nullable"`
	RetentionUntil       time.Time `json:"retention_until"`
}

type TextQuoteSelectorHandler struct {
	service *ingestionapplication.TextQuoteSelectorService
}

func NewTextQuoteSelectorHandler(service *ingestionapplication.TextQuoteSelectorService) *TextQuoteSelectorHandler {
	return &TextQuoteSelectorHandler{service: service}
}

func RegisterTextQuoteSelectorRoutes(router *gin.Engine, service *ingestionapplication.TextQuoteSelectorService, authenticator httptransport.Authenticator) {
	if router == nil || service == nil {
		return
	}
	handler := NewTextQuoteSelectorHandler(service)
	versions := router.Group("/api/v1/document-versions", httptransport.RequireAuthentication(authenticator),
		httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	versions.POST("/:id/text-quote-selectors", httptransport.Wrap(handler.Locate))
}

// Locate derives exact UTF-8 byte offsets and W3C context from the immutable
// plaintext projection. Ambiguous or missing excerpts are rejected.
// @Summary Create an exact document-version quote selector
// @Tags document-versions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "document version ID"
// @Param If-Match header string true "strong plaintext SHA-256 ETag"
// @Param Idempotency-Key header string true "request idempotency key"
// @Param request body LocateTextQuoteSelectorRequestDTO true "unique exact excerpt"
// @Success 201 {object} ContentResult[TextQuoteSelectorResponseDTO]
// @Failure 400 {object} ContentResult[EmptyResponse]
// @Failure 401 {object} ContentResult[EmptyResponse]
// @Failure 403 {object} ContentResult[EmptyResponse]
// @Failure 404 {object} ContentResult[EmptyResponse]
// @Failure 409 {object} ContentResult[EmptyResponse]
// @Router /api/v1/document-versions/{id}/text-quote-selectors [post]
func (handler *TextQuoteSelectorHandler) Locate(c *gin.Context) error {
	httptransport.SetModule(c, "ingestion")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	var request LocateTextQuoteSelectorRequestDTO
	if err := c.ShouldBindJSON(&request); err != nil {
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	request.ExactQuote = strings.TrimSpace(request.ExactQuote)
	request.PlaintextSHA256 = strings.TrimSpace(request.PlaintextSHA256)
	request.NormalizationVersion = strings.TrimSpace(request.NormalizationVersion)
	if !strongTextQuoteETagMatches(c, request.PlaintextSHA256) || !validTextQuoteIdempotencyKey(c) {
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	result, err := handler.service.LocateAndCreate(c.Request.Context(), ingestionapplication.LocateTextQuoteSelectorCommand{
		DocumentVersionID: id, ExactQuote: request.ExactQuote, PlaintextSHA256: request.PlaintextSHA256,
		NormalizationVersion: request.NormalizationVersion, DecisionAt: time.Now().UTC(),
	})
	if err != nil {
		return versionedCitationHTTPError(err)
	}
	selector := result.Selector
	c.Header("Cache-Control", "private, no-store")
	httptransport.Created(c, TextQuoteSelectorResponseDTO{ID: selector.ID, Version: selector.Version,
		DocumentVersionID: selector.DocumentVersionID, ExactQuote: selector.ExactQuote, Prefix: selector.Prefix,
		Suffix: selector.Suffix, UTF8ByteStart: selector.UTF8ByteStart, UTF8ByteEnd: selector.UTF8ByteEnd,
		QuoteSHA256: selector.QuoteSHA256, PlaintextSHA256: selector.PlaintextSHA256,
		NormalizationVersion: selector.NormalizationVersion, SelectorVersion: selector.SelectorVersion,
		MarkdownAnchor: selector.MarkdownAnchor, RetentionUntil: selector.RetentionUntil})
	return nil
}

func strongTextQuoteETagMatches(c *gin.Context, digest string) bool {
	values := c.Request.Header.Values("If-Match")
	return len(values) == 1 && values[0] == fmt.Sprintf("\"%s\"", digest)
}

func validTextQuoteIdempotencyKey(c *gin.Context) bool {
	values := c.Request.Header.Values("Idempotency-Key")
	if len(values) != 1 {
		return false
	}
	value := strings.TrimSpace(values[0])
	return value != "" && len(value) <= 96 && !strings.ContainsAny(value, "\r\n")
}
