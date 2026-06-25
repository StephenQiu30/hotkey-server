package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorBody is the unified error response format for all API endpoints.
type ErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

const internalErrorCode = "internal_error"

func newInternalErrorBody() ErrorBody {
	return ErrorBody{
		Error: "internal server error",
		Code:  internalErrorCode,
	}
}

func respondError(c *gin.Context, status int, message string) {
	body := ErrorBody{Error: message}
	if status >= http.StatusInternalServerError {
		body.Code = internalErrorCode
	}
	c.JSON(status, body)
}

func respondInternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, newInternalErrorBody())
}
