package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/event"
	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/StephenQiu30/hotkey-server/internal/openapi"
	"github.com/StephenQiu30/hotkey-server/internal/source"
	"github.com/StephenQiu30/hotkey-server/internal/trust"
	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	return NewRouterWithServices(keyword.NewService(), source.NewService(), content.NewService(), event.NewService(event.Options{VectorEnabled: true}))
}

func NewRouterWithKeywordService(keywordService *keyword.Service) *gin.Engine {
	return NewRouterWithServices(keywordService, source.NewService(), content.NewService(), event.NewService(event.Options{VectorEnabled: true}))
}

func NewRouterWithServices(keywordService *keyword.Service, sourceService *source.Service, contentService *content.Service, eventServices ...*event.Service) *gin.Engine {
	if contentService == nil {
		contentService = content.NewService()
	}
	eventService := event.NewService(event.Options{VectorEnabled: true})
	if len(eventServices) > 0 && eventServices[0] != nil {
		eventService = eventServices[0]
	}
	trustService := trust.NewService()

	return newRouter(keywordService, sourceService, contentService, eventService, trustService)
}

func newRouter(keywordService *keyword.Service, sourceService *source.Service, contentService *content.Service, eventService *event.Service, trustService *trust.Service) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", handleHealth)
	router.GET("/openapi.json", handleOpenAPI)
	router.GET("/api/v1/admin/keywords", listPlatformKeywords(keywordService))
	router.POST("/api/v1/admin/keywords", createPlatformKeyword(keywordService))
	router.PATCH("/api/v1/admin/keywords/:id", setPlatformKeywordEnabled(keywordService))
	router.GET("/api/v1/admin/sources", listSources(sourceService))
	router.PATCH("/api/v1/admin/sources/:id", updateSource(sourceService))
	router.GET("/api/v1/admin/source-items", listSourceItems(contentService))
	router.POST("/api/v1/admin/source-items", ingestSourceItem(contentService))
	router.GET("/api/v1/admin/event-clusters", listEventClusters(eventService))
	router.POST("/api/v1/admin/event-candidates", upsertEventCandidate(eventService))
	router.POST("/api/v1/admin/event-evidence", addEventEvidence(trustService))
	router.POST("/api/v1/admin/events/:id/ai-summary", setEventAISummary(trustService))
	router.GET("/api/v1/events/:id/evidence", getEventEvidence(trustService))
	router.POST("/api/v1/keywords/follow", followKeyword(keywordService))
	router.POST("/api/v1/keywords/block", blockKeyword(keywordService))
	router.POST("/api/v1/keywords/additional", addUserKeyword(keywordService))
	router.GET("/api/v1/keywords/preferences", getUserPreferences(keywordService))
	return router
}

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "hotkey-server",
	})
}

func handleOpenAPI(c *gin.Context) {
	c.JSON(http.StatusOK, openapi.Spec())
}

type createKeywordRequest struct {
	Term     string `json:"term"`
	Category string `json:"category"`
}

type updateKeywordRequest struct {
	Enabled *bool `json:"enabled"`
}

type updateSourceRequest struct {
	Enabled          *bool `json:"enabled"`
	RateLimitPerHour *int  `json:"rateLimitPerHour"`
}

type ingestSourceItemRequest struct {
	SourceID    string            `json:"sourceId"`
	OriginalURL string            `json:"originalUrl"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	PublishedAt time.Time         `json:"publishedAt"`
	FetchedAt   time.Time         `json:"fetchedAt"`
	RawMetadata map[string]string `json:"rawMetadata"`
}

type eventCandidateRequest struct {
	SourceItemID string    `json:"sourceItemId"`
	Title        string    `json:"title"`
	ContentHash  string    `json:"contentHash"`
	Vector       []float64 `json:"vector"`
}

type eventEvidenceRequest struct {
	EventID      string `json:"eventId"`
	SourceID     string `json:"sourceId"`
	SourceItemID string `json:"sourceItemId"`
	Layer        string `json:"layer"`
	TrustLevel   string `json:"trustLevel"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	HeatWeight   int    `json:"heatWeight"`
	RiskNote     string `json:"riskNote"`
}

type aiSummaryRequest struct {
	Summary     string   `json:"summary"`
	CitationIDs []string `json:"citationIds"`
}

type userKeywordRequest struct {
	UserID string `json:"userId"`
	Term   string `json:"term"`
}

func listPlatformKeywords(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"keywords": service.ListPlatformKeywords()})
	}
}

func createPlatformKeyword(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createKeywordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		created, err := service.CreatePlatformKeyword(keyword.CreatePlatformKeywordInput{
			Term:     req.Term,
			Category: req.Category,
		})
		if err != nil {
			writeKeywordError(c, err)
			return
		}
		c.JSON(http.StatusCreated, created)
	}
}

func setPlatformKeywordEnabled(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req updateKeywordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		if req.Enabled == nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "enabled is required")
			return
		}
		updated, err := service.SetPlatformKeywordEnabled(c.Param("id"), *req.Enabled)
		if err != nil {
			writeKeywordError(c, err)
			return
		}
		c.JSON(http.StatusOK, updated)
	}
}

func listSources(service *source.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"sources": service.ListSources()})
	}
}

