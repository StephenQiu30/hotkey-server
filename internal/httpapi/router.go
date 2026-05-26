package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/adminapi"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/event"
	"github.com/StephenQiu30/hotkey-server/internal/hotspot"
	"github.com/StephenQiu30/hotkey-server/internal/keyword"
	"github.com/StephenQiu30/hotkey-server/internal/openapi"
	"github.com/StephenQiu30/hotkey-server/internal/rbac"
	"github.com/StephenQiu30/hotkey-server/internal/redisinfra"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/StephenQiu30/hotkey-server/internal/source"
	"github.com/StephenQiu30/hotkey-server/internal/tenant"
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
	hotspotService := hotspot.NewService()
	reportService := report.NewService()
	redisInfraService := redisinfra.NewService()
	adminAPIService := adminapi.NewService()
	tenantService := tenant.NewService()
	rbacService := rbac.NewService()

	return newRouter(keywordService, sourceService, contentService, eventService, trustService, hotspotService, reportService, redisInfraService, adminAPIService, tenantService, rbacService)
}

func newRouter(keywordService *keyword.Service, sourceService *source.Service, contentService *content.Service, eventService *event.Service, trustService *trust.Service, hotspotService *hotspot.Service, reportService *report.Service, redisInfraService *redisinfra.Service, adminAPIService *adminapi.Service, tenantService *tenant.Service, rbacService *rbac.Service) *gin.Engine {
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
	router.GET("/api/v1/admin/task-runs", listAdminTaskRuns(adminAPIService))
	router.POST("/api/v1/admin/reports/daily", triggerAdminDailyReport(reportService, adminAPIService))
	router.POST("/api/v1/admin/tenants", createTenant(tenantService))
	router.GET("/api/v1/admin/tenants", listTenants(tenantService))
	router.POST("/api/v1/admin/tenants/:id/members", addTenantMember(tenantService))
	router.GET("/api/v1/admin/tenants/:id/keywords", listTenantKeywords(keywordService))
	router.POST("/api/v1/admin/tenants/:id/keywords", createTenantKeyword(keywordService))
	router.GET("/api/v1/admin/tenants/:id/sources", listTenantSources(sourceService))
	router.POST("/api/v1/admin/tenants/:id/sources", createTenantSource(sourceService))
	router.PATCH("/api/v1/admin/tenants/:id/sources/:sourceId", updateTenantSource(sourceService))
	router.POST("/api/v1/admin/tenants/:id/roles", grantTenantRole(rbacService))
	router.POST("/api/v1/admin/tenants/:id/authorize", authorizeTenantAction(rbacService))
	router.GET("/api/v1/admin/tenants/:id/audit-logs", listTenantAuditLogs(rbacService))
	router.GET("/api/v1/users/:id/tenants", listUserTenants(tenantService))
	router.GET("/api/v1/events/:id/evidence", getEventEvidence(trustService))
	router.GET("/api/v1/hotspots", listHotspots(hotspotService))
	router.GET("/api/v1/hotspots/:id", getHotspotDetail(hotspotService))
	router.GET("/api/v1/reports/daily", getPlatformDailyReport(reportService))
	router.GET("/api/v1/users/:id/reports/daily", getUserDailyReport(reportService))
	router.GET("/api/v1/tenants/:id/reports/daily", getTenantDailyReport(reportService))
	router.POST("/api/v1/refresh-queue", enqueueRefresh(redisInfraService))
	router.GET("/api/v1/admin/refresh-queue", listRefreshQueue(redisInfraService))
	router.GET("/api/v1/admin/redis/health", getRedisHealth(redisInfraService))
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

type refreshQueueRequest struct {
	UserID string `json:"userId"`
	Scope  string `json:"scope"`
	Target string `json:"target"`
}

type adminDailyReportRequest struct {
	Date     string   `json:"date"`
	Scope    string   `json:"scope"`
	UserID   string   `json:"userId"`
	Keywords []string `json:"keywords"`
}

type createTenantRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type tenantMemberRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type roleGrantRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type authorizeRequest struct {
	UserID string `json:"userId"`
	Action string `json:"action"`
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

func listHotspots(service *hotspot.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"hotspots": service.ListHotspots(hotspot.ListOptions{
			Keyword:  c.Query("keyword"),
			Region:   c.Query("region"),
			Language: c.Query("language"),
			MinTrust: parsePositiveInt(c.Query("minTrust")),
			SortBy:   c.DefaultQuery("sort", hotspot.SortByHeat),
		})})
	}
}

