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
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type notificationService interface {
	ListUserNotifications(context.Context, application.ListUserNotificationsQuery) (application.ListUserNotificationsResult, error)
	RecordDeliveryAttempt(context.Context, application.RecordNotificationDeliveryAttemptCommand) (application.RecordNotificationDeliveryAttemptResult, error)
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
	clock             func() time.Time
}

func NewHandler(service notificationService, config StreamConfig) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("user notification service is required")
	}
	if config.PollInterval <= 0 || config.HeartbeatInterval <= 0 || config.MaxConnections <= 0 {
		return nil, fmt.Errorf("notification stream configuration is invalid")
	}
	return &Handler{
		service: service, pollInterval: config.PollInterval, heartbeatInterval: config.HeartbeatInterval,
		slots: make(chan struct{}, config.MaxConnections), clock: func() time.Time { return time.Now().UTC() },
	}, nil
}

// List godoc
//
// @Summary List current user's monitor notifications after a durable cursor
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param after_id query int false "last processed user notification ID" minimum(0)
// @Param monitor_id query int false "authorized monitor filter" minimum(1)
// @Param limit query int false "page size" minimum(1) maximum(100)
// @Success 200 {object} NotificationResult[UserNotificationPageResponseDTO]
// @Failure 400 {object} NotificationResult[EmptyResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Failure 503 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	c.Header("Cache-Control", "private, no-store")
	query, err := notificationListQuery(c, false)
	if err != nil {
		return err
	}
	page, err := handler.service.ListUserNotifications(c.Request.Context(), query)
	if err != nil {
		return notificationError(err)
	}
	httptransport.OK(c, userNotificationPageResponse(page))
	return nil
}

// Stream godoc
//
// @Summary Stream current user's monitor notifications with durable replay
// @Tags notifications
// @Produce text/event-stream
// @Security BearerAuth
// @Param after_id query int false "last processed user notification ID" minimum(0)
// @Param monitor_id query int false "authorized monitor filter" minimum(1)
// @Param Last-Event-ID header int false "last processed user notification ID" minimum(0)
// @Success 200 {string} string
// @Failure 400 {object} NotificationResult[EmptyResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Failure 503 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/stream [get]
func (handler *Handler) Stream(c *gin.Context) {
	httptransport.SetModule(c, "notification")
	query, err := notificationListQuery(c, true)
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

	page, err := handler.service.ListUserNotifications(c.Request.Context(), query)
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
	c.Header("Cache-Control", "private, no-store, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(stdhttp.StatusOK)
	cursor, writeErr := handler.writeNotificationFrames(c, flusher, query.UserID, page)
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
			page, err := handler.service.ListUserNotifications(c.Request.Context(), application.ListUserNotificationsQuery{
				UserID: query.UserID, MonitorID: query.MonitorID, AfterID: cursor, Limit: 100,
			})
			if err != nil {
				return
			}
			cursor, err = handler.writeNotificationFrames(c, flusher, query.UserID, page)
			if err != nil {
				return
			}
		}
	}
}

func (handler *Handler) writeNotificationFrames(c *gin.Context, flusher stdhttp.Flusher, userID int64, page application.ListUserNotificationsResult) (int64, error) {
	cursor := page.NextAfterID
	for _, item := range page.Items {
		response := userNotificationResponse(item)
		payload, err := json.Marshal(response)
		if err != nil {
			return cursor, err
		}
		if _, err := fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", item.ID, item.EventType, payload); err != nil {
			return cursor, err
		}
		flusher.Flush()
		// Delivery attempts are independent append-only transport facts. A
		// bookkeeping failure must not duplicate or retract an already written SSE frame.
		_, _ = handler.service.RecordDeliveryAttempt(c.Request.Context(), application.RecordNotificationDeliveryAttemptCommand{
			UserNotificationID: item.ID, UserID: userID, Channel: "sse", DeliveryTargetKey: "browser_stream",
			Status: "succeeded", AttemptedAt: handler.clock(),
		})
	}
	return cursor, nil
}

func notificationListQuery(c *gin.Context, stream bool) (application.ListUserNotificationsQuery, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return application.ListUserNotificationsQuery{}, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	afterIDValue := strings.TrimSpace(c.Query("after_id"))
	if stream && strings.TrimSpace(c.GetHeader("Last-Event-ID")) != "" {
		afterIDValue = strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	}
	afterID, err := parseNonNegativeInt64(afterIDValue)
	if err != nil {
		return application.ListUserNotificationsQuery{}, invalidNotificationRequest()
	}
	limit := 50
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return application.ListUserNotificationsQuery{}, invalidNotificationRequest()
		}
		limit = parsed
	}
	var monitorID *int64
	if value := strings.TrimSpace(c.Query("monitor_id")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 {
			return application.ListUserNotificationsQuery{}, invalidNotificationRequest()
		}
		monitorID = &parsed
	}
	query := application.ListUserNotificationsQuery{UserID: subject.UserID, MonitorID: monitorID, AfterID: afterID, Limit: limit}
	if query.UserID <= 0 || query.AfterID < 0 || query.Limit <= 0 || query.Limit > 100 {
		return application.ListUserNotificationsQuery{}, invalidNotificationRequest()
	}
	return query, nil
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
