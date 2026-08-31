package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"
)

const (
	notificationWebSocketProtocol     = "hotkey.notifications.v1"
	notificationWebSocketAuthTimeout  = 5 * time.Second
	notificationWebSocketWriteTimeout = 5 * time.Second
	notificationWebSocketReadLimit    = 8192
)

type notificationService interface {
	ListUserNotifications(context.Context, application.ListUserNotificationsQuery) (application.ListUserNotificationsResult, error)
	RecordDeliveryAttempt(context.Context, application.RecordNotificationDeliveryAttemptCommand) (application.RecordNotificationDeliveryAttemptResult, error)
	RecordNotificationReadReceipt(context.Context, application.RecordNotificationReadReceiptCommand) (application.RecordNotificationReadReceiptResult, error)
}

type StreamConfig struct {
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	MaxConnections    int
	AllowedOrigins    []string
}

type Handler struct {
	service           notificationService
	pollInterval      time.Duration
	heartbeatInterval time.Duration
	slots             chan struct{}
	originPatterns    []string
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
		slots: make(chan struct{}, config.MaxConnections), originPatterns: append([]string(nil), config.AllowedOrigins...),
		clock: func() time.Time { return time.Now().UTC() },
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
	query, err := notificationListQuery(c)
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

// RecordReadReceipt godoc
//
// @Summary Advance the current user's durable notification read cursor
// @Tags notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body RecordNotificationReadReceiptRequest true "monotonic visible notification cursor"
// @Success 200 {object} NotificationResult[NotificationReadReceiptResponseDTO]
// @Failure 400 {object} NotificationResult[NotificationReadReceiptResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Failure 404 {object} NotificationResult[NotificationReadReceiptResponseDTO]
// @Failure 409 {object} NotificationResult[NotificationReadReceiptResponseDTO]
// @Failure 503 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/read-receipts [post]
func (handler *Handler) RecordReadReceipt(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	c.Header("Cache-Control", "private, no-store")
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	var request RecordNotificationReadReceiptRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return invalidNotificationRequest()
	}
	result, err := handler.service.RecordNotificationReadReceipt(c.Request.Context(), application.RecordNotificationReadReceiptCommand{
		UserID: subject.UserID, ReadThroughID: request.ReadThroughID,
	})
	if err != nil {
		return notificationReadReceiptError(err, result)
	}
	httptransport.OK(c, notificationReadReceiptResponse(result))
	return nil
}

type notificationWebSocketAuthenticateFrame struct {
	Type      string `json:"type"`
	Token     string `json:"token"`
	AfterID   int64  `json:"after_id"`
	MonitorID *int64 `json:"monitor_id,omitempty"`
}

type notificationWebSocketControlFrame struct {
	Type    string    `json:"type"`
	AfterID int64     `json:"after_id"`
	SentAt  time.Time `json:"sent_at,omitempty"`
}

type notificationWebSocketItemFrame struct {
	Type  string                      `json:"type"`
	ID    int64                       `json:"id"`
	Event string                      `json:"event"`
	Data  UserNotificationResponseDTO `json:"data"`
}

// WebSocket godoc
//
// @Summary Upgrade to the authenticated notification WebSocket
// @Description Request hotkey.notifications.v1, then send one authenticate frame containing token, after_id and optional monitor_id before business data is emitted.
// @Tags notifications
// @Produce json
// @Success 101 {string} string
// @Failure 400 {object} NotificationResult[EmptyResponseDTO]
// @Failure 503 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/ws [get]
func (handler *Handler) WebSocket(authenticator httptransport.Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		httptransport.SetModule(c, "notification")
		if !webSocketProtocolRequested(c.Request.Header.Values("Sec-WebSocket-Protocol")) {
			httptransport.WriteError(c, invalidNotificationRequest())
			return
		}
		select {
		case handler.slots <- struct{}{}:
			defer func() { <-handler.slots }()
		default:
			httptransport.WriteError(c, sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "notification stream capacity reached"))
			return
		}

		connection, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			Subprotocols:   []string{notificationWebSocketProtocol},
			OriginPatterns: handler.originPatterns,
		})
		if err != nil {
			return
		}
		defer func() { _ = connection.CloseNow() }()
		connection.SetReadLimit(notificationWebSocketReadLimit)

		authenticationContext, cancelAuthentication := context.WithTimeout(c.Request.Context(), notificationWebSocketAuthTimeout)
		authentication, err := readNotificationWebSocketAuthentication(authenticationContext, connection)
		if err == nil {
			err = validateNotificationWebSocketAuthentication(authentication)
		}
		var subject httptransport.Subject
		if err == nil {
			subject, err = httptransport.AuthenticateBearerToken(authenticationContext, authenticator, authentication.Token)
		}
		cancelAuthentication()
		authentication.Token = ""
		if err != nil {
			_ = connection.Close(websocket.StatusPolicyViolation, "authentication failed")
			return
		}

		query := application.ListUserNotificationsQuery{
			UserID: subject.UserID, MonitorID: authentication.MonitorID, AfterID: authentication.AfterID, Limit: 100,
		}
		streamContext := connection.CloseRead(c.Request.Context())
		page, err := handler.service.ListUserNotifications(streamContext, query)
		if err != nil {
			_ = connection.Close(websocket.StatusInternalError, "notification service unavailable")
			return
		}
		if err := writeNotificationWebSocketJSON(streamContext, connection, notificationWebSocketControlFrame{Type: "ready", AfterID: query.AfterID}); err != nil {
			return
		}
		cursor, err := handler.writeWebSocketNotificationFrames(streamContext, connection, subject.UserID, page)
		if err != nil {
			return
		}

		poll := time.NewTicker(handler.pollInterval)
		heartbeat := time.NewTicker(handler.heartbeatInterval)
		defer poll.Stop()
		defer heartbeat.Stop()
		for {
			select {
			case <-streamContext.Done():
				return
			case <-heartbeat.C:
				if err := writeNotificationWebSocketJSON(streamContext, connection, notificationWebSocketControlFrame{
					Type: "heartbeat", AfterID: cursor, SentAt: handler.clock(),
				}); err != nil {
					return
				}
			case <-poll.C:
				page, err := handler.service.ListUserNotifications(streamContext, application.ListUserNotificationsQuery{
					UserID: subject.UserID, MonitorID: authentication.MonitorID, AfterID: cursor, Limit: 100,
				})
				if err != nil {
					return
				}
				cursor, err = handler.writeWebSocketNotificationFrames(streamContext, connection, subject.UserID, page)
				if err != nil {
					return
				}
			}
		}
	}
}

