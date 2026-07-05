## Why

当前系统仅支持 X (Twitter) 单一数据源的热点采集，无法覆盖中文互联网的主流热点信息（微博热搜、知乎热榜、百度热点）。用户需要扩展多平台支持、引入"热点事件"独立实体、优化项目工程结构，使系统真正成为可用的多源热点监控平台。

## What Changes

### 新能力
1. 多数据源采集：接入微博热榜、知乎热榜、百度热点（公开接口/网页爬取）
2. 热点事件独立实体（HotEvent）：Post → Topic → HotEvent 三层事件模型
3. 榜单采集 Job：定时轮询各平台排行榜，标准化入库
4. 跨平台热点聚合：余弦相似度 + 关键词交集的匹配算法，合并 X Topic 与国内榜单话题
5. 数据过期清理 Job：按策略定期清理旧数据
6. 项目工程结构重构：提取 connector 抽象层、job 按职责分包、wiring 注册器模式
7. HotEvent REST API：榜单查询、热点事件列表/详情

### 现有能力的修改
- `digest/export-orchestrator`：扩展每日日报，覆盖多平台热点事件（不仅仅是 X Topic）
- `database/models`：增加 hot_events、hot_event_platforms 等新表

### 非目标（明确不做）
- 不接入前端界面（仅开放 REST API）
- 不做指数热力图
- 不做实时 WebSocket 推送
- 不改动现有 X 采集/poll_monitor 核心逻辑
- 不改动现有 Jaccard 聚类逻辑
- 不改动现有通知/Alert 模块

## Capabilities

### New Capabilities
- `multi-platform`: 多数据源采集架构，含 connector 接口抽象和平台适配器
- `hot-event`: 热点事件实体、热度分计算、跨平台聚合
- `trending-collector`: 榜单定时采集 Job 框架
- `data-cleanup`: 历史数据过期清理策略
- `project-refactor`: 项目工程结构优化（connector 隔离、jobs 分包、wiring 注册器）
- `hot-event-api`: 热点事件 REST API

### Modified Capabilities
- `export-orchestrator`: 扩展每日日报，包含多平台热点事件摘要
- `daily-digest`: 数据源从 X Topic 扩展至 HotEvent

## Impact

- **新增代码**：`internal/connector/`、`internal/platform/weibo/`、`internal/platform/zhihu/`、`internal/platform/baidu/`、`internal/hotevent/`、`internal/collector/`、`internal/aggregator/`、`internal/cleanup/`
- **修改代码**：`app/worker_jobs.go`（注册器模式）、`database/models.go`（新表）、`http/`（新 handler）、`jobs/poll_monitor.go`（接口迁移）
- **迁移影响**：`PlatformConnector` 接口从 `internal/jobs/` 迁移到 `internal/connector/`，需确保旧引用更新
- **无外部依赖变更**：go.mod 不新增 SDK 依赖，全部 HTTP 直调
