# Convert 层实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 创建 `internal/convert/` 转换层，将 Entity ↔ DTO ↔ VO 的转换逻辑从 repository 和 controller 中抽取到独立包中。

**Architecture:** 按领域拆分为 5 个 convert 文件，每个文件包含 Entity→DTO（供 repository 调用）和 DTO→VO（供 controller 调用）两组导出函数。旧代码中的私有转换函数和内联构造逐步替换为 convert 包调用。

**Tech Stack:** Go 1.26

---

### Task 1: 创建 convert/ 包 + 5 个领域转换文件

**Files:**
- Create: `internal/convert/auth_convert.go`
- Create: `internal/convert/monitor_convert.go`
- Create: `internal/convert/notify_convert.go`
- Create: `internal/convert/hotevent_convert.go`
- Create: `internal/convert/report_convert.go`

- [ ] **Step 1: Create `internal/convert/auth_convert.go`**

`internal/convert/auth_convert.go`:

```go
package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

// UserEntityToDTO converts a User entity to a User DTO.
func UserEntityToDTO(u entity.User) dto.User {
	return dto.User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		DisplayName:  u.DisplayName,
		Status:       u.Status,
		PlanType:     u.PlanType,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// UserDTOToVO converts a User DTO to a UserData VO.
func UserDTOToVO(u dto.User) vo.UserData {
	return vo.UserData{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
	}
}

// LoginDTOToVO converts a User DTO + token to a LoginData VO.
func LoginDTOToVO(u dto.User, token string) vo.LoginData {
	return vo.LoginData{
		User:  UserDTOToVO(u),
		Token: token,
	}
}
```

- [ ] **Step 2: Create `internal/convert/monitor_convert.go`**

`internal/convert/monitor_convert.go`:

```go
package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

// MonitorEntityToDTO converts a KeywordMonitor entity to a Monitor DTO.
func MonitorEntityToDTO(m entity.KeywordMonitor) dto.Monitor {
	return dto.Monitor{
		ID:                   m.ID,
		UserID:               m.UserID,
		Name:                 m.Name,
		QueryText:            m.QueryText,
		Language:             m.Language,
		Region:               m.Region,
		Status:               m.Status,
		PollIntervalMinutes:  m.PollIntervalMinutes,
		AlertEnabled:         m.AlertEnabled,
		AlertThresholdConfig: m.AlertThresholdConfig.Data,
		LastPolledAt:         m.LastPolledAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// MonitorDTOToVO converts a Monitor DTO to a MonitorData VO.
func MonitorDTOToVO(m dto.Monitor) vo.MonitorData {
	return vo.MonitorData{
		ID:                  m.ID,
		UserID:              m.UserID,
		Name:                m.Name,
		QueryText:           m.QueryText,
		Language:            m.Language,
		Region:              m.Region,
		Status:              m.Status,
		PollIntervalMinutes: m.PollIntervalMinutes,
		AlertEnabled:        m.AlertEnabled,
	}
}

// MonitorSliceDTOToVO converts a slice of Monitor DTO to a slice of MonitorData VO.
func MonitorSliceDTOToVO(ms []dto.Monitor) []vo.MonitorData {
	result := make([]vo.MonitorData, len(ms))
	for i, m := range ms {
		result[i] = MonitorDTOToVO(m)
	}
	return result
}
```

- [ ] **Step 3: Create `internal/convert/notify_convert.go`**

`internal/convert/notify_convert.go`:

```go
package convert

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

// NotificationEntityToDTO converts a UserNotification entity to a Notification DTO.
func NotificationEntityToDTO(n entity.UserNotification) dto.Notification {
	return dto.Notification{
		ID:             n.ID,
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
		ReadAt:         n.ReadAt,
		SentAt:         n.SentAt,
		CreatedAt:      n.CreatedAt,
	}
}

// NotificationDTOToVO converts a Notification DTO to a NotificationData VO.
// time.Time fields are formatted as RFC3339 strings for JSON output.
func NotificationDTOToVO(n dto.Notification) vo.NotificationData {
	r := vo.NotificationData{
		ID:             n.ID,
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
		CreatedAt:      n.CreatedAt.Format(time.RFC3339),
	}
	if n.ReadAt != nil {
		s := n.ReadAt.Format(time.RFC3339)
		r.ReadAt = &s
	}
	return r
}

// NotificationSliceDTOToVO converts a slice of Notification DTO to NotificationData VOs.
func NotificationSliceDTOToVO(ns []dto.Notification) []vo.NotificationData {
	result := make([]vo.NotificationData, len(ns))
	for i, n := range ns {
		result[i] = NotificationDTOToVO(n)
	}
	return result
}
```

