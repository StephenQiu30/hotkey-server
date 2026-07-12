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
		Code:      http.StatusOK,
		ErrorCode: enum.ErrorCodeSuccess,
		Data:      data,
	})
}

// RespondCreated writes a 201 response with unified success body.
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, vo.ResponseBody{
		Code:      http.StatusCreated,
		ErrorCode: enum.ErrorCodeSuccess,
		Data:      data,
	})
}

// RespondPage writes a 200 response with paginated body.
func RespondPage(c *gin.Context, data any, page, pageSize, total int) {
	c.JSON(http.StatusOK, vo.PageBody{
		Code:      http.StatusOK,
		ErrorCode: enum.ErrorCodeSuccess,
		Data:      data,
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
	})
}

// RespondError writes a structured JSON error response.
// The HTTP status and message are inferred from the error code via the spec registry.
func RespondError(c *gin.Context, code enum.ErrorCode, message string) {
	spec := GetErrorSpec(code)
	_ = message // Kept until callers migrate; public responses never expose text.
	c.JSON(spec.HTTPStatus, vo.ResponseBody{
		Code:      spec.HTTPStatus,
		ErrorCode: code,
		Data:      nil,
	})
}

// RespondInternalError writes a generic 500 error using the unified envelope.
func RespondInternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, vo.ResponseBody{
		Code:      http.StatusInternalServerError,
		ErrorCode: enum.ErrorCodeInternal,
		Data:      nil,
	})
}

// RespondAppError writes an *AppError as a unified JSON error response.
func RespondAppError(c *gin.Context, err error) {
	var coded interface{ ErrorCode() enum.ErrorCode }
	if errors.As(err, &coded) {
		code := coded.ErrorCode()
		spec := GetErrorSpec(code)
		c.JSON(spec.HTTPStatus, vo.ResponseBody{Code: spec.HTTPStatus, ErrorCode: code, Data: nil})
		return
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, vo.ResponseBody{
			Code:      appErr.HTTPStatus,
			ErrorCode: appErr.Code,
			Data:      nil,
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
		var coded interface{ ErrorCode() enum.ErrorCode }
		if errors.As(c.Errors.Last().Err, &appErr) || errors.As(c.Errors.Last().Err, &coded) {
			RespondAppError(c, c.Errors.Last().Err)
			c.Abort()
			return
		}

		// Unknown error type - use internal error
		c.JSON(http.StatusInternalServerError, vo.ResponseBody{
			Code:      http.StatusInternalServerError,
			ErrorCode: enum.ErrorCodeInternal,
			Data:      nil,
		})
		c.Abort()
	}
}
