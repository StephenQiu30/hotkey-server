package vo

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