- [ ] **Step 4: Create `internal/convert/hotevent_convert.go`**

`internal/convert/hotevent_convert.go`:

```go
package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// HotEventEntityToDTO converts a HotEvent entity to a HotEvent DTO pointer.
func HotEventEntityToDTO(m entity.HotEvent) *dto.HotEvent {
	return &dto.HotEvent{
		ID:          m.ID,
		Name:        m.Name,
		HeatScore:   m.HeatScore,
		Platform:    m.Platform,
		Trend:       m.Trend,
		FirstSeenAt: m.FirstSeenAt,
		LastSeenAt:  m.LastSeenAt,
		PeakAt:      m.PeakAt,
		TopicIDs:    fromInt64Array(m.TopicIDs),
		PostIDs:     fromInt64Array(m.PostIDs),
		Summary:     m.Summary,
		Category:    m.Category,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// HotEventSliceEntityToDTO converts a slice of HotEvent entities to HotEvent DTO pointers.
func HotEventSliceEntityToDTO(models []entity.HotEvent) []*dto.HotEvent {
	events := make([]*dto.HotEvent, len(models))
	for i, m := range models {
		events[i] = HotEventEntityToDTO(m)
	}
	return events
}

func fromInt64Array(src pkg.Int64Array) []int64 {
	return []int64(src)
}
```

- [ ] **Step 5: Create `internal/convert/report_convert.go`**

`internal/convert/report_convert.go`:

```go
package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
)

// ReportExportEntityToDTO converts a ReportExport entity to a ReportExport DTO.
func ReportExportEntityToDTO(model entity.ReportExport) dto.ReportExport {
	return dto.ReportExport{
		ID:           model.ID,
		ReportID:     model.ReportID,
		ExportKind:   model.ExportKind,
		TargetPath:   model.TargetPath,
		Status:       model.Status,
		ErrorMessage: model.ErrorMessage,
		PublishedAt:  model.PublishedAt,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}
```

- [ ] **Step 6: Verify build**

Run: `make build`
Expected: success (files compile, no callers yet so no unused-function warnings)

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: add convert layer (auth/monitor/notify/hotevent/report converters)"
```

---

### Task 2: Auth 转换替换 — repository + controller

**Files:**
- Modify: `internal/repository/user_repository.go`
- Modify: `internal/controller/auth_controller.go`

- [ ] **Step 1: Replace `toAuthUser` in user_repository.go**

In `internal/repository/user_repository.go`, import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Delete the `toAuthUser` function (lines 62-73):
```go
func toAuthUser(m entity.User) dto.User {
	return dto.User{...}
}
```

Replace calls to `toAuthUser(m)` with `convert.UserEntityToDTO(m)`:

Line 35: `return convert.UserEntityToDTO(m), nil`
Line 46: `result := convert.UserEntityToDTO(m)`
Line 58: `result := convert.UserEntityToDTO(m)`

After the change, verify the import block includes `convert`:

```go
import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/convert"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
)
```

- [ ] **Step 2: Replace VO constructions in auth_controller.go**

In `internal/controller/auth_controller.go`, import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Replace line 57 (`registerHandler`):
```go
// Before:
RespondCreated(c, vo.UserData{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName})
// After:
RespondCreated(c, convert.UserDTOToVO(user))
```

Replace lines 106-109 (`loginHandler`):
```go
// Before:
RespondOK(c, vo.LoginData{
	User:  vo.UserData{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName},
	Token: tokenStr,
})
// After:
RespondOK(c, convert.LoginDTOToVO(user, tokenStr))
```

After the change, check if `vo` import is still needed in this file. If the only remaining use of `vo` is in swagger comments (type references in strings), it can be removed from the import block (but check first — it may still be used for `vo.UserData` in swagger `// @Success` inline references which are just strings, not imports needed).

- [ ] **Step 3: Verify build**