func getHotspotDetail(service *hotspot.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, err := service.GetHotspotDetail(c.Param("id"))
		if err != nil {
			writeHotspotError(c, err)
			return
		}
		c.JSON(http.StatusOK, detail)
	}
}

func getPlatformDailyReport(service *report.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		reportDate, ok := parseReportDate(c)
		if !ok {
			return
		}
		daily, err := service.GeneratePlatformDailyReport(reportDate, defaultReportHotspots())
		if err != nil {
			writeReportError(c, err)
			return
		}
		c.JSON(http.StatusOK, daily)
	}
}

func getUserDailyReport(service *report.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		reportDate, ok := parseReportDate(c)
		if !ok {
			return
		}
		daily, err := service.GenerateUserDailyReport(reportDate, c.Param("id"), splitCSV(c.Query("keywords")), defaultReportHotspots())
		if err != nil {
			writeReportError(c, err)
			return
		}
		c.JSON(http.StatusOK, daily)
	}
}

func getTenantDailyReport(service *report.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		reportDate, ok := parseReportDate(c)
		if !ok {
			return
		}
		daily, err := service.GenerateTenantDailyReport(reportDate, c.Param("id"), defaultReportHotspots())
		if err != nil {
			writeReportError(c, err)
			return
		}
		c.JSON(http.StatusOK, daily)
	}
}

func createTenant(service *tenant.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createTenantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		created, err := service.CreateTenant(tenant.CreateTenantInput{Name: req.Name, Slug: req.Slug})
		if err != nil {
			writeTenantError(c, err)
			return
		}
		c.JSON(http.StatusCreated, created)
	}
}

func listTenants(service *tenant.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenants": service.ListTenants()})
	}
}

func addTenantMember(service *tenant.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req tenantMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		if err := service.AddMembership(tenant.MembershipInput{
			TenantID: c.Param("id"),
			UserID:   req.UserID,
			Role:     req.Role,
		}); err != nil {
			writeTenantError(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	}
}

func listUserTenants(service *tenant.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenants": service.ListUserTenants(c.Param("id"))})
	}
}

func listTenantKeywords(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"keywords": service.ListPlatformKeywordsByTenant(c.Param("id"))})
	}
}

func createTenantKeyword(service *keyword.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createKeywordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		created, err := service.CreatePlatformKeyword(keyword.CreatePlatformKeywordInput{
			TenantID: c.Param("id"),
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

func listTenantSources(service *source.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"sources": service.ListSourcesByTenant(c.Param("id"))})
	}
}

func createTenantSource(service *source.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req source.Source
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		req.TenantID = c.Param("id")
		if err := service.RegisterSource(req); err != nil {
			writeSourceError(c, err)
			return
		}
		c.JSON(http.StatusCreated, req)
	}
}

func updateTenantSource(service *source.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req updateSourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		updated, err := service.UpdateTenantSourceConfig(c.Param("id"), c.Param("sourceId"), source.UpdateSourceConfigInput{
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

func grantTenantRole(service *rbac.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req roleGrantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		binding := service.GrantRole(rbac.RoleGrantInput{
			TenantID: c.Param("id"),
			UserID:   req.UserID,
			Role:     req.Role,
		})
		c.JSON(http.StatusCreated, binding)
	}
}

func authorizeTenantAction(service *rbac.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req authorizeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		allowed := service.Can(rbac.AuthorizeInput{
			TenantID: c.Param("id"),
			UserID:   req.UserID,
			Action:   req.Action,
		})
		c.JSON(http.StatusOK, gin.H{"allowed": allowed})
	}
}

func listTenantAuditLogs(service *rbac.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"events": service.ListAuditEvents(c.Param("id"))})
	}
}

func listAdminTaskRuns(service *adminapi.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"taskRuns": service.ListTaskRuns(adminapi.ListTaskRunsOptions{
			Status: c.Query("status"),
		})})
	}
}

