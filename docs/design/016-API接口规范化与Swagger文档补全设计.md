# API 接口规范化与 Swagger 文档补全设计

> **日期：** 2026-07-09
> **状态：** 已批准

## 目标

1. 将所有 Request 类型从 `controller/` 迁移到 `model/dto/`
2. 补全缺少 Swagger 注释的 handler（Report + HotEvent）
3. 统一 Trending/HotEvent 的响应格式
4. 保持整洁的包依赖关系

## 现状问题

### Request 类型位置不合理

当前 5 个请求类型定义在 `controller/request_controller.go`：

- `RegisterRequest`, `LoginRequest` → 应归入 `model/dto/`
- `CreateMonitorRequest`, `UpdateMonitorRequest` → 应归入 `model/dto/`
- `CreateReportRequest` → 定义在 `report_controller.go` 中，应归入 `model/dto/`

### Swagger 注释缺失

| 包 | Handler | 状态 |
|------|---------|--------|
| report/ | create, list, get, html, send (5个) | ❌ 无注释 |
| trending/ | list, list-hot, get-hot, get-posts (4个) | ❌ 无注释 |

### Trending 响应格式不一致

trending_controller.go 中的 4 个 handler 直接使用 `c.JSON(200, gin.H{"data": ...})`，没有走统一的 `RespondOK()` / `respondError()` 模式，缺少 `request_id` 字段。

## 设计

### 1. Request 类型迁移（model/dto/）

按领域拆分文件，每个 handler 引用 `dto.XxxRequest`：

```
model/dto/
├── auth_request.go       RegisterRequest, LoginRequest
├── monitor_request.go    CreateMonitorRequest, UpdateMonitorRequest
└── report_request.go     CreateReportRequest
```

controller handler 中的 `var body RegisterRequest` 改为 `var body dto.RegisterRequest`，移除 `request_controller.go`。

### 2. Swagger 响应类型分离

从 `request_controller.go` 提取到 `controller/swagger_response.go`。内容不变，仅文件拆分。保留在 controller 层是因为这些类型引用了 `vo.*`、`content.*`、`service.*`，不适合下沉到 vo/。

新增缺失的响应类型：

```go
type ReportResponse struct { ... }
type ReportListResponse struct { ... }
type TrendingListResponse struct { ... }
type HotEventResponse struct { ... }
type HotEventListResponse struct { ... }
type HotEventPostsResponse struct { ... }
```

### 3. Swagger 注释补全

每个 handler 增加标准 swagger 注释模板：

```go
// @Summary    简要描述
// @ID        handler-id
// @Tags      标签组
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     ...
// @Success   200 {object} XxxResponse
// @Failure   400 {object} platformhttp.ErrorBody
// @Failure   401 {object} platformhttp.ErrorBody
// @Failure   500 {object} platformhttp.ErrorBody
// @Router    /path [method]
```

### 4. Trending 响应格式统一

4 个 handler 从：
```go
c.JSON(http.StatusOK, gin.H{"data": items})
c.JSON(http.StatusBadRequest, gin.H{"error": "..."})
```
改为：
```go
RespondOK(c, items)
respondError(c, http.StatusBadRequest, "...")
```

## 不修改范围

- `model/entity/`, `model/enum/`, `model/vo/` 等已有清晰的包边界，不动
- `platform/http/errors.go` — 已有 `AppError` 和标准错误码，不需要改
- Controller 业务逻辑、DTO 字段定义——不动，仅改位置和注释

## 验证方式

1. `make build` 编译通过
2. `make test` 测试全部通过
3. `make lint` 静态检查通过
4. 检查 swagger 页面 `/swagger/index.html` 可正常加载