func updateSource(service *source.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req updateSourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		updated, err := service.UpdateSourceConfig(c.Param("id"), source.UpdateSourceConfigInput{
			Enabled:          req.Enabled,
			RateLimitPerHour: req.RateLimitPerHour,
		})
		if err != nil {
			writeSourceError(c, err)
			return
		}
		c.JSON(http.StatusOK, updated)
	}
}

func listSourceItems(service *content.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"items": service.ListSourceItems()})
	}
}

func ingestSourceItem(service *content.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ingestSourceItemRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		item, result, err := service.IngestSourceItem(content.IngestSourceItemInput{
			SourceID:    req.SourceID,
			OriginalURL: req.OriginalURL,
			Title:       req.Title,
			Summary:     req.Summary,
			PublishedAt: req.PublishedAt,
			FetchedAt:   req.FetchedAt,
			RawMetadata: req.RawMetadata,
		})
		if err != nil {
			writeContentError(c, err)
			return
		}
		status := http.StatusCreated
		if result == content.ResultDuplicate {
			status = http.StatusOK
		}
		c.JSON(status, gin.H{
			"result": result,
			"item":   item,
		})
	}
}

func listEventClusters(service *event.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"clusters": service.ListClusters()})
	}
}

func upsertEventCandidate(service *event.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req eventCandidateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		match, err := service.UpsertCandidate(event.CandidateInput{
			SourceItemID: req.SourceItemID,
			Title:        req.Title,
			ContentHash:  req.ContentHash,
			Vector:       req.Vector,
		})
		if err != nil {
			writeEventError(c, err)
			return
		}
		status := http.StatusOK
		if match.MatchMethod == event.MatchMethodSeed {
			status = http.StatusCreated
		}
		c.JSON(status, match)
	}
}

func addEventEvidence(service *trust.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req eventEvidenceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		if err := service.AddEvidence(trust.EvidenceInput{
			EventID:      req.EventID,
			SourceID:     req.SourceID,
			SourceItemID: req.SourceItemID,
			Layer:        req.Layer,
			TrustLevel:   req.TrustLevel,
			Title:        req.Title,
			URL:          req.URL,
			HeatWeight:   req.HeatWeight,
			RiskNote:     req.RiskNote,
		}); err != nil {
			writeTrustError(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	}
}

func setEventAISummary(service *trust.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req aiSummaryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		if err := service.SetAISummary(c.Param("id"), trust.AISummaryInput{
			Summary:     req.Summary,
			CitationIDs: req.CitationIDs,
		}); err != nil {
			writeTrustError(c, err)
			return
		}
		c.JSON(http.StatusOK, service.GetEventEvidence(c.Param("id")))
	}
}

func getEventEvidence(service *trust.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, service.GetEventEvidence(c.Param("id")))
	}
}

func followKeyword(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		handleUserKeywordMutation(c, service.FollowKeyword)
	}
}

func blockKeyword(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		handleUserKeywordMutation(c, service.BlockKeyword)
	}
}

func addUserKeyword(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		handleUserKeywordMutation(c, service.AddUserKeyword)
	}
}

func getUserPreferences(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Query("userId")
		if userID == "" {
			writeError(c, http.StatusBadRequest, "invalid_user", "userId is required")
			return
		}
		c.JSON(http.StatusOK, service.GetUserPreferences(userID))
	}
}

func handleUserKeywordMutation(c *gin.Context, mutate func(string, string) error) {
	var req userKeywordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return
	}
	if err := mutate(req.UserID, req.Term); err != nil {
		writeKeywordError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func writeKeywordError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, keyword.ErrInvalidKeyword):
		writeError(c, http.StatusBadRequest, "invalid_keyword", "keyword term is required")
	case errors.Is(err, keyword.ErrInvalidUserID):
		writeError(c, http.StatusBadRequest, "invalid_user", "userId is required")
	case errors.Is(err, keyword.ErrKeywordNotFound):
		writeError(c, http.StatusNotFound, "keyword_not_found", "keyword was not found")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected keyword error")
	}
}

func writeTrustError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, trust.ErrMissingCitation):
		writeError(c, http.StatusBadRequest, "missing_citation", "AI summary must include source citations")
	case errors.Is(err, trust.ErrInvalidEvidence):
		writeError(c, http.StatusBadRequest, "invalid_evidence", "event evidence is invalid")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected trust error")
	}
}

func writeEventError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, event.ErrInvalidCandidate):
		writeError(c, http.StatusBadRequest, "invalid_event_candidate", "event candidate is missing required fields")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected event error")
	}
}

func writeContentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, content.ErrInvalidSourceItem):
		writeError(c, http.StatusBadRequest, "invalid_source_item", "source item is missing required trace fields")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected content error")
	}
}

func writeSourceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, source.ErrInvalidSourceConfig):
		writeError(c, http.StatusBadRequest, "invalid_source_config", "source configuration is invalid")
	case errors.Is(err, source.ErrNonCompliantSource):
		writeError(c, http.StatusBadRequest, "non_compliant_source", "source access mode is not allowed")
	case errors.Is(err, source.ErrSourceNotFound):
		writeError(c, http.StatusNotFound, "source_not_found", "source was not found")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected source error")
	}
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
