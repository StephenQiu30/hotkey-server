package controller

import (
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// ErrorBody is a Swagger-visible copy of platformhttp.ErrorBody.
// swaggo cannot resolve cross-package type aliases, so we duplicate the struct.
type ErrorBody struct {
	Error     string `json:"error"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// HealthResponse wraps vo.HealthBody for swagger documentation.
type HealthResponse struct {
	Data      vo.HealthBody `json:"data"`
	RequestID string        `json:"request_id,omitempty"`
}

// UserResponse wraps vo.UserData for swagger documentation.
type UserResponse struct {
	Data      vo.UserData `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

// LoginResponse wraps vo.LoginData for swagger documentation.
type LoginResponse struct {
	Data      vo.LoginData `json:"data"`
	RequestID string       `json:"request_id,omitempty"`
}

// MonitorResponse wraps vo.MonitorData for swagger documentation.
type MonitorResponse struct {
	Data      vo.MonitorData `json:"data"`
	RequestID string         `json:"request_id,omitempty"`
}

// MonitorListResponse wraps a list of MonitorData for swagger documentation.
type MonitorListResponse struct {
	Data      []vo.MonitorData `json:"data"`
	RequestID string           `json:"request_id,omitempty"`
}

// PostListResponse wraps content.PostSummary for swagger documentation.
type PostListResponse struct {
	Data      []content.PostSummary `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
}

// TopicListResponse wraps service.TopicSummary for swagger documentation.
type TopicListResponse struct {
	Data      []service.TopicSummary `json:"data"`
	RequestID string                  `json:"request_id,omitempty"`
}

// TrendListResponse wraps service.TrendPoint for swagger documentation.
type TrendListResponse struct {
	Data      []service.TrendPoint `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
}

// NotificationListResponse wraps vo.NotificationData for swagger documentation.
type NotificationListResponse struct {
	Data      []vo.NotificationData `json:"data"`
	RequestID string                 `json:"request_id,omitempty"`
}

// MarkNotificationReadResponse wraps vo.MarkNotificationReadData for swagger documentation.
type MarkNotificationReadResponse struct {
	Data      vo.MarkNotificationReadData `json:"data"`
	RequestID string                       `json:"request_id,omitempty"`
}

// ReportResponse wraps a report for swagger documentation.
type ReportResponse struct {
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

// ReportListResponse wraps a paginated list of reports for swagger documentation.
type ReportListResponse struct {
	Data      interface{} `json:"data"`
	Page      int         `json:"page"`
	PageSize  int         `json:"page_size"`
	Total     int         `json:"total"`
	RequestID string      `json:"request_id,omitempty"`
}

// TrendingListResponse wraps a trending items list for swagger documentation.
type TrendingListResponse struct {
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

// HotEventResponse wraps a single hot event for swagger documentation.
type HotEventResponse struct {
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}

// HotEventListResponse wraps a hot event list with total count for swagger documentation.
type HotEventListResponse struct {
	Data      interface{} `json:"data"`
	Meta      interface{} `json:"meta"`
	RequestID string      `json:"request_id,omitempty"`
}

// HotEventPostsResponse wraps event posts for swagger documentation.
type HotEventPostsResponse struct {
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id,omitempty"`
}
