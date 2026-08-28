package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	searchapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/application"
	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type searchService interface {
	Search(context.Context, searchapplication.Request) (searchapplication.Result, error)
}

type Handler struct{ service searchService }

func NewHandler(service searchService) *Handler { return &Handler{service: service} }

// List godoc
//
// @Summary Search current PostgreSQL content, event and knowledge projections
// @Tags search
// @Produce json
// @Security BearerAuth
// @Param q query string true "lexical keyword" maxlength(100)
// @Param types query string false "comma-separated content,event,knowledge"
// @Param source_connection_id query int false "source connection filter" minimum(1)
// @Param monitor_id query int false "monitor filter" minimum(1)
// @Param entity query string false "exact normalized entity" maxlength(128)
// @Param status query string false "resource status"
// @Param sort query string false "relevance or latest" Enums(relevance,latest)
// @Param from query string false "inclusive RFC3339 start"
// @Param to query string false "inclusive RFC3339 end"
// @Param limit query int false "result limit" minimum(1) maximum(100)
// @Param cursor query string false "opaque signed search snapshot cursor"
// @Success 200 {object} SearchResult[SearchPageResponseDTO]
// @Failure 400 {object} SearchResult[SearchEmptyResponseDTO]
// @Failure 401 {object} SearchResult[SearchEmptyResponseDTO]
// @Failure 403 {object} SearchResult[SearchEmptyResponseDTO]
// @Failure 503 {object} SearchResult[SearchEmptyResponseDTO]
// @Router /api/v1/search [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "search")
	c.Header("Cache-Control", "private, no-store")
	if handler == nil || handler.service == nil {
		return searchError(searchapplication.ErrUnavailable)
	}
	query, err := searchQuery(c)
	if err != nil {
		return err
	}
	subject, found := httptransport.SubjectFromContext(c)
	if !found {
		return sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	result, err := handler.service.Search(c.Request.Context(), searchapplication.Request{
		Query: query, Subject: searchapplication.Subject{UserID: subject.UserID, Role: string(subject.Role)},
		Cursor: strings.TrimSpace(c.Query("cursor")),
	})
	if err != nil {
		return searchError(err)
	}
	httptransport.OK(c, searchPageResponse(result))
	return nil
}

func searchQuery(c *gin.Context) (searchdomain.Query, error) {
	query := searchdomain.Query{
		Keyword: c.Query("q"), Entity: c.Query("entity"), Status: c.Query("status"), Sort: c.Query("sort"),
	}
	var err error
	if query.Types, err = searchTypes(c.QueryArray("types")); err != nil {
		return searchdomain.Query{}, invalidSearchRequest()
	}
	if query.SourceConnectionID, err = searchOptionalID(c.Query("source_connection_id")); err != nil {
		return searchdomain.Query{}, invalidSearchRequest()
	}
	if query.MonitorID, err = searchOptionalID(c.Query("monitor_id")); err != nil {
		return searchdomain.Query{}, invalidSearchRequest()
	}
	if query.From, err = searchOptionalTime(c.Query("from")); err != nil {
		return searchdomain.Query{}, invalidSearchRequest()
	}
	if query.To, err = searchOptionalTime(c.Query("to")); err != nil {
		return searchdomain.Query{}, invalidSearchRequest()
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		query.Limit, err = strconv.Atoi(raw)
		if err != nil {
			return searchdomain.Query{}, invalidSearchRequest()
		}
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil {
		return searchdomain.Query{}, invalidSearchRequest()
	}
	return query, nil
}

func searchTypes(values []string) ([]searchdomain.ResourceType, error) {
	var result []searchdomain.ResourceType
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			resourceType := searchdomain.ResourceType(part)
			if !resourceType.Valid() {
				return nil, errors.New("invalid resource type")
			}
			result = append(result, resourceType)
		}
	}
	return result, nil
}

func searchOptionalID(raw string) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, errors.New("invalid id")
	}
	return &value, nil
}

func searchOptionalTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	value = value.UTC()
	return &value, nil
}

func invalidSearchRequest() error {
	return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid search request")
}

func searchError(err error) error {
	switch {
	case errors.Is(err, searchapplication.ErrInvalidQuery):
		return invalidSearchRequest()
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "search service unavailable")
	}
}