func readNotificationWebSocketAuthentication(ctx context.Context, connection *websocket.Conn) (notificationWebSocketAuthenticateFrame, error) {
	messageType, payload, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText {
		return notificationWebSocketAuthenticateFrame{}, invalidNotificationRequest()
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var frame notificationWebSocketAuthenticateFrame
	if err := decoder.Decode(&frame); err != nil {
		return notificationWebSocketAuthenticateFrame{}, invalidNotificationRequest()
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return notificationWebSocketAuthenticateFrame{}, invalidNotificationRequest()
	}
	return frame, nil
}

func validateNotificationWebSocketAuthentication(frame notificationWebSocketAuthenticateFrame) error {
	if frame.Type != "authenticate" || strings.TrimSpace(frame.Token) == "" || len(frame.Token) > notificationWebSocketReadLimit ||
		frame.AfterID < 0 || frame.MonitorID != nil && *frame.MonitorID <= 0 {
		return invalidNotificationRequest()
	}
	return nil
}

func webSocketProtocolRequested(values []string) bool {
	for _, value := range values {
		for _, protocol := range strings.Split(value, ",") {
			if strings.TrimSpace(protocol) == notificationWebSocketProtocol {
				return true
			}
		}
	}
	return false
}

func (handler *Handler) writeWebSocketNotificationFrames(ctx context.Context, connection *websocket.Conn, userID int64, page application.ListUserNotificationsResult) (int64, error) {
	cursor := page.NextAfterID
	for _, item := range page.Items {
		if err := writeNotificationWebSocketJSON(ctx, connection, notificationWebSocketItemFrame{
			Type: "notification", ID: item.ID, Event: item.EventType, Data: userNotificationResponse(item),
		}); err != nil {
			return cursor, err
		}
		_, _ = handler.service.RecordDeliveryAttempt(ctx, application.RecordNotificationDeliveryAttemptCommand{
			UserNotificationID: item.ID, UserID: userID, Channel: "websocket", DeliveryTargetKey: "browser_ws",
			Status: "succeeded", AttemptedAt: handler.clock(),
		})
	}
	return cursor, nil
}

func writeNotificationWebSocketJSON(ctx context.Context, connection *websocket.Conn, value any) error {
	writeContext, cancel := context.WithTimeout(ctx, notificationWebSocketWriteTimeout)
	defer cancel()
	return wsjson.Write(writeContext, connection, value)
}

func notificationListQuery(c *gin.Context) (application.ListUserNotificationsQuery, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return application.ListUserNotificationsQuery{}, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	afterIDValue := strings.TrimSpace(c.Query("after_id"))
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

func notificationReadReceiptError(err error, result application.RecordNotificationReadReceiptResult) error {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	var response any
	if result.ReadThroughID >= 0 {
		response = notificationReadReceiptResponse(result)
	}
	var mapped *sharederrors.AppError
	switch {
	case errors.Is(err, sharedrepository.ErrConflict):
		mapped = sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "notification read cursor cannot move backward")
	case errors.Is(err, sharedrepository.ErrNotFound):
		mapped = sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "notification read cursor is not visible")
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		mapped = sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid notification read cursor")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		mapped = sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "notification service unavailable")
	default:
		return fmt.Errorf("record notification read receipt: %w", err)
	}
	mapped.Data = response
	return mapped
}
