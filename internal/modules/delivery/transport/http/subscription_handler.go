package http

import (
	"context"
	"net/http"
	"strconv"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type subscriptionService interface {
	Create(context.Context, deliveryapplication.CreateSubscriptionInput) (deliveryapplication.SubscriptionSecret, error)
	List(context.Context, identitydomain.Subject) ([]domain.Subscription, error)
	Get(context.Context, identitydomain.Subject, int64) (domain.Subscription, error)
	Update(context.Context, deliveryapplication.UpdateSubscriptionInput) (domain.Subscription, error)
	RotateRSSToken(context.Context, deliveryapplication.RotateRSSTokenInput) (deliveryapplication.SubscriptionSecret, error)
}

var _ subscriptionService = (*deliveryapplication.SubscriptionService)(nil)

type SubscriptionHandler struct{ service subscriptionService }

func NewSubscriptionHandler(service subscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{service: service}
}

// ListSubscriptions returns only the current user's subscriptions and never
// exposes RSS hashes or any delivery recipient owned by another user.
// @Summary List the current user's report subscriptions
// @Tags delivery
// @Produce json
// @Security BearerAuth
// @Success 200 {object} DeliveryResult[[]SubscriptionResponse]
// @Failure 401 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 503 {object} DeliveryResult[DeliveryEmptyResponse]
// @Router /api/v1/report-subscriptions [get]
func (handler *SubscriptionHandler) List(c *gin.Context) error {
	subject, err := deliverySubject(c)
	if err != nil {
		return err
	}
	items, err := handler.service.List(c.Request.Context(), subject)
	if err != nil {
		return err
	}
	response := make([]SubscriptionResponse, 0, len(items))
	for _, item := range items {
		response = append(response, subscriptionResponse(item))
	}
	httptransport.OK(c, response)
	return nil
}

// GetSubscription returns one current-user subscription without token material.
// @Summary Get a report subscription
// @Tags delivery
// @Produce json
// @Security BearerAuth
// @Param id path int true "subscription ID"
// @Success 200 {object} DeliveryResult[SubscriptionResponse]
// @Failure 400 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 401 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 404 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 503 {object} DeliveryResult[DeliveryEmptyResponse]
// @Router /api/v1/report-subscriptions/{id} [get]
func (handler *SubscriptionHandler) Get(c *gin.Context) error {
	subject, err := deliverySubject(c)
	if err != nil {
		return err
	}
	id, err := subscriptionID(c)
	if err != nil {
		return err
	}
	subscription, err := handler.service.Get(c.Request.Context(), subject, id)
	if err != nil {
		return err
	}
	httptransport.OK(c, subscriptionResponse(subscription))
	return nil
}

// CreateSubscription creates an email or private RSS subscription. An RSS
// token is returned only in this create response.
// @Summary Create a report subscription
// @Tags delivery
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateSubscriptionRequest true "subscription"
// @Success 201 {object} DeliveryResult[SubscriptionSecretResponse]
// @Failure 400 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 401 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 409 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 503 {object} DeliveryResult[DeliveryEmptyResponse]
// @Router /api/v1/report-subscriptions [post]
func (handler *SubscriptionHandler) Create(c *gin.Context) error {
	subject, err := deliverySubject(c)
	if err != nil {
		return err
	}
	var request CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return deliveryInvalidRequest(err)
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	result, err := handler.service.Create(c.Request.Context(), deliveryapplication.CreateSubscriptionInput{Subject: subject, MonitorID: request.MonitorID, ReportType: request.ReportType, Channel: domain.Channel(request.Channel), Recipient: request.Recipient, Timezone: request.Timezone, Schedule: request.Schedule, Enabled: enabled})
	if err != nil {
		return err
	}
	httptransport.Created(c, subscriptionSecretResponse(result))
	return nil
}

// UpdateSubscription changes the current user's mutable subscription fields
// under an explicit optimistic version.
// @Summary Update a report subscription
// @Tags delivery
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "subscription ID"
// @Param request body UpdateSubscriptionRequest true "mutable subscription fields"
// @Success 200 {object} DeliveryResult[SubscriptionResponse]
// @Failure 400 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 401 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 404 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 409 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 503 {object} DeliveryResult[DeliveryEmptyResponse]
// @Router /api/v1/report-subscriptions/{id} [patch]
func (handler *SubscriptionHandler) Update(c *gin.Context) error {
	subject, err := deliverySubject(c)
	if err != nil {
		return err
	}
	id, err := subscriptionID(c)
	if err != nil {
		return err
	}
	var request UpdateSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return deliveryInvalidRequest(err)
	}
	subscription, err := handler.service.Update(c.Request.Context(), deliveryapplication.UpdateSubscriptionInput{Subject: subject, SubscriptionID: id, ExpectedVersion: request.ExpectedVersion, Recipient: request.Recipient, Timezone: request.Timezone, Schedule: request.Schedule, Enabled: request.Enabled})
	if err != nil {
		return err
	}
	httptransport.OK(c, subscriptionResponse(subscription))
	return nil
}

// RotateRSSToken invalidates the old private Feed URL immediately and returns
// the replacement opaque token once.
// @Summary Rotate a private RSS token
// @Tags delivery
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "subscription ID"
// @Param request body RotateRSSTokenRequest true "expected version"
// @Success 200 {object} DeliveryResult[SubscriptionSecretResponse]
// @Failure 400 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 401 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 404 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 409 {object} DeliveryResult[DeliveryEmptyResponse]
// @Failure 503 {object} DeliveryResult[DeliveryEmptyResponse]
// @Router /api/v1/report-subscriptions/{id}/rss-token/rotate [post]
func (handler *SubscriptionHandler) RotateToken(c *gin.Context) error {
	subject, err := deliverySubject(c)
	if err != nil {
		return err
	}
	id, err := subscriptionID(c)
	if err != nil {
		return err
	}
	var request RotateRSSTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return deliveryInvalidRequest(err)
	}
	result, err := handler.service.RotateRSSToken(c.Request.Context(), deliveryapplication.RotateRSSTokenInput{Subject: subject, SubscriptionID: id, ExpectedVersion: request.ExpectedVersion})
	if err != nil {
		return err
	}
	httptransport.OK(c, subscriptionSecretResponse(result))
	return nil
}

func deliverySubject(c *gin.Context) (identitydomain.Subject, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return identitydomain.Subject{}, sharederrors.New(sharederrors.CodeUnauthenticated, http.StatusUnauthorized, "")
	}
	return identitydomain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: identitydomain.Role(subject.Role)}, nil
}

func subscriptionID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, deliveryInvalidRequest(err)
	}
	return id, nil
}

func deliveryInvalidRequest(_ error) error {
	return sharederrors.New(sharederrors.CodeInvalidRequest, http.StatusBadRequest, "invalid subscription request")
}
