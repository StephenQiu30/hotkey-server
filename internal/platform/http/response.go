package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ResponseBody is the unified success response format.
type ResponseBody struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id,omitempty"`
}

// PageBody is the unified paginated response format.
type PageBody struct {
	Data      any    `json:"data"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
	Total     int    `json:"total"`
	RequestID string `json:"request_id,omitempty"`
}

// RespondOK writes a successful response using the unified response contract.
func RespondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, ResponseBody{
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondCreated writes a created response using the unified response contract.
func RespondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, ResponseBody{
		Data:      data,
		RequestID: requestIDFromContext(c),
	})
}

// RespondPage writes a paginated response using the unified response contract.
func RespondPage(c *gin.Context, data any, page, pageSize, total int) {
	c.JSON(http.StatusOK, PageBody{
		Data:      data,
		Page:      page,
		PageSize:  pageSize,
		Total:     total,
		RequestID: requestIDFromContext(c),
	})
}
