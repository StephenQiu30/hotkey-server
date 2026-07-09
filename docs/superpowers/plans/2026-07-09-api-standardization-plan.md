# API 接口规范化与 Swagger 文档补全实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将所有 Request 类型移到 model/dto/，补全缺失的 9 个 handler Swagger 注释，统一 Trending 响应格式。

**Architecture:** Request 类型按领域拆分到 `model/dto/xxx_request.go`；Swagger 响应类型从 `request_controller.go` 提取到独立的 `swagger_response.go` (保留在 controller 层避免循环依赖)；Report (5个) + Trending (4个) handler 补标准 swagger 注释；Trending handler 统一使用 `RespondOK()`/`respondError()` 替代 `gin.H`。

**Tech Stack:** Go 1.26, Gin, swaggo (gin-swagger)

---

### Task 1: 创建 dto request 类型文件

**Files:**
- Create: `internal/model/dto/auth_request.go`
- Create: `internal/model/dto/monitor_request.go`
- Create: `internal/model/dto/report_request.go`

- [ ] **Step 1: Create `auth_request.go`**

```go
package dto

// RegisterRequest is the request body for POST /api/v1/auth/register.
type RegisterRequest struct {
	Email       string `json:"email" example:"user@example.com"`
	Password    string `json:"password" example:"Passw0rd!"`
	DisplayName string `json:"display_name" example:"Stephen"`
}

// LoginRequest is the request body for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email" example:"user@example.com"`
	Password string `json:"password" example:"Passw0rd!"`
}
```

- [ ] **Step 2: Create `monitor_request.go`**

```go
package dto

// CreateMonitorRequest is the request body for POST /api/v1/monitors.
type CreateMonitorRequest struct {
	Name                string `json:"name" example:"AI monitor"`
	QueryText           string `json:"query_text" example:"openai OR gpt"`
	Language            string `json:"language,omitempty" example:"en"`
	Region              string `json:"region,omitempty" example:"US"`
	PollIntervalMinutes int    `json:"poll_interval_minutes" example:"15"`
	AlertEnabled        bool   `json:"alert_enabled" example:"true"`
}

// UpdateMonitorRequest is the request body for PATCH /api/v1/monitors/:id.
type UpdateMonitorRequest struct {
	Name                *string `json:"name,omitempty" example:"AI monitor"`
	QueryText           *string `json:"query_text,omitempty" example:"openai OR gpt"`
	Language            *string `json:"language,omitempty" example:"en"`
	Region              *string `json:"region,omitempty" example:"US"`
	PollIntervalMinutes *int    `json:"poll_interval_minutes,omitempty" example:"15"`
	AlertEnabled        *bool   `json:"alert_enabled,omitempty" example:"true"`
	Status              *string `json:"status,omitempty" example:"active"`
}
```

- [ ] **Step 3: Create `report_request.go`**

```go
package dto

import "time"

// CreateReportRequest is the request body for POST /api/v1/reports.
type CreateReportRequest struct {
	ReportType  string `json:"report_type" example:"weekly"`
	PeriodStart string `json:"period_start,omitempty" example:"2026-06-24"`
	PeriodEnd   string `json:"period_end,omitempty" example:"2026-06-30"`
	Send        bool   `json:"send" example:"false"`
}

// ToInput converts the request into a CreateInput for the service layer.
func (r CreateReportRequest) ToInput() (CreateInput, error) {
	var start *time.Time
	if r.PeriodStart != "" {
		parsed, err := time.Parse("2006-01-02", r.PeriodStart)
		if err != nil {
			return CreateInput{}, err
		}
		start = &parsed
	}
	var end *time.Time
	if r.PeriodEnd != "" {
		parsed, err := time.Parse("2006-01-02", r.PeriodEnd)
		if err != nil {
			return CreateInput{}, err
		}
		end = &parsed
	}
	return CreateInput{
		ReportType:  r.ReportType,
		PeriodStart: start,
		PeriodEnd:   end,
		Send:        r.Send,
	}, nil
}
```

Note: `toInput()` 方法从 report_controller.go 的 `CreateReportRequest` 类型上迁移过来。

- [ ] **Step 4: Verify build**

```bash
make build
# Expected: success
```

- [ ] **Step 5: Commit**

```bash
git add internal/model/dto/auth_request.go internal/model/dto/monitor_request.go internal/model/dto/report_request.go
git commit -m "impl: add dto request types (auth/monitor/report)"
```

---

### Task 2: 更新 controller handler 引用 dto 类型 + 提取 swagger_response.go

**Files:**
- Modify: `internal/controller/auth_controller.go`
- Modify: `internal/controller/monitor_controller.go`
- Modify: `internal/controller/report_controller.go`
- Create: `internal/controller/swagger_response.go`
- Delete: `internal/controller/request_controller.go` (extract its swagger response types first)

- [ ] **Step 1: Create `swagger_response.go`** — 提取 request_controller.go 中的响应类型 + 新增缺失类型

```go
package controller

