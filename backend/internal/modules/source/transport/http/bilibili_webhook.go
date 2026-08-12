package http

import (
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type bilibiliWebhookService interface {
	HandleBilibiliDeauthorization(context.Context, domain.BilibiliWebhook) (bool, error)
}

type BilibiliWebhookHandler struct {
	service bilibiliWebhookService
	secret  string
}

func RegisterBilibiliWebhookRoutes(router *gin.Engine, service bilibiliWebhookService, secret string) {
	if router == nil || service == nil {
		return
	}
	handler := &BilibiliWebhookHandler{service: service, secret: secret}
	router.POST("/api/v1/source-webhooks/bilibili", httptransport.Wrap(handler.Receive))
}

// Receive handles only the two event types documented by the official MVP.
// @Summary Receive Bilibili Open Platform webhook
// @Tags source webhooks
// @Accept json
// @Produce json
// @Param x-bilibili-signature header string true "official webhook signature"
// @Success 200 {object} SourceResult[EmptyResponse]
// @Failure 400 {object} SourceResult[EmptyResponse]
// @Failure 401 {object} SourceResult[EmptyResponse]
// @Router /api/v1/source-webhooks/bilibili [post]
func (handler *BilibiliWebhookHandler) Receive(c *gin.Context) error {
	httptransport.SetModule(c, "source")
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, domain.BilibiliWebhookMaxBodyBytes+1))
	if err != nil || len(body) > domain.BilibiliWebhookMaxBodyBytes {
		httptransport.Fail(c, stdhttp.StatusBadRequest, 40001, "invalid request")
		return nil
	}
	webhook, err := domain.VerifyBilibiliWebhook(body, c.GetHeader("x-bilibili-signature"), handler.secret)
	if err != nil {
		httptransport.Fail(c, stdhttp.StatusUnauthorized, 40101, "invalid webhook")
		return nil
	}
	if webhook.Event == "verify_webhooks" {
		var echo any
		if json.Unmarshal(webhook.Echo, &echo) != nil {
			httptransport.Fail(c, stdhttp.StatusBadRequest, 40001, "invalid request")
			return nil
		}
		// Bilibili's endpoint-verification handshake requires the challenge
		// response shape verbatim; the application's normal response envelope
		// would make the platform reject an otherwise valid webhook endpoint.
		httptransport.WebhookChallenge(c, echo)
		return nil
	}
	_, err = handler.service.HandleBilibiliDeauthorization(c.Request.Context(), webhook)
	if err != nil {
		return err
	}
	httptransport.Empty(c)
	return nil
}

var _ bilibiliWebhookService = (*sourceapplication.Service)(nil)
