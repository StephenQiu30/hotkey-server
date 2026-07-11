package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

// RespondOK writes a 200 response with unified success body.
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, vo.ResponseBody{
		Code:      enum.ErrorCodeSuccess,
		Message:   "success",
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondCreated writes a 201 response with unified success body.
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, vo.ResponseBody{
		Code:      enum.ErrorCodeSuccess,
		Message:   "success",
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondPage writes a 200 response with paginated body.
func RespondPage(c *gin.Context, data any, page, pageSize, total int) {
	c.JSON(http.StatusOK, vo.PageBody{
		Code:      enum.ErrorCodeSuccess,
		Message:   "success",
		Data:      data,
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		RequestID: requestIDFromContext(c),
	})
}

// RespondError writes a structured JSON error response.
// The HTTP status and message are inferred from the error code via the spec registry.
func RespondError(c *gin.Context, code enum.ErrorCode, message string) {
	spec := GetErrorSpec(code)
	if message == "" {
		message = spec.Message
	}
	c.JSON(spec.HTTPStatus, vo.ResponseBody{
		Code:      code,
		Message:   message,
		Data:      nil,
		RequestID: requestIDFromContext(c),
	})
}

// RespondInternalError writes a generic 500 error using the unified envelope.
func RespondInternalError(c *gin.Context) {
	spec := GetErrorSpec(enum.ErrorCodeInternal)
	c.JSON(http.StatusInternalServerError, vo.ResponseBody{
		Code:      enum.ErrorCodeInternal,
		Message:   spec.Message,
		Data:      nil,
		RequestID: requestIDFromContext(c),
	})
}

// RespondAppError writes an *AppError as a unified JSON error response.
func RespondAppError(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, vo.ResponseBody{
			Code:      appErr.Code,
			Message:   appErr.Message,
			Data:      nil,
			RequestID: requestIDFromContext(c),
		})
		return
	}
	RespondInternalError(c)
}

// ErrorHandlerMiddleware catches unhandled errors from c.Errors after the
// handler executes and writes a unified error response, including panics
// caught by RecoverMiddleware. Register this middleware after AuthMiddleware
// but before route registrations.
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		var appErr *AppError
		if errors.As(c.Errors.Last().Err, &appErr) {
			RespondAppError(c, appErr)
			c.Abort()
			return
		}

		// Unknown error type - use internal error
		spec := GetErrorSpec(enum.ErrorCodeInternal)
		c.JSON(http.StatusInternalServerError, vo.ResponseBody{
			Code:      enum.ErrorCodeInternal,
			Message:   spec.Message,
			Data:      nil,
			RequestID: requestIDFromContext(c),
		})
		c.Abort()
	}
}
