package http

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type aiRunRecomputeService interface {
	Schedule(context.Context, int64) (intelligenceapplication.AIRunRecomputeResult, error)
}

var _ aiRunRecomputeService = (*intelligenceapplication.AIRunRecomputeService)(nil)

type AIRunHandler struct{ recompute aiRunRecomputeService }

func NewAIRunHandler(recompute aiRunRecomputeService) *AIRunHandler {
	return &AIRunHandler{recompute: recompute}
}

// Recompute schedules recovery of the owning durable business job. The
// recovery job itself receives only the failed AI run identifier.
// @Summary Recompute a failed AI run
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Param id path int true "AI run ID"
// @Success 202 {object} ModelProfileResult[AIRunRecomputeResponse]
// @Failure 400 {object} ModelProfileResult[EmptyResponse]
// @Failure 401 {object} ModelProfileResult[EmptyResponse]
// @Failure 403 {object} ModelProfileResult[EmptyResponse]
// @Failure 404 {object} ModelProfileResult[EmptyResponse]
// @Failure 409 {object} ModelProfileResult[EmptyResponse]
// @Failure 503 {object} ModelProfileResult[EmptyResponse]
// @Router /api/v1/ai/runs/{id}/recompute [post]
func (handler *AIRunHandler) Recompute(c *gin.Context) error {
	httptransport.SetModule(c, "intelligence")
	if handler == nil || handler.recompute == nil {
		return aiRunRecomputeError(sharedrepository.ErrUnavailable)
	}
	runID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || runID <= 0 {
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid AI run id")
	}
	result, err := handler.recompute.Schedule(c.Request.Context(), runID)
	if err != nil {
		return aiRunRecomputeError(err)
	}
	httptransport.Accepted(c, AIRunRecomputeResponse{RunID: result.RunID, JobID: result.JobID, Created: result.Created})
	return nil
}

func aiRunRecomputeError(err error) error {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "invalid AI run request")
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "AI run not found")
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "AI run cannot be recomputed")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "AI run service unavailable")
	default:
		return fmt.Errorf("schedule AI run recompute: %w", err)
	}
}
