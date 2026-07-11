package controller

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// _ is an anchor so swaggo can resolve {object} platformhttp.ErrorBody
// from annotations in the controller package without duplicating the struct.
var _ = platformhttp.ErrorBody{}

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
	Data      dto.Report `json:"data"`
	RequestID string     `json:"request_id,omitempty"`
}

// ReportListResponse wraps a paginated list of reports for swagger documentation.
type ReportListResponse struct {
	Data      []dto.Report `json:"data"`
	Page      int          `json:"page"`
	PageSize  int          `json:"page_size"`
	Total     int          `json:"total"`
	RequestID string       `json:"request_id,omitempty"`
}

// TrendingItem is a lightweight trending entry.
type TrendingItem struct {
	Platform string  `json:"platform"`
	Title    string  `json:"title"`
	Rank     int     `json:"rank"`
	Heat     float64 `json:"heat"`
	URL      string  `json:"url"`
}

// TrendingListResponse wraps a trending items list for swagger documentation.
type TrendingListResponse struct {
	Data      []TrendingItem `json:"data"`
	RequestID string         `json:"request_id,omitempty"`
}

// HotEventItem is a hot event list entry.
type HotEventItem struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	HeatScore float64 `json:"heat_score"`
	Platform  string  `json:"platform"`
	Trend     string  `json:"trend"`
	Summary   string  `json:"summary"`
	Category  string  `json:"category"`
	Status    string  `json:"status"`
}

// HotEventMeta holds list metadata.
type HotEventMeta struct {
	Total int64 `json:"total"`
}

// HotEventListResponse wraps a hot event list with total count for swagger documentation.
type HotEventListResponse struct {
	Data      []HotEventItem `json:"data"`
	Meta      HotEventMeta   `json:"meta"`
	RequestID string         `json:"request_id,omitempty"`
}

// EventPlatformItem is a hot event platform entry.
type EventPlatformItem struct {
	Platform string  `json:"platform"`
	Rank     int     `json:"rank"`
	Title    string  `json:"title"`
	URL      string  `json:"url"`
	Heat     float64 `json:"heat"`
}

// HotEventDetail is the full detail for a hot event.
type HotEventDetail struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	HeatScore   float64            `json:"heat_score"`
	Platform    string             `json:"platform"`
	Trend       string             `json:"trend"`
	FirstSeenAt time.Time          `json:"first_seen_at"`
	LastSeenAt  time.Time          `json:"last_seen_at"`
	Summary     string             `json:"summary"`
	Category    string             `json:"category"`
	Status      string             `json:"status"`
	Platforms   []EventPlatformItem `json:"platforms,omitempty"`
}

// HotEventResponse wraps a single hot event for swagger documentation.
type HotEventResponse struct {
	Data      HotEventDetail `json:"data"`
	RequestID string         `json:"request_id,omitempty"`
}

// HotEventPostsResponse wraps event posts for swagger documentation.
type HotEventPostsResponse struct {
	Data      []service.PostBrief `json:"data"`
	RequestID string              `json:"request_id,omitempty"`
}

// AuthTokenResponse wraps vo.AuthTokenData for swagger documentation.
type AuthTokenResponse struct {
	Data      vo.AuthTokenData `json:"data"`
	RequestID string           `json:"request_id,omitempty"`
}

// AuthenticatedUserResponse wraps vo.AuthenticatedUserData for swagger documentation.
type AuthenticatedUserResponse struct {
	Data      vo.AuthenticatedUserData `json:"data"`
	RequestID string                   `json:"request_id,omitempty"`
}

// VerificationSendResponse wraps a verification code send response for swagger documentation.
type VerificationSendResponse struct {
	Data      struct {
		Email   string `json:"email"`
		Message string `json:"message"`
	} `json:"data"`
	RequestID string `json:"request_id,omitempty"`
}

// VerificationTicketResponse wraps a verification ticket response for swagger documentation.
type VerificationTicketResponse struct {
	Data      struct {
		Ticket string `json:"ticket"`
	} `json:"data"`
	RequestID string `json:"request_id,omitempty"`
}