func triggerAdminDailyReport(reportService *report.Service, adminService *adminapi.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminDailyReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		reportDate, ok := parseReportDateValue(c, req.Date)
		if !ok {
			return
		}

		var (
			daily report.DailyReport
			err   error
		)
		if req.Scope == report.ScopeUser {
			daily, err = reportService.GenerateUserDailyReport(reportDate, req.UserID, req.Keywords, defaultReportHotspots())
		} else {
			daily, err = reportService.GeneratePlatformDailyReport(reportDate, defaultReportHotspots())
		}
		if err != nil {
			adminService.RecordTaskRun(adminapi.TaskRunInput{
				TaskName: "daily-report",
				Status:   adminapi.TaskStatusFailed,
				Message:  err.Error(),
			})
			writeReportError(c, err)
			return
		}

		run := adminService.RecordTaskRun(adminapi.TaskRunInput{
			TaskName: "daily-report",
			Status:   adminapi.TaskStatusSucceeded,
			Message:  "manual trigger accepted",
		})
		c.JSON(http.StatusAccepted, gin.H{
			"report":  daily,
			"taskRun": run,
		})
	}
}

func enqueueRefresh(service *redisinfra.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req refreshQueueRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
			return
		}
		result, err := service.EnqueueRefresh(redisinfra.RefreshRequest{
			UserID: req.UserID,
			Scope:  req.Scope,
			Target: req.Target,
		})
		if err != nil {
			writeRedisInfraError(c, err)
			return
		}
		if !result.Accepted {
			c.JSON(http.StatusTooManyRequests, result)
			return
		}
		c.JSON(http.StatusAccepted, result)
	}
}

func listRefreshQueue(service *redisinfra.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"queue": service.ListRefreshQueue()})
	}
}

func getRedisHealth(service *redisinfra.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, service.Health())
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

func writeTenantError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenant.ErrInvalidTenant):
		writeError(c, http.StatusBadRequest, "invalid_tenant", "tenant name and slug are required")
	case errors.Is(err, tenant.ErrInvalidMembership):
		writeError(c, http.StatusBadRequest, "invalid_membership", "tenant membership is invalid")
	case errors.Is(err, tenant.ErrTenantNotFound):
		writeError(c, http.StatusNotFound, "tenant_not_found", "tenant was not found")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected tenant error")
	}
}

func writeRedisInfraError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, redisinfra.ErrInvalidRedisInfraRequest):
		writeError(c, http.StatusBadRequest, "invalid_redis_infra_request", "redis infra request is invalid")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected redis infra error")
	}
}

func writeReportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, report.ErrMissingEvidenceLink):
		writeError(c, http.StatusBadRequest, "missing_evidence_link", "daily report item must link to event evidence")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected report error")
	}
}

func writeHotspotError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, hotspot.ErrHotspotNotFound):
		writeError(c, http.StatusNotFound, "hotspot_not_found", "hotspot was not found")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "unexpected hotspot error")
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

func parseReportDate(c *gin.Context) (time.Time, bool) {
	return parseReportDateValue(c, c.Query("date"))
}

func parseReportDateValue(c *gin.Context, value string) (time.Time, bool) {
	if value == "" {
		value = time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_report_date", "date must use YYYY-MM-DD")
		return time.Time{}, false
	}
	return parsed, true
}

func parsePositiveInt(value string) int {
	var result int
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		result = result*10 + int(ch-'0')
	}
	return result
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func defaultReportHotspots() []report.HotspotSnapshot {
	return []report.HotspotSnapshot{
		{
			EventID:     "cluster_openai_reasoning",
			Title:       "OpenAI releases new reasoning model",
			Keywords:    []string{"OpenAI", "model", "reasoning"},
			HeatScore:   88,
			TrustScore:  95,
			EvidenceIDs: []string{"item_1", "item_2"},
		},
		{
			EventID:     "cluster_ai_safety_report",
			Title:       "Anthropic publishes AI safety report",
			Keywords:    []string{"Anthropic", "safety", "AI"},
			HeatScore:   64,
			TrustScore:  90,
			EvidenceIDs: []string{"item_3"},
		},
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
