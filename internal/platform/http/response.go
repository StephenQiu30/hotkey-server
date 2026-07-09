package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"go.uber.org/zap"
)

// RespondOK writes a 200 response with unified success body.
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, vo.ResponseBody{
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondCreated writes a 201 response with unified success body.
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, vo.ResponseBody{
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondPage writes a 200 response with paginated body.
func RespondPage(c *gin.Context, data any, page, pageSize, total int) {
	c.JSON(http.StatusOK, vo.PageBody{
		Data:      data,
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		RequestID: requestIDFromContext(c),
	})
}

// RespondError writes a structured JSON error response.
// The HTTP status is inferred from the error code via errorCodeToHTTPStatus.
func RespondError(c *gin.Context, code enum.ErrorCode, message string) {
	c.JSON(errorCodeToHTTPStatus(code), ErrorBody{
		Error:     message,
		Code:      string(code),
		RequestID: requestIDFromContext(c),
	})
}

// RespondInternalError writes a generic 500 error.
func RespondInternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, ErrorBody{
		Error:     "internal server error",
		Code:      string(enum.ErrorCodeInternal),
		RequestID: requestIDFromContext(c),
	})
}

// RespondAppError writes an *AppError as a unified JSON error response.
func RespondAppError(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, ErrorBody{
			Error:     appErr.Message,
			Code:      string(appErr.Code),
			RequestID: requestIDFromContext(c),
		})
		return
	}
	RespondInternalError(c)
}

// errorCodeToHTTPStatus maps a stable ErrorCode to its HTTP status code.
func errorCodeToHTTPStatus(code enum.ErrorCode) int {
	switch code {
	case enum.ErrorCodeBadRequest:
		return http.StatusBadRequest
	case enum.ErrorCodeUnauthorized:
		return http.StatusUnauthorized
	case enum.ErrorCodeForbidden:
		return http.StatusForbidden
	case enum.ErrorCodeNotFound:
		return http.StatusNotFound
	case enum.ErrorCodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// ErrorHandlerMiddleware catches unhandled errors from c.Errors after the
// handler executes and writes a unified error response. Register this
// middleware after AuthMiddleware but before route registrations.
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		if len(c.Errors) > 0 {
			var appErr *AppError
			if errors.As(c.Errors.Last().Err, &appErr) {
				RespondAppError(c, appErr)
				c.Abort()
				return
			}
		}

		logging.Ctx(c.Request.Context()).Warn("unhandled gin error",
			zap.Any("errors", c.Errors),
		)
		RespondInternalError(c)
		c.Abort()
	}
}
func requestIDFromContext(c *gin.Context) string {
	if value, ok := c.Get("request_id"); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return c.GetHeader("X-Request-Id")
}
