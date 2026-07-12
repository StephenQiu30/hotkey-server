package vo

import "github.com/StephenQiu30/hotkey-server/internal/model/enum"

// ResponseBody is the unified success response format.
type ResponseBody struct {
	Code      int            `json:"code"`
	ErrorCode enum.ErrorCode `json:"error_code"`
	Data      any            `json:"data"`
}

// PageBody is the unified paginated response format.
type PageBody struct {
	Code      int            `json:"code"`
	ErrorCode enum.ErrorCode `json:"error_code"`
	Data      any            `json:"data"`
	Page      int            `json:"page"`
	PageSize  int            `json:"page_size"`
	Total     int            `json:"total"`
}
