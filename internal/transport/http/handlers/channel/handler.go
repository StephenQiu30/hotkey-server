package channel

import (
	"errors"
	"net/http"

	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *servicechannel.Service
}

type channelRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type keywordRequest struct {
	Keyword string `json:"keyword"`
	Enabled *bool  `json:"enabled"`
}

type dailySendAtRequest struct {
	DailySendAt string `json:"dailySendAt"`
}

func New(service *servicechannel.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListChannels(c *gin.Context) {
	channels, err := h.service.ListChannels(c.Request.Context(), servicechannel.ListChannelsInput{
		ActiveOnly: c.Query("includeDisabled") != "true",
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"channels": channelResponses(channels)})
}

func (h *Handler) ListSubscriptions(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	subscriptions, err := h.service.ListSubscriptions(c.Request.Context(), account.ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscriptions": subscriptionResponses(subscriptions)})
}

func (h *Handler) Subscribe(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	subscription, err := h.service.Subscribe(c.Request.Context(), servicechannel.UserChannelInput{
		UserID:    account.ID,
		ChannelID: c.Param("channelID"),
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"subscription": subscriptionResponse(subscription)})
}

func (h *Handler) Unsubscribe(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	if err := h.service.Unsubscribe(c.Request.Context(), servicechannel.UserChannelInput{
		UserID:    account.ID,
		ChannelID: c.Param("channelID"),
	}); err != nil && !errors.Is(err, servicechannel.ErrNotFound) {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ListKeywords(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	keywords, err := h.service.ListKeywords(c.Request.Context(), account.ID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"keywords": keywordResponses(keywords)})
}

func (h *Handler) CreateKeyword(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req keywordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	keyword, err := h.service.CreateKeyword(c.Request.Context(), servicechannel.KeywordInput{
		UserID:  account.ID,
		Keyword: req.Keyword,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"keyword": keywordResponse(keyword)})
}

func (h *Handler) UpdateKeyword(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req keywordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	keyword, err := h.service.UpdateKeyword(c.Request.Context(), servicechannel.UpdateKeywordInput{
		UserID:    account.ID,
		KeywordID: c.Param("keywordID"),
		Keyword:   req.Keyword,
		Enabled:   req.Enabled,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"keyword": keywordResponse(keyword)})
}

func (h *Handler) DeleteKeyword(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	if err := h.service.DeleteKeyword(c.Request.Context(), account.ID, c.Param("keywordID")); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) SetUserDailySendAt(c *gin.Context) {
	account, ok := authhandler.CurrentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req dailySendAtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if err := h.service.SetUserDailySendAt(c.Request.Context(), servicechannel.UserDailySendAtInput{
		UserID:      account.ID,
		DailySendAt: req.DailySendAt,
	}); err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"dailySendAt": req.DailySendAt})
}

func (h *Handler) CreateChannel(c *gin.Context) {
	var req channelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	channel, err := h.service.CreateChannel(c.Request.Context(), servicechannel.CreateChannelInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"channel": channelResponse(channel)})
}

func (h *Handler) UpdateChannel(c *gin.Context) {
	var req channelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if req.Status != "" {
		channel, err := h.service.UpdateChannelStatus(c.Request.Context(), servicechannel.UpdateChannelStatusInput{
			ChannelID: c.Param("channelID"),
			Status:    servicechannel.ChannelStatus(req.Status),
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"channel": channelResponse(channel)})
		return
	}
	channel, err := h.service.UpdateChannel(c.Request.Context(), servicechannel.UpdateChannelInput{
		ChannelID:   c.Param("channelID"),
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"channel": channelResponse(channel)})
}

func (h *Handler) DeleteChannel(c *gin.Context) {
	if err := h.service.DeleteChannel(c.Request.Context(), c.Param("channelID")); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) SetDefaultDailySendAt(c *gin.Context) {
	var req dailySendAtRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if err := h.service.SetDefaultDailySendAt(c.Request.Context(), req.DailySendAt); err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"dailySendAt": req.DailySendAt})
}

func channelResponse(channel servicechannel.Channel) gin.H {
	return gin.H{
		"id":          channel.ID,
		"name":        channel.Name,
		"slug":        channel.Slug,
		"description": channel.Description,
		"status":      channel.Status,
	}
}

func channelResponses(channels []servicechannel.Channel) []gin.H {
	responses := make([]gin.H, 0, len(channels))
	for _, channel := range channels {
		responses = append(responses, channelResponse(channel))
	}
	return responses
}

func subscriptionResponse(subscription servicechannel.Subscription) gin.H {
	return gin.H{
		"userId":  subscription.UserID,
		"channel": channelResponse(subscription.Channel),
	}
}

func subscriptionResponses(subscriptions []servicechannel.Subscription) []gin.H {
	responses := make([]gin.H, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		responses = append(responses, subscriptionResponse(subscription))
	}
	return responses
}

func keywordResponse(keyword servicechannel.Keyword) gin.H {
	return gin.H{
		"id":      keyword.ID,
		"keyword": keyword.Keyword,
		"enabled": keyword.Enabled,
	}
}

func keywordResponses(keywords []servicechannel.Keyword) []gin.H {
	responses := make([]gin.H, 0, len(keywords))
	for _, keyword := range keywords {
		responses = append(responses, keywordResponse(keyword))
	}
	return responses
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, servicechannel.ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, servicechannel.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, servicechannel.ErrAlreadyExists):
		writeError(c, http.StatusConflict, "channel_slug_already_exists", "channel slug already exists")
	case errors.Is(err, servicechannel.ErrChannelDisabled):
		writeError(c, http.StatusConflict, "channel_disabled", "channel disabled")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