import (
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

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
	RequestID string                 `json:"request_id,omitempty"`
}

// TrendListResponse wraps service.TrendPoint for swagger documentation.
type TrendListResponse struct {
	Data      []service.TrendPoint `json:"data"`
	RequestID string               `json:"request_id,omitempty"`
}

// NotificationListResponse wraps vo.NotificationData for swagger documentation.
type NotificationListResponse struct {
	Data      []vo.NotificationData `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
}

// MarkNotificationReadResponse wraps vo.MarkNotificationReadData for swagger documentation.
type MarkNotificationReadResponse struct {
	Data      vo.MarkNotificationReadData `json:"data"`
	RequestID string                      `json:"request_id,omitempty"`
}

// --- Newly added response types for Report and Trending ---

// ReportResponse wraps a dto.Report for swagger documentation.
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
```

- [ ] **Step 2: Update `auth_controller.go`** — 引用 `dto.RegisterRequest` / `dto.LoginRequest`

```go
// 在 registerHandler 中
var body dto.RegisterRequest

// 在 loginHandler 中
var body dto.LoginRequest
```

移除顶部 import 中不必要的部分，确保 `"github.com/StephenQiu30/hotkey-server/internal/model/dto"` 仍在 import 中（已经在了）。

- [ ] **Step 3: Update `monitor_controller.go`** — 引用 `dto.CreateMonitorRequest` / `dto.UpdateMonitorRequest`

```go
// 在 createMonitorHandler 中
var body dto.CreateMonitorRequest

// 在 updateMonitorHandler 中
var body dto.UpdateMonitorRequest
```

- [ ] **Step 4: Update `report_controller.go`** — 引用 `dto.CreateReportRequest`

将 handler 中 `var req CreateReportRequest` 改为 `var req dto.CreateReportRequest`，调用改为 `req.ToInput()`：

```go
var req dto.CreateReportRequest
if err := c.ShouldBindJSON(&req); err != nil {
    respondError(c, http.StatusBadRequest, "invalid input")
    return
}
input, err := req.ToInput()
```

移除 report_controller.go 中的 `CreateReportRequest` 结构体定义及其 `toInput()` 方法。

- [ ] **Step 5: Remove `request_controller.go`**

```bash
git rm internal/controller/request_controller.go
```

- [ ] **Step 6: Verify build**

```bash
make build
# Expected: success
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move request types to dto/, extract swagger response types"
```

---

### Task 3: 补全 Report 5 个 handler 的 Swagger 注释

**Files:**
- Modify: `internal/controller/report_controller.go`

- [ ] **Step 1: Add swagger comment to `createReportHandler`**

```go
// createReportHandler godoc
// @Summary Create a report
// @ID create-report
// @Tags reports
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body dto.CreateReportRequest true "Report creation payload"
// @Success 201 {object} ReportResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports [post]
```

- [ ] **Step 2: Add swagger comment to `listReportsHandler`**

```go
// listReportsHandler godoc
// @Summary List reports
// @ID list-reports
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Max results" default(50)
// @Param offset query int false "Offset" default(0)
// @Param report_type query string false "Filter by report type (daily|weekly)"
// @Success 200 {object} ReportListResponse
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports [get]
```

- [ ] **Step 3: Add swagger comment to `getReportHandler`**

```go
// getReportHandler godoc
// @Summary Get a report by ID
// @ID get-report
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "Report ID"
// @Success 200 {object} ReportResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports/{id} [get]
```

- [ ] **Step 4: Add swagger comment to `getReportHTMLHandler`**

```go
// getReportHTMLHandler godoc
// @Summary Get report as HTML
// @ID get-report-html
// @Tags reports
// @Produce html
// @Security BearerAuth
// @Param id path int true "Report ID"
// @Success 200 {string} string "HTML content"
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports/{id}/html [get]
```

- [ ] **Step 5: Add swagger comment to `sendReportHandler`**

```go
// sendReportHandler godoc
// @Summary Mark and send a report
// @ID send-report
// @Tags reports
// @Produce json
// @Security BearerAuth
// @Param id path int true "Report ID"
// @Success 200 {object} ReportResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 401 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/reports/{id}/send [post]
```

- [ ] **Step 6: Verify build**

```bash
make build
# Expected: success
```

- [ ] **Step 7: Commit**

```bash
git add internal/controller/report_controller.go
git commit -m "docs: add swagger annotations to report handlers"
```

---

### Task 4: 补全 Trending/HotEvent 4 个 handler 的 Swagger 注释 + 修复响应格式

**Files:**
- Modify: `internal/controller/trending_controller.go`

- [ ] **Step 1: Add swagger comment + fix response format in `listTrendingHandler`**

Replace `c.JSON(http.StatusOK, gin.H{"data": items})` with `RespondOK(c, items)`:

```go
// listTrendingHandler godoc
// @Summary List trending hot events across platforms
// @ID list-trending
// @Tags trending
// @Produce json
// @Param platform query string false "Platform filter"
// @Param limit query int false "Max results" default(20)
// @Success 200 {object} TrendingListResponse
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/trending [get]
```

- [ ] **Step 2: Add swagger comment + fix response format in `listHotEventsHandler`**

Replace `c.JSON(http.StatusOK, gin.H{"data": items, "meta": gin.H{"total": total}})` with a structured response. Use `RespondOK(c, map[string]interface{}{"items": items, "total": total})`:

```go
// listHotEventsHandler godoc
// @Summary List hot events with filter and pagination
// @ID list-hot-events
// @Tags hot-events
// @Produce json
// @Param status query string false "Status filter" default(active)
// @Param platform query string false "Platform filter"
// @Param sort query string false "Sort field" default(heat_score)
// @Param limit query int false "Max results" default(20)
// @Success 200 {object} HotEventListResponse
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/hot-events [get]
```

Replace gin.H error responses:
```go
// 替换:
_ = c.Error(fmt.Errorf("list trending: %w", err))
c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trending data"})
// 改为:
respondInternalError(c)

// 类似地:
_ = c.Error(fmt.Errorf("list hot events: %w", err))
c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch hot events"})
// 改为:
respondInternalError(c)
```

- [ ] **Step 3: Add swagger comment + fix response format in `getHotEventHandler`**

```go
// getHotEventHandler godoc
// @Summary Get a hot event by ID with platform details
// @ID get-hot-event
// @Tags hot-events
// @Produce json
// @Param id path int true "Hot Event ID"
// @Success 200 {object} HotEventResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/hot-events/{id} [get]
```

Replace:
```go
// 替换:
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event id"})
c.JSON(http.StatusNotFound, gin.H{"error": "hot event not found"})
c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch hot event"})
c.JSON(http.StatusOK, gin.H{"data": detail})
// 改为:
respondError(c, http.StatusBadRequest, "invalid event id")
respondError(c, http.StatusNotFound, "hot event not found")
respondInternalError(c)
RespondOK(c, detail)
```

- [ ] **Step 4: Add swagger comment + fix response format in `getHotEventPostsHandler`**

```go
// getHotEventPostsHandler godoc
// @Summary Get posts for a hot event
// @ID get-hot-event-posts
// @Tags hot-events
// @Produce json
// @Param id path int true "Hot Event ID"
// @Success 200 {object} HotEventPostsResponse
// @Failure 400 {object} platformhttp.ErrorBody
// @Failure 404 {object} platformhttp.ErrorBody
// @Failure 500 {object} platformhttp.ErrorBody
// @Router /api/v1/hot-events/{id}/posts [get]
```

Replace gin.H responses with `respondError` / `RespondOK` (same pattern as step 3).

- [ ] **Step 5: Update import in trending_controller.go**

Ensure the file imports `"github.com/StephenQiu30/hotkey-server/internal/platform/http"` (for `platformhttp.ErrorBody` reference in swagger comments). It likely already imports relevant packages. Remove unused `"fmt"` if it's only used for error logging.

- [ ] **Step 6: Verify build**

```bash
make build
# Expected: success
```

- [ ] **Step 7: Commit**

```bash
git add internal/controller/trending_controller.go
git commit -m "docs: add swagger annotations to trending handlers, unify response format"
```

---

### Task 5: 全量验证 + 推送

**Files:** (全部修改文件)

- [ ] **Step 1: Run full CI suite**

```bash
make build && make lint && make test
# Expected: all green
```

- [ ] **Step 2: Push to GitHub**

```bash
git push origin main
```

- [ ] **Step 3: Wait for CI check**

```bash
gh run watch --branch main --exit-status
# Expected: all jobs green
```
