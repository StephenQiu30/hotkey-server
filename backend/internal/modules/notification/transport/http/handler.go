package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type notificationService interface {
	ListAfter(context.Context, application.ListInput) (domain.NotificationPage, error)
}

type StreamConfig struct {
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	MaxConnections    int
}

type Handler struct {
	service           notificationService
	pollInterval      time.Duration
	heartbeatInterval time.Duration
	slots             chan struct{}
}

func NewHandler(service notificationService, config StreamConfig) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("notification service is required")
	}
	if config.PollInterval <= 0 || config.HeartbeatInterval <= 0 || config.MaxConnections <= 0 {
		return nil, fmt.Errorf("notification stream configuration is invalid")
	}
	return &Handler{
		service: service, pollInterval: config.PollInterval, heartbeatInterval: config.HeartbeatInterval,
		slots: make(chan struct{}, config.MaxConnections),
	}, nil
}

// List godoc
//
// @Summary List notification events after a durable cursor
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param after_id query int false "last processed notification ID" minimum(0)
// @Param limit query int false "page size" minimum(1) maximum(100)
// @Success 200 {object} NotificationResult[NotificationPageResponse]
// @Failure 400 {object} NotificationResult[EmptyResponse]
// @Failure 401 {object} NotificationResult[EmptyResponse]
// @Failure 503 {object} NotificationResult[EmptyResponse]
// @Router /api/v1/notifications [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	input, err := notificationListInput(c, false)
	if err != nil {
		return err
	}
	page, err := handler.service.ListAfter(c.Request.Context(), input)
	if err != nil {
		return notificationError(err)
	}
	httptransport.OK(c, notificationPageResponse(page))
	return nil
}

// Stream godoc
//
// @Summary Stream notification events after a durable cursor
// @Tags notifications
// @Produce text/event-stream
// @Security BearerAuth
// @Param after_id query int false "last processed notification ID" minimum(0)
// @Param Last-Event-ID header int false "last processed notification ID" minimum(0)
// @Success 200 {string} string
// @Failure 400 {object} NotificationResult[EmptyResponse]
// @Failure 401 {object} NotificationResult[EmptyResponse]
// @Failure 503 {object} NotificationResult[EmptyResponse]
// @Router /api/v1/notifications/stream [get]
func (handler *Handler) Stream(c *gin.Context) {
	httptransport.SetModule(c, "notification")
	input, err := notificationListInput(c, true)
	if err != nil {
		httptransport.WriteError(c, err)
		return
	}
	select {
	case handler.slots <- struct{}{}:
		defer func() { <-handler.slots }()
	default:
		httptransport.WriteError(c, sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "notification stream capacity reached"))
		return
	}

	page, err := handler.service.ListAfter(c.Request.Context(), input)
	if err != nil {
		httptransport.WriteError(c, notificationError(err))
		return
	}
	flusher, ok := c.Writer.(stdhttp.Flusher)
	if !ok {
		httptransport.WriteError(c, sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "notification stream unavailable"))
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(stdhttp.StatusOK)
	cursor, writeErr := writeNotificationFrames(c, flusher, page)
	if writeErr != nil {
		return
	}

	poll := time.NewTicker(handler.pollInterval)
	heartbeat := time.NewTicker(handler.heartbeatInterval)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-poll.C:
			page, err := handler.service.ListAfter(c.Request.Context(), application.ListInput{Role: input.Role, AfterID: cursor, Limit: domain.MaximumListLimit})
			if err != nil {
				return
			}
			cursor, err = writeNotificationFrames(c, flusher, page)
			if err != nil {
				return
			}
		}
	}
}

func writeNotificationFrames(c *gin.Context, flusher stdhttp.Flusher, page domain.NotificationPage) (int64, error) {
	cursor := page.NextAfterID
	for _, event := range page.Items {
		payload, err := json.Marshal(notificationResponse(event))
		if err != nil {
			return cursor, err
		}
		if _, err := fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.EventType, payload); err != nil {
			return cursor, err
		}
	}
	flusher.Flush()
	return cursor, nil
}

func notificationListInput(c *gin.Context, stream bool) (application.ListInput, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return application.ListInput{}, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	afterIDValue := strings.TrimSpace(c.Query("after_id"))
	if stream && strings.TrimSpace(c.GetHeader("Last-Event-ID")) != "" {
		afterIDValue = strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	}
	afterID, err := parseNonNegativeInt64(afterIDValue)
	if err != nil {
		return application.ListInput{}, invalidNotificationRequest()
	}
	limit := domain.DefaultListLimit
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return application.ListInput{}, invalidNotificationRequest()
		}
		limit = parsed
	}
	input := application.ListInput{Role: domain.AudienceRole(subject.Role), AfterID: afterID, Limit: limit}
	query := domain.NotificationQuery{Role: input.Role, AfterID: input.AfterID, Limit: input.Limit}
	if err := query.Validate(); err != nil {
		return application.ListInput{}, invalidNotificationRequest()
	}
	return input, nil
}

func parseNonNegativeInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return parsed, nil
}

func invalidNotificationRequest() error {
	return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid notification request")
}

func notificationError(err error) error {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return invalidNotificationRequest()
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "notification service unavailable")
	default:
		return fmt.Errorf("notification operation: %w", err)
	}
}
