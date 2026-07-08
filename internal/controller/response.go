package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

// RespondOK writes a successful response using the unified response contract.
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, vo.ResponseBody{
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondCreated writes a created response using the unified response contract.
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, vo.ResponseBody{
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondPage writes a paginated response using the unified response contract.
func RespondPage(c *gin.Context, data any, page, pageSize, total int) {
	c.JSON(http.StatusOK, vo.PageBody{
		Data:      data,
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		RequestID: requestIDFromContext(c),
	})
}

// respondError writes an error response using the platform/http error contract.
func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, platformhttp.ErrorBody{
		Error:     message,
		Code:      string(errorCodeForHTTPStatus(status)),
		RequestID: requestIDFromContext(c),
	})
}

// respondInternalError writes a generic internal server error response.
func respondInternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, platformhttp.ErrorBody{
		Error:     "internal server error",
		Code:      string(platformhttp.ErrorCodeInternal),
		RequestID: requestIDFromContext(c),
	})
}

func errorCodeForHTTPStatus(status int) platformhttp.ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return platformhttp.ErrorCodeBadRequest
	case http.StatusUnauthorized:
		return platformhttp.ErrorCodeUnauthorized
	case http.StatusForbidden:
		return platformhttp.ErrorCodeForbidden
	case http.StatusNotFound:
		return platformhttp.ErrorCodeNotFound
	case http.StatusConflict:
		return platformhttp.ErrorCodeConflict
	default:
		return platformhttp.ErrorCodeInternal
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
