package monitortopic

import (
	"errors"
	"net/http"

	svc "github.com/StephenQiu30/hotkey-server/internal/service/monitortopic"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP endpoints for monitor topic operations.
type Handler struct {
	service *svc.Service
}

type createTopicRequest struct {
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Language            string   `json:"language"`
	Platforms           []string `json:"platforms"`
	SimilarityThreshold *float64 `json:"similarityThreshold"`
	CollectIntervalMin  *int     `json:"collectIntervalMin"`
	DailyReportEnabled  *bool    `json:"dailyReportEnabled"`
	ObsidianOutputDir   string   `json:"obsidianOutputDir"`
}

type updateTopicRequest struct {
	Name                *string   `json:"name"`
	Description         *string   `json:"description"`
	Language            *string   `json:"language"`
	Platforms           *[]string `json:"platforms"`
	SimilarityThreshold *float64  `json:"similarityThreshold"`
	CollectIntervalMin  *int      `json:"collectIntervalMin"`
	DailyReportEnabled  *bool     `json:"dailyReportEnabled"`
	ObsidianOutputDir   *string   `json:"obsidianOutputDir"`
}

type setStatusRequest struct {
	Status string `json:"status"`
}

type addKeywordRequest struct {
	Word string `json:"word"`
	Type string `json:"type"`
}

// New creates a new monitor topic handler.
func New(service *svc.Service) *Handler {
	return &Handler{service: service}
}

// ListTopics returns all topics for the authenticated user.
func (h *Handler) ListTopics(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	topics, err := h.service.ListTopics(c.Request.Context(), account.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"topics": topicResponses(topics)})
}

// CreateTopic creates a new monitor topic for the authenticated user.
func (h *Handler) CreateTopic(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req createTopicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	platforms := make([]svc.Platform, len(req.Platforms))
	for i, p := range req.Platforms {
		platforms[i] = svc.Platform(p)
	}
	var lang *svc.Language
	if req.Language != "" {
		l := svc.Language(req.Language)
		lang = &l
	}
	input := svc.CreateTopicInput{
		UserID:              account.ID,
		Name:                req.Name,
		Description:         req.Description,
		Platforms:           platforms,
		SimilarityThreshold: req.SimilarityThreshold,
		CollectIntervalMin:  req.CollectIntervalMin,
		DailyReportEnabled:  req.DailyReportEnabled,
		ObsidianOutputDir:   req.ObsidianOutputDir,
	}
	if lang != nil {
		input.Language = *lang
	}
	topic, err := h.service.CreateTopic(c.Request.Context(), input)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"topic": topicResponse(topic)})
}

// GetTopic returns a single topic by ID.
func (h *Handler) GetTopic(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	topic, err := h.service.GetTopic(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if topic.UserID != account.ID {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"topic": topicResponse(topic)})
}

// UpdateTopic applies partial updates to a topic.
func (h *Handler) UpdateTopic(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req updateTopicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	// Verify ownership
	existing, err := h.service.GetTopic(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if existing.UserID != account.ID {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	input := svc.UpdateTopicInput{
		TopicID:             c.Param("topicID"),
		Name:                req.Name,
		Description:         req.Description,
		SimilarityThreshold: req.SimilarityThreshold,
		CollectIntervalMin:  req.CollectIntervalMin,
		DailyReportEnabled:  req.DailyReportEnabled,
		ObsidianOutputDir:   req.ObsidianOutputDir,
	}
	if req.Language != nil {
		lang := svc.Language(*req.Language)
		input.Language = &lang
	}
	if req.Platforms != nil {
		platforms := make([]svc.Platform, len(*req.Platforms))
		for i, p := range *req.Platforms {
			platforms[i] = svc.Platform(p)
		}
		input.Platforms = platforms
	}
	topic, err := h.service.UpdateTopic(c.Request.Context(), input)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"topic": topicResponse(topic)})
}

// SetTopicStatus changes the status of a topic.
func (h *Handler) SetTopicStatus(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req setStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	// Verify ownership
	existing, err := h.service.GetTopic(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if existing.UserID != account.ID {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	topic, err := h.service.SetTopicStatus(c.Request.Context(), c.Param("topicID"), svc.TopicStatus(req.Status))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"topic": topicResponse(topic)})
}

// DeleteTopic deletes a topic and its keywords.
func (h *Handler) DeleteTopic(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	// Verify ownership
	existing, err := h.service.GetTopic(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if existing.UserID != account.ID {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	if err := h.service.DeleteTopic(c.Request.Context(), c.Param("topicID")); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListKeywords returns all keywords for a topic.
func (h *Handler) ListKeywords(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	// Verify ownership
	existing, err := h.service.GetTopic(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if existing.UserID != account.ID {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	keywords, err := h.service.ListKeywords(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"keywords": keywordResponses(keywords)})
}

// AddKeyword adds a keyword or exclusion word to a topic.
func (h *Handler) AddKeyword(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	// Verify ownership
	existing, err := h.service.GetTopic(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if existing.UserID != account.ID {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	var req addKeywordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	kw, err := h.service.AddKeyword(c.Request.Context(), svc.AddKeywordInput{
		TopicID: c.Param("topicID"),
		Word:    req.Word,
		Type:    svc.KeywordType(req.Type),
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"keyword": keywordResponse(kw)})
}

// DeleteKeyword removes a keyword from a topic.
func (h *Handler) DeleteKeyword(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	// Verify ownership via keyword lookup
	keywords, err := h.service.ListKeywords(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	existing, err := h.service.GetTopic(c.Request.Context(), c.Param("topicID"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if existing.UserID != account.ID {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	found := false
	for _, kw := range keywords {
		if kw.ID == c.Param("keywordID") {
			found = true
			break
		}
	}
	if !found {
		writeError(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	if err := h.service.DeleteKeyword(c.Request.Context(), c.Param("keywordID")); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func topicResponse(topic svc.MonitorTopic) gin.H {
	return gin.H{
		"id":                  topic.ID,
		"name":                topic.Name,
		"description":         topic.Description,
		"status":              topic.Status,
		"language":            topic.Language,
		"platforms":           topic.Platforms,
		"similarityThreshold": topic.SimilarityThreshold,
		"collectIntervalMin":  topic.CollectIntervalMin,
		"dailyReportEnabled":  topic.DailyReportEnabled,
		"obsidianOutputDir":   topic.ObsidianOutputDir,
		"createdAt":           topic.CreatedAt,
		"updatedAt":           topic.UpdatedAt,
	}
}

func topicResponses(topics []svc.MonitorTopic) []gin.H {
	responses := make([]gin.H, 0, len(topics))
	for _, topic := range topics {
		responses = append(responses, topicResponse(topic))
	}
	return responses
}

func keywordResponse(kw svc.TopicKeyword) gin.H {
	return gin.H{
		"id":   kw.ID,
		"word": kw.Word,
		"type": kw.Type,
	}
}

func keywordResponses(keywords []svc.TopicKeyword) []gin.H {
	responses := make([]gin.H, 0, len(keywords))
	for _, kw := range keywords {
		responses = append(responses, keywordResponse(kw))
	}
	return responses
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, svc.ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, svc.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, svc.ErrAlreadyExists):
		writeError(c, http.StatusConflict, "already_exists", "resource already exists")
	case errors.Is(err, svc.ErrInvalidTransition):
		writeError(c, http.StatusConflict, "invalid_status_transition", "invalid status transition")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
