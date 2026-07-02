## Why

当前仓库的 `platform_posts → topics → publish_daily_topics` 主线只覆盖了话题级别的日报导出，缺少 **Event（事件）** 抽象层。知识中台需要新增 Event 主对象来聚合单次热点事件的多个帖子，同时冻结 Theme（主题策展）、ExportBundle（导出包）、人工标注 sidecar、revision 追踪等边界，使得后续 Vault 导出与结构化回写不会从错误抽象起步。

## What Changes

- **新增 `events` 表**：独立 Event 主对象，monitor_id + event_key 唯一约束，含 first_seen_at / last_active_at 时间窗口
- **新增 `topic_events` 关联表**：连接 Event → Topic，1:N 关系（一个 Topic 可包含多个 Events）
- **新增 `knowledge_runs` 表**：跟踪每次知识同步运行的记录（运行类型、目标日期、状态、错误信息）
- **新增 `themes` 表**：主题策展对象，绑定到 monitor（一个 monitor 可定义多个 themes）
- **新增 `export_bundles` 表**：导出包种子，记录一次导出的 scope（bundle_kind、时间范围、状态）
- **新增 sidecar 表**：
  - `event_annotations`：人工标签和分析结论（event_id 唯一）
  - `topic_annotations`：素材研判状态和人工摘要（topic_id 唯一）
- **新增 `theme_memberships` 表**：多对多连接 Theme → Event / Topic
- **新增 `knowledge_object_revisions` 表**：对象级 revision 追踪，用于冲突检测
- **重构 `publish_daily_topics` 为知识同步基线的兼容适配层**，不再作为唯一知识对象主线
- **Event 领域服务**：提供 BuildEventFromPosts，确保 Event 不等于 Topic 标题别名
- **EventRepo**：基本 CRUD 实现
- **契约层**：冻结 Event / Revision 的最小输出格式

## Capabilities

### New Capabilities
- `knowledge-model`: Event 主对象、Topic/Event 基数、Theme/ExportBundle 的最小定义、人工知识 sidecar 模型、revision contract

### Modified Capabilities
- `daily-digest`: 原 `publish_daily_topics` 将从唯一知识对象主线改造为知识同步基线的兼容适配层，内部委托给新的 PublishKnowledgeSnapshotJob

## Impact

- `db/schema.sql`：新增 9 张表，不修改现有表结构
- `internal/database/models.go`：新增 GORM 模型
- `internal/database/`：新增 `eventrepo.go`（EventRepo）
- `internal/event/`：新建领域服务包
- `internal/topic/`：新增 TopicEventLinker
- `internal/jobs/`：新增 `publish_knowledge_snapshot.go`，保留 `publish_daily_topics.go` 作为兼容适配
- `internal/obsidian/`：新增 `contracts.go`（契约构造器）
- **不重新引入 `db/migrations/`**
- **不实现最终 Vault 导出模板**
- **不在这个阶段实现任意自由文本回写**
