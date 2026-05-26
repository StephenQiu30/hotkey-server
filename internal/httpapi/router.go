package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/StephenQiu30/hotkey-server/internal/openapi"
	"github.com/StephenQiu30/hotkey-server/internal/source"
	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	return NewRouterWithServices(keyword.NewService(), source.NewService(), content.NewService())
}

func NewRouterWithKeywordService(keywordService *keyword.Service) *gin.Engine {
	return NewRouterWithServices(keywordService, source.NewService(), content.NewService())
}

func NewRouterWithServices(keywordService *keyword.Service, sourceService *source.Service, contentServices ...*content.Service) *gin.Engine {
	contentService := content.NewService()
	if len(contentServices) > 0 && contentServices[0] != nil {
		contentService = contentServices[0]
	}

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
