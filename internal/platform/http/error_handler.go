package http

import (
	"context"
	"errors"
	stdhttp "net/http"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

const statusClientClosedRequest = 499

func WriteError(c *gin.Context, err error) {
	status, code, message := errorResponse(err)
	Fail(c, status, code, message)
}

func errorResponse(err error) (int, int, string) {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError.HTTPStatus, appError.Code, appError.Message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		definition, _ := sharederrors.Lookup(sharederrors.CodeDeadlineExceeded)
		return definition.HTTPStatus, definition.Code, definition.Message
	}
	if errors.Is(err, context.Canceled) {
		definition, _ := sharederrors.Lookup(sharederrors.CodeInvalidRequest)
		return statusClientClosedRequest, definition.Code, "request canceled"
	}
	definition, _ := sharederrors.Lookup(sharederrors.CodeInternal)
	return stdhttp.StatusInternalServerError, definition.Code, definition.Message
}
