package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const maximumPushSubscriptionRequestBytes = 16 * 1024

type pushSubscriptionHTTPService interface {
	Capability() application.PushCapabilityDTO
	Register(context.Context, application.RegisterPushSubscriptionCommand) (application.PushSubscriptionDTO, error)
	List(context.Context, application.ListPushSubscriptionsQuery) (application.ListPushSubscriptionsResult, error)
	Update(context.Context, application.UpdatePushSubscriptionCommand) (application.PushSubscriptionDTO, error)
	Disable(context.Context, application.DisablePushSubscriptionCommand) (application.PushSubscriptionDTO, error)
}

type PushSubscriptionHandler struct{ service pushSubscriptionHTTPService }

func NewPushSubscriptionHandler(service *application.PushSubscriptionService) (*PushSubscriptionHandler, error) {
	return newPushSubscriptionHandler(service)
}

func newPushSubscriptionHandler(service pushSubscriptionHTTPService) (*PushSubscriptionHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("push subscription service is required")
	}
	return &PushSubscriptionHandler{service: service}, nil
}

// Capability returns only whether this deployment can accept Web Push opt-in
// and the public VAPID application-server key.
// @Summary Get Web Push capability
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} NotificationResult[PushCapabilityResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/push-capability [get]
func (handler *PushSubscriptionHandler) Capability(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	if _, err := pushSubject(c); err != nil {
		return err
	}
	c.Header("Cache-Control", "private, no-store")
	httptransport.OK(c, pushCapabilityResponse(handler.service.Capability()))
	return nil
}

// List returns only safe device metadata and monitor selections. Browser
// endpoint and key material are never projected back to the client.
// @Summary List current user's Web Push devices
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} NotificationResult[PushSubscriptionListResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Failure 503 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/push-subscriptions [get]
func (handler *PushSubscriptionHandler) List(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	userID, err := pushSubject(c)
	if err != nil {
		return err
	}
	result, err := handler.service.List(c.Request.Context(), application.ListPushSubscriptionsQuery{UserID: userID})
	if err != nil {
		return pushSubscriptionError(err)
	}
	c.Header("Cache-Control", "private, no-store")
	httptransport.OK(c, pushSubscriptionListResponse(result))
	return nil
}

// Register persists a browser-created subscription only after an explicit
// user gesture. Idempotency-Key makes retries safe without exposing secrets.
// @Summary Register a Web Push device
// @Tags notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param Idempotency-Key header string true "unique registration key"
// @Param request body RegisterPushSubscriptionRequestDTO true "browser subscription and notification preferences"
// @Success 201 {object} NotificationResult[PushSubscriptionResponseDTO]
// @Failure 400 {object} NotificationResult[EmptyResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Failure 404 {object} NotificationResult[EmptyResponseDTO]
// @Failure 409 {object} NotificationResult[EmptyResponseDTO]
// @Failure 503 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/push-subscriptions [post]
func (handler *PushSubscriptionHandler) Register(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	userID, err := pushSubject(c)
	if err != nil {
		return err
	}
	var request RegisterPushSubscriptionRequestDTO
	if err := bindPushSubscriptionJSON(c, &request); err != nil {
		return err
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	result, err := handler.service.Register(c.Request.Context(), application.RegisterPushSubscriptionCommand{
		UserID: userID, Endpoint: request.Endpoint, P256DH: request.Keys.P256DH, Auth: request.Keys.Auth,
		DeviceLabel: request.DeviceLabel, Timezone: request.Timezone, QuietStart: request.QuietStart,
		QuietEnd: request.QuietEnd, TTLSeconds: request.TTLSeconds, MonitorIDs: request.MonitorIDs,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return pushSubscriptionError(err)
	}
	setPushSubscriptionHeaders(c, result.Version)
	httptransport.Created(c, pushSubscriptionResponse(result))
	return nil
}

// Update atomically replaces device preferences and its complete Monitor
// allow-list using one strong version ETag.
// @Summary Update a Web Push device
// @Tags notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "subscription ID"
// @Param If-Match header string true "strong subscription version ETag, e.g. v1"
// @Param request body UpdatePushSubscriptionRequestDTO true "complete device preference replacement"
// @Success 200 {object} NotificationResult[PushSubscriptionResponseDTO]
// @Failure 400 {object} NotificationResult[EmptyResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Failure 404 {object} NotificationResult[EmptyResponseDTO]
// @Failure 409 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/push-subscriptions/{id} [put]
func (handler *PushSubscriptionHandler) Update(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	userID, subscriptionID, version, err := pushMutationIdentity(c)
	if err != nil {
		return err
	}
	var request UpdatePushSubscriptionRequestDTO
	if err := bindPushSubscriptionJSON(c, &request); err != nil {
		return err
	}
	result, err := handler.service.Update(c.Request.Context(), application.UpdatePushSubscriptionCommand{
		UserID: userID, SubscriptionID: subscriptionID, ExpectedVersion: version,
		DeviceLabel: request.DeviceLabel, Timezone: request.Timezone, QuietStart: request.QuietStart,
		QuietEnd: request.QuietEnd, TTLSeconds: request.TTLSeconds, MonitorIDs: request.MonitorIDs,
	})
	if err != nil {
		return pushSubscriptionError(err)
	}
	setPushSubscriptionHeaders(c, result.Version)
	httptransport.OK(c, pushSubscriptionResponse(result))
	return nil
}

// Disable unregisters one device while preserving its delivery audit.
// @Summary Disable a Web Push device
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param id path int true "subscription ID"
// @Param If-Match header string true "strong subscription version ETag, e.g. v1"
// @Success 200 {object} NotificationResult[PushSubscriptionResponseDTO]
// @Failure 400 {object} NotificationResult[EmptyResponseDTO]
// @Failure 401 {object} NotificationResult[EmptyResponseDTO]
// @Failure 404 {object} NotificationResult[EmptyResponseDTO]
// @Failure 409 {object} NotificationResult[EmptyResponseDTO]
// @Router /api/v1/notifications/push-subscriptions/{id} [delete]
func (handler *PushSubscriptionHandler) Disable(c *gin.Context) error {
	httptransport.SetModule(c, "notification")
	userID, subscriptionID, version, err := pushMutationIdentity(c)
	if err != nil {
		return err
	}
	result, err := handler.service.Disable(c.Request.Context(), application.DisablePushSubscriptionCommand{
		UserID: userID, SubscriptionID: subscriptionID, ExpectedVersion: version,
	})
	if err != nil {
		return pushSubscriptionError(err)
	}
	setPushSubscriptionHeaders(c, result.Version)
	httptransport.OK(c, pushSubscriptionResponse(result))
	return nil
}

func bindPushSubscriptionJSON(c *gin.Context, destination any) error {
	c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, maximumPushSubscriptionRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalidNotificationRequest()
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return invalidNotificationRequest()
	}
	if err := binding.Validator.ValidateStruct(destination); err != nil {
		return invalidNotificationRequest()
	}
	return nil
}

func pushSubject(c *gin.Context) (int64, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return 0, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	return subject.UserID, nil
}

func pushMutationIdentity(c *gin.Context) (int64, int64, int64, error) {
	userID, err := pushSubject(c)
	if err != nil {
		return 0, 0, 0, err
	}
	subscriptionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || subscriptionID <= 0 {
		return 0, 0, 0, invalidNotificationRequest()
	}
	values := c.Request.Header.Values("If-Match")
	if len(values) != 1 || !strings.HasPrefix(values[0], `"v`) || !strings.HasSuffix(values[0], `"`) {
		return 0, 0, 0, invalidNotificationRequest()
	}
	version, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(values[0], `"v`), `"`), 10, 64)
	if err != nil || version <= 0 {
		return 0, 0, 0, invalidNotificationRequest()
	}
	return userID, subscriptionID, version, nil
}

func setPushSubscriptionHeaders(c *gin.Context, version int64) {
	c.Header("ETag", fmt.Sprintf(`"v%d"`, version))
	c.Header("Cache-Control", "private, no-store")
}

func pushSubscriptionError(err error) error {
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return invalidNotificationRequest()
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "push subscription not found")
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "push subscription changed")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "web push is unavailable")
	default:
		return fmt.Errorf("push subscription operation: %w", err)
	}
}
