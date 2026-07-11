package vo

import "github.com/StephenQiu30/hotkey-server/internal/model/enum"

// ResponseBody is the unified success response format.
type ResponseBody struct {
	Code      enum.ErrorCode `json:"code"`
	Message   string         `json:"message"`
	Data      any            `json:"data"`
	RequestID string         `json:"request_id"`
}

// PageBody is the unified paginated response format.
type PageBody struct {
	Code      enum.ErrorCode `json:"code"`
	Message   string         `json:"message"`
	Data      any            `json:"data"`
	Page      int            `json:"page"`
	PageSize  int            `json:"page_size"`
	Total     int            `json:"total"`
	RequestID string         `json:"request_id"`
}
