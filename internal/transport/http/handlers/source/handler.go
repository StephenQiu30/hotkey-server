package source

import (
	"errors"
	"net/http"
	"time"

	platformfetcher "github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service  *servicesource.Service
	fetchers map[servicesource.SourceType]platformfetcher.Fetcher
}

type sourceRequest struct {
	Name             string   `json:"name"`
	Type             string   `json:"type"`
	URL              string   `json:"url"`
	ComplianceNote   string   `json:"complianceNote"`
	FetchIntervalMin int      `json:"fetchIntervalMin"`
	RateLimitPerHour int      `json:"rateLimitPerHour"`
	ChannelIDs       []string `json:"channelIDs"`
}

type statusRequest struct {
	Status string `json:"status"`
}

func New(service *servicesource.Service, fetcherMaps ...map[servicesource.SourceType]platformfetcher.Fetcher) *Handler {
	fetchers := map[servicesource.SourceType]platformfetcher.Fetcher{
		servicesource.SourceTypeRSS:        platformfetcher.NewRSSFetcher(nil),
		servicesource.SourceTypePublicPage: platformfetcher.NewPublicPageFetcher(nil),
	}
	if len(fetcherMaps) > 0 && fetcherMaps[0] != nil {
		fetchers = fetcherMaps[0]
	}
	return &Handler{
		service:  service,
		fetchers: fetchers,
	}
}

func (h *Handler) ListSources(c *gin.Context) {
	sources, err := h.service.ListSources(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sources": sourceResponses(sources)})
}

func (h *Handler) CreateSource(c *gin.Context) {
	var req sourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	source, err := h.service.CreateSource(c.Request.Context(), servicesource.CreateSourceInput{
		Name:             req.Name,
		Type:             servicesource.SourceType(req.Type),
		URL:              req.URL,
		ComplianceNote:   req.ComplianceNote,
		FetchIntervalMin: req.FetchIntervalMin,
		RateLimitPerHour: req.RateLimitPerHour,
		ChannelIDs:       req.ChannelIDs,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"source": sourceResponse(source)})
}

func (h *Handler) UpdateSource(c *gin.Context) {
	var req sourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	source, err := h.service.UpdateSource(c.Request.Context(), servicesource.UpdateSourceInput{
		SourceID:         c.Param("sourceID"),
		Name:             req.Name,
		Type:             servicesource.SourceType(req.Type),
		URL:              req.URL,
		ComplianceNote:   req.ComplianceNote,
		FetchIntervalMin: req.FetchIntervalMin,
		RateLimitPerHour: req.RateLimitPerHour,
		ChannelIDs:       req.ChannelIDs,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"source": sourceResponse(source)})
}

func (h *Handler) SetSourceStatus(c *gin.Context) {
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	source, err := h.service.SetSourceStatus(c.Request.Context(), servicesource.SetSourceStatusInput{
		SourceID: c.Param("sourceID"),
		Status:   servicesource.SourceStatus(req.Status),
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"source": sourceResponse(source)})
}

func (h *Handler) ListCollectionRuns(c *gin.Context) {
	runs, err := h.service.ListCollectionRuns(c.Request.Context(), c.Param("sourceID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"collectionRuns": collectionRunResponses(runs)})
}

func (h *Handler) TestFetch(c *gin.Context) {
	ctx := c.Request.Context()
	source, err := h.service.SourceByID(ctx, c.Param("sourceID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	fetcher, exists := h.fetchers[source.Type]
	if !exists {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	startedAt := time.Now().UTC()
	items, fetchErr := fetcher.Fetch(ctx, platformfetcher.Source{
		ID:             source.ID,
		Type:           platformfetcher.SourceType(source.Type),
		URL:            source.URL,
		ComplianceNote: source.ComplianceNote,
	})
	finishedAt := time.Now().UTC()
	input := servicesource.RecordCollectionRunInput{
		SourceID:   source.ID,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}
	if fetchErr != nil {
		input.Status = servicesource.CollectionRunStatusFailed
		input.Error = fetchErr.Error()
	} else {
		input.Status = servicesource.CollectionRunStatusSuccess
		input.ItemsFetched = len(items)
	}
	run, err := h.service.RecordCollectionRun(ctx, input)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if fetchErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"collectionRun": collectionRunResponse(run)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"collectionRun": collectionRunResponse(run)})
}

func sourceResponse(source servicesource.Source) gin.H {
	response := gin.H{
		"id":               source.ID,
		"name":             source.Name,
		"type":             source.Type,
		"url":              source.URL,
		"status":           source.Status,
		"complianceNote":   source.ComplianceNote,
		"fetchIntervalMin": source.FetchIntervalMin,
		"rateLimitPerHour": source.RateLimitPerHour,
		"channelIDs":       source.ChannelIDs,
		"lastError":        source.LastError,
	}
	if source.LastCollectedAt != nil {
		response["lastCollectedAt"] = source.LastCollectedAt
	}
	return response
}

func sourceResponses(sources []servicesource.Source) []gin.H {
	responses := make([]gin.H, 0, len(sources))
	for _, source := range sources {
		responses = append(responses, sourceResponse(source))
	}
	return responses
}

func collectionRunResponses(runs []servicesource.CollectionRun) []gin.H {
	responses := make([]gin.H, 0, len(runs))
	for _, run := range runs {
		responses = append(responses, collectionRunResponse(run))
	}
	return responses
}

func collectionRunResponse(run servicesource.CollectionRun) gin.H {
	return gin.H{
		"id":           run.ID,
		"sourceID":     run.SourceID,
		"status":       run.Status,
		"itemsFetched": run.ItemsFetched,
		"error":        run.Error,
		"startedAt":    run.StartedAt,
		"finishedAt":   run.FinishedAt,
	}
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, servicesource.ErrComplianceNoteRequired):
		writeError(c, http.StatusBadRequest, "compliance_note_required", "compliance note required")
	case errors.Is(err, servicesource.ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, servicesource.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, servicesource.ErrAlreadyExists):
		writeError(c, http.StatusConflict, "source_already_exists", "source already exists")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
