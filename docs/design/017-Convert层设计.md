# Convert 层设计

> **日期：** 2026-07-09
> **状态：** 已批准

## 目标

引入 `internal/convert/` 层，将 Entity ↔ DTO ↔ VO 的转换逻辑从 repository 和 controller 中抽取到独立包中，解决当前转换函数分散、命名不统一、不可复用的问题。

## 现状问题

当前转换函数散落在两个层中：

### Entity → DTO（分散在 repository 中，和 DB 逻辑混在一起）

| 文件 | 函数 | 命名风格 |
|------|------|---------|
| `repository/user_repository.go` | `toAuthUser(m entity.User) dto.User` | `toAuthUser` |
| `repository/monitor_repository.go` | `toDomainMonitor(m entity.KeywordMonitor) dto.Monitor` | `toDomainMonitor` |
| `repository/notify_repository.go` | `toDomainNotification(m entity.UserNotification) dto.Notification` | `toDomainNotification` |
| `repository/hot_event_repository.go` | `toHotEvent(m entity.HotEvent) *dto.HotEvent` | `toHotEvent` |
| `repository/report_export_repository.go` | `toDTOReportExport(model entity.ReportExport) dto.ReportExport` | `toDTOReportExport` |

### DTO → VO（分散在 controller 中）

| 文件 | 形式 | 说明 |
|------|------|------|
| `controller/notify_controller.go` | `toNotificationResponse(n dto.Notification) vo.NotificationData` | 私有函数 |
| `controller/monitor_controller.go` | `monitorToResponse(m dto.Monitor) vo.MonitorData` | 私有函数 |
| `controller/auth_controller.go` | `vo.UserData{...}` 内联构造 | 没有抽取 |

### 问题

1. **转换和业务逻辑混合** — 转换函数嵌入在 repo/controller 文件中，和 DB 查询 / HTTP 响应混在一起
2. **命名不统一** — `toDomain*`, `toDTO*`, `to*` 三种命名混用
3. **不可复用** — 如果多个地方需要同一组转换，只能复制代码或 import 不相关的 repo 包
4. **不可单独测试** — 转换逻辑没有独立测试
5. **缺少 DTO → VO 的统一转换层** — 部分 VO 构造在 controller 内联，部分在私有函数

## 设计

### 包结构

新建 `internal/convert/` 包，按领域拆分为独立文件：

```
internal/convert/
├── auth_convert.go          UserEntity → dto.User, dto.User → vo.UserData/LoginData
├── monitor_convert.go       entity.KeywordMonitor → dto.Monitor, dto.Monitor → vo.MonitorData
├── notify_convert.go        entity.UserNotification → dto.Notification, dto.Notification → vo.NotificationData
├── hotevent_convert.go      entity.HotEvent/HotEventDetail → dto.HotEvent/HotEventDetail
├── report_convert.go        entity.ReportExport → dto.ReportExport
├── convert.go              (future) gob/JSON 注册等
```

### 转换方向

```
[Entity]  →  convert.*EntityToDTO  →  [DTO]  →  convert.*DTOToVO  →  [VO]
    ↑                                       ↑
  repository 引用                         controller 引用
```

### 函数命名规范

| 方向 | 命名 | 示例返回值 |
|------|------|-----------|
| Entity → DTO | `{Domain}EntityToDTO(entity) DTO` | `UserEntityToDTO(u entity.User) dto.User` |
| DTO → VO | `{Domain}DTOToVO(dto) VO` | `UserDTOToVO(u dto.User) vo.UserData` |

### 示例代码

#### `auth_convert.go`

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
        DisplayName:  u.DisplayName,
        PasswordHash: u.PasswordHash,
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

#### `monitor_convert.go`

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
        ID:                   m.ID,
        Name:                 m.Name,
        QueryText:            m.QueryText,
        Language:             m.Language,
        Region:               m.Region,
        Status:               m.Status,
        PollIntervalMinutes:  m.PollIntervalMinutes,
        AlertEnabled:         m.AlertEnabled,
        AlertThresholdConfig: m.AlertThresholdConfig,
        LastPolledAt:         m.LastPolledAt,
        CreatedAt:            m.CreatedAt,
        UpdatedAt:            m.UpdatedAt,
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

#### `notify_convert.go`

```go
package convert

import (
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
func NotificationDTOToVO(n dto.Notification) vo.NotificationData {
    return vo.NotificationData{
        ID:             n.ID,
        Channel:        n.Channel,
        DeliveryStatus: n.DeliveryStatus,
        ReadAt:         n.ReadAt,
    }
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

### 迁移方案

分为 5 个任务，逐步将旧调用替换为新 convert 函数，不改变任何业务逻辑：

| 任务 | 内容 |
|------|------|
| Task 1 | 创建 `convert/` 包和 5 个领域文件 |
| Task 2 | Auth 转换：repository + controller 替换 |
| Task 3 | Monitor 转换：repository + controller 替换 |
| Task 4 | Notify + ReportExport + HotEvent 转换替换 |
| Task 5 | 全量验证 + 清理旧代码和未使用 import |

### 不修改范围

- DTO、Entity、VO 的类型定义 — 只移动转换代码
- controller handler 的业务逻辑
- service 层的业务逻辑（不涉及 VO/entity 转换）
- repository 的 DB 查询逻辑

## 验证

1. `make build` 编译通过
2. `make test` 测试全部通过
3. `make lint` 静态检查通过
4. 转换函数类型签名与旧函数输出一致（无行为变更）