Run: `make build`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: migrate auth convert functions to convert package"
```

---

### Task 3: Monitor 转换替换 — repository + controller

**Files:**
- Modify: `internal/repository/monitor_repository.go`
- Modify: `internal/controller/monitor_controller.go`

- [ ] **Step 1: Replace `toDomainMonitor` in monitor_repository.go**

In `internal/repository/monitor_repository.go`, remove the `toDomainMonitor` function (lines 124-140).

Import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Find all calls to `toDomainMonitor(m)` and replace with `convert.MonitorEntityToDTO(m)`.

Search location calls (likely in `GetByID`, `Create`, `Update`, `ListByUser` methods):
```
toDomainMonitor(model)  →  convert.MonitorEntityToDTO(model)
```

- [ ] **Step 2: Replace `monitorToResponse` and loop in monitor_controller.go**

In `internal/controller/monitor_controller.go`, import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Delete `monitorToResponse` function (lines 27-34).

Replace line 60-63 (`listMonitorsHandler`):
```go
// Before:
resp := make([]vo.MonitorData, len(monitors))
for i, m := range monitors {
	resp[i] = monitorToResponse(m)
}
// After:
resp := convert.MonitorSliceDTOToVO(monitors)
```

Replace line 113 (`createMonitorHandler`):
```go
// Before:
RespondCreated(c, monitorToResponse(m))
// After:
RespondCreated(c, convert.MonitorDTOToVO(m))
```

Replace line 160 (`getMonitorHandler`):
```go
// Before:
RespondOK(c, monitorToResponse(m))
// After:
RespondOK(c, convert.MonitorDTOToVO(m))
```

Replace line 234 (`updateMonitorHandler`):
```go
// Before:
RespondOK(c, monitorToResponse(updated))
// After:
RespondOK(c, convert.MonitorDTOToVO(updated))
```

After the change, check if `vo` import is still needed in this file. If `vo.MonitorData` is not referenced directly anywhere else, remove the `vo` import.

- [ ] **Step 3: Verify build**

Run: `make build`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: migrate monitor convert functions to convert package"
```

---

### Task 4: Notify + HotEvent + ReportExport 转换替换

**Files:**
- Modify: `internal/repository/notify_repository.go`
- Modify: `internal/controller/notify_controller.go`
- Modify: `internal/repository/hot_event_repository.go`
- Modify: `internal/repository/report_export_repository.go`

- [ ] **Step 1: Replace `toDomainNotification` in notify_repository.go**

In `internal/repository/notify_repository.go`, import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Delete `toDomainNotification` function (lines 58-69).

Replace calls: `toDomainNotification(m)` → `convert.NotificationEntityToDTO(m)`

- [ ] **Step 2: Replace `toNotificationResponse` and loop in notify_controller.go**

In `internal/controller/notify_controller.go`, import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Delete `toNotificationResponse` function (lines 20-31).

Replace lines 57-60 (`listNotificationsHandler`):
```go
// Before:
result := make([]vo.NotificationData, len(items))
for i, n := range items {
	result[i] = toNotificationResponse(n)
}
// After:
result := convert.NotificationSliceDTOToVO(items)
```

If `vo` is no longer referenced elsewhere in the file, remove the `vo` import. If `time` is no longer referenced elsewhere, remove the `time` import.

- [ ] **Step 3: Replace `toHotEvent` in hot_event_repository.go**

In `internal/repository/hot_event_repository.go`, import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Delete `toHotEvent` function (lines 163-181) and `fromInt64Array` helper (line 185).

Delete `toInt64Array` function (line 183) — this is unused after removing `toHotEvent`; verify first with `grep -n "toInt64Array" hot_event_repository.go`.

Replace calls:
- `toHotEvent(m)` → `convert.HotEventEntityToDTO(m)`
- Loop (around line 98): call `convert.HotEventSliceEntityToDTO(models)` instead of building slice in a loop

Remove `pkg` import if `pkg.Int64Array` is no longer used directly in this file.

- [ ] **Step 4: Replace `toDTOReportExport` in report_export_repository.go**

In `internal/repository/report_export_repository.go`, import `"github.com/StephenQiu30/hotkey-server/internal/convert"`.

Delete `toDTOReportExport` function (lines 96-108).

Replace: `toDTOReportExport(model)` → `convert.ReportExportEntityToDTO(model)`

- [ ] **Step 5: Verify build**

Run: `make build`
Expected: success

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: migrate notify/hotevent/report convert functions to convert package"
```

---

### Task 5: 全量验证 + 清理

**Files:** (all modified files from Tasks 1-4)

- [ ] **Step 1: Run full validation suite**

Run:
```bash
make build && make lint && make test
```

Expected: all three pass

- [ ] **Step 2: Check for unused imports**

Run:
```bash
go vet ./internal/convert/...
go vet ./internal/repository/...
go vet ./internal/controller/...
```

Expected: no "unused import" warnings

If any "unused import" warnings appear, remove the unused import from the affected file. Common candidates:
- `internal/controller/auth_controller.go`: `vo` import may become unused
- `internal/controller/monitor_controller.go`: `vo` import may become unused
- `internal/controller/notify_controller.go`: `vo` and `time` imports may become unused
- `internal/repository/hot_event_repository.go`: `pkg` import may become unused

- [ ] **Step 3: Final commit if needed**

```bash
git add -A
git commit -m "chore: clean up unused imports after convert layer migration"
```
