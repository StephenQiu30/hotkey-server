# knowledge-model Specification

## Purpose

定义知识中台的核心知识模型：Event 主对象、Theme/ExportBundle 最小定义、人工知识 sidecar、revision contract。这些模型构成知识同步、导出和回写的基础抽象层。

## ADDED Requirements

### Requirement: Event 主对象

系统 SHALL 定义 `events` 表作为知识中台的独立事件主对象。

- `monitor_id` SHALL 引用 `keyword_monitors(id)`
- `event_key` SHALL 在同一 monitor_id 下唯一
- `title` SHALL 为事件标题，不自动坍缩为关联 topic 的标题
- `first_seen_at` 和 `last_active_at` SHALL 基于关联帖子的发布时间计算
- `machine_status` SHALL 支持 `active` | `inactive` | `archived` 状态值

#### Scenario: Event 创建成功
- **WHEN** 传入有效的 CreateEventInput（monitor_id, event_key, title, first_seen_at, last_active_at）
- **THEN** EventRepo.CreateEvent 返回非零 id，events 表新增一行记录

#### Scenario: Event 不能是 Topic 标题别名
- **WHEN** 使用 Topic 标题 "AI 监管" 创建 Event
- **THEN** Event title 不能自动坍缩等于 Topic title

### Requirement: Topic-Event 关联

系统 SHALL 定义 `topic_events` 表实现 Topic ↔ Event 的 N:1 关联。

- `relationship_type` SHALL 默认值为 `member`
- `(topic_id, event_id)` SHALL 为唯一约束
- 1 个 Topic SHALL 关联 0..N 个 Events

#### Scenario: 关联 Topic 和 Event
- **WHEN** TopicEventLinker.LinkEvent 传入有效的 topic_id 和 event_id
- **THEN** topic_events 表新增关联记录，不报错

#### Scenario: 重复关联被拒绝
- **WHEN** 对同一 (topic_id, event_id) 重复调用 LinkEvent
- **THEN** 返回唯一约束冲突错误

### Requirement: Theme（主题策展）

系统 SHALL 定义 `themes` 表作为主题策展对象。

- `theme_key` SHALL 在同一 monitor_id 下唯一
- Theme SHALL 绑定到特定 monitor，通过 `theme_memberships` 关联 Events 和 Topics
- 一个 Event 或 Topic SHALL 可同时属于多个 Themes

#### Scenario: 创建 Theme
- **WHEN** 传入有效的 monitor_id 和 theme_key
- **THEN** themes 表新增一行记录，包含唯一的 theme_key

### Requirement: ExportBundle（导出包）

系统 SHALL 定义 `export_bundles` 表作为导出操作的 scope 快照。

- `bundle_kind` SHALL 标识导出类型（如 `daily-obsidian`, `weekly-vault`）
- `date_start` / `date_end` SHALL 定义导出时间窗口
- `status` SHALL 支持 `pending` | `processing` | `completed` | `failed`

#### Scenario: 创建 ExportBundle
- **WHEN** 传入 monitor_id, bundle_key, bundle_kind, 和时间窗口
- **THEN** export_bundles 表新增一行记录，status 初始为 `pending`

### Requirement: 人工知识 Sidecar

系统 SHALL 定义 `event_annotations` 和 `topic_annotations` 作为人工输入扩展表。

- `event_annotations` SHALL 以 `event_id` 为 UNIQUE，含 `manual_tags` (jsonb) 和 `analyst_conclusion`
- `topic_annotations` SHALL 以 `topic_id` 为 UNIQUE，含 `material_status` 和 `manual_summary`
- `material_status` SHALL 支持 `unreviewed` | `reviewed` | `confirmed` | `rejected`

#### Scenario: 创建 Event Annotation
- **WHEN** 为存在的 event 插入 event_annotations 行
- **THEN** manual_tags 默认值为 `[]`，analyst_conclusion 默认为空字符串

#### Scenario: 创建 Topic Annotation
- **WHEN** 为存在的 topic 插入 topic_annotations 行
- **THEN** material_status 默认值为 `unreviewed`，manual_summary 默认为空字符串

### Requirement: ThemeMemberships 多态关联

系统 SHALL 定义 `theme_memberships` 表实现 Theme 与 Event/Topic 的多态关联。

- `source_kind` SHALL 区分关联的目标类型（`event` 或 `topic`）
- `event_id` 或 `topic_id` SHALL 至少一个不为 NULL

#### Scenario: Theme 关联 Event
- **WHEN** 向 theme_memberships 插入 (theme_id, event_id, source_kind='event')
- **THEN** 关联记录生效，查询该 theme 的 events 时返回该 event

### Requirement: KnowledgeObjectRevisions 版本追踪

系统 SHALL 定义 `knowledge_object_revisions` 表追踪知识对象的当前版本。

- `revision` SHALL 为基于内容 SHA-256 前缀（8 hex chars）的 revision token
- `(object_type, object_id)` SHALL 为唯一约束
- `source_path` SHALL 记录导出或同步的源路径

#### Scenario: 创建 Revision
- **WHEN** 为知识对象计算 revision
- **THEN** revision 格式为 `{object_type}:{object_id}:{sha256_prefix}`
- **THEN** unique 约束确保同一对象只有一个当前 revision

### Requirement: KnowledgeRun 运行记录

系统 SHALL 定义 `knowledge_runs` 表跟踪每次知识同步运行的记录。

- `run_key` SHALL 唯一标识一次运行
- `run_type` SHALL 标识运行类型（如 `daily-digest`, `event-sync`）
- `status` SHALL 支持 `pending` | `running` | `completed` | `failed`
- `error_message` SHALL 记录失败原因

#### Scenario: 记录成功运行
- **WHEN** 知识同步任务完成
- **THEN** knowledge_runs 新增一行，status 为 `completed`，error_message 为空

#### Scenario: 记录失败运行
- **WHEN** 知识同步任务失败
- **THEN** knowledge_runs 新增一行，status 为 `failed`，error_message 包含错误描述
