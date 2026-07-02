## Context

当前数据主线为 `platform_posts → topics (aggregation) → top N → publish_daily_topics → Obsidian Markdown`。该流水线以 Topic 为中心，缺乏独立的事件（Event）抽象层——一个热点话题可能包含多个子事件（如 AI 监管新规发布→行业解读→企业回应），而现有模型无法区分这些粒度。

同步基线当前为 `publish_daily_topics` 单一 job，直接操作 Obsidian 目录写入。知识中台需要将同步基线升级为可分步骤的通用流水线：Digest → Event Assembly → Export (with Theme/ExportBundle scoping)。

## Goals / Non-Goals

**Goals:**
- 新增 Event 主对象模型和对应 repository
- 新增 Topic ↔ Event 关联（1:N）
- 新增 Theme（主题策展）、ExportBundle（导出包）的最小定义
- 新增 event_annotations / topic_annotations 人工知识 sidecar 模型
- 新增 theme_memberships 多态关联
- 新增 knowledge_object_revisions 版本追踪
- 新增 KnowledgeRun 运行记录
- 实现 Event 领域服务的 BuildEventFromPosts
- 将 publish_daily_topics 改造为兼容适配层，内部委托给 PublishKnowledgeSnapshotJob
- 冻结 Event / Revision 的最小契约输出

**Non-Goals:**
- 不实现最终 Vault 导出模板
- 不实现结构化回写应用层
- 不重新引入 `db/migrations/`
- 不把 Event 退化成 Topic 别名
- 不在这个阶段实现任意自由文本回写

## Decisions

### 1. Event 独立表 vs JSONB 嵌入 Topic

**决策**：独立 `events` 表 + `topic_events` 关联表。

- **原因**：Event 有自己的生命周期（first_seen_at / last_active_at）、状态（machine_status）、和后续 sidecar（event_annotations），嵌入 JSONB 使查询和更新复杂度上升
- **替代方案**：在 topics 表加 event_list JSONB → 查询不便，无法加 FK 约束

### 2. EventKey 构造

**决策**：`{seed-hash}:{date}` 格式，由 NormalizeEventKey 生成。

- **原因**：同一 seed 在相同日期产生的 EventKey 一致，支持幂等
- **唯一约束**：`(monitor_id, event_key)` 防止同 monitor 下事件重复

### 3. Topic:Event 基数

**决策**：1 个 Topic 可拥有多个 Events（1:N），通过 topic_events.relationship_type 区分关系。

- **原因**：一个热点话题通常包含多个时间粒度的事件（回应、发酵、政策落地等）
- Event 不反向引用 Topic，使用 topic_events 桥接表支持未来 M:N 扩展

### 4. Theme 与 ExportBundle 分离

**决策**：Theme 是持续存在的策展对象（人工或规则定义），ExportBundle 是一次导出操作的 scope 快照。

- **原因**：导出操作需要锁定当时的 scope（哪些 theme/event/topic），Theme 本身是活的、持续更新的

### 5. Revision 基于 SHA-256 前缀

**决策**：`hex(sha256(content)[:8])` 作为 revision token。

- **原因**：轻量、内容可寻址、无中心版本号依赖
- **唯一约束**：`(object_type, object_id)` 确保每个对象只有一个当前 revision

### 6. publish_daily_topics 改造为适配层

**决策**：保留旧函数签名，内部委托给 PublishKnowledgeSnapshotJob。

- **原因**：不破坏现有调度器入口（daily_scheduler），同时将新功能走新流水线
- **替代方案**：直接替换 → 现有调度器需修改，不符合最小变更原则

### 7. Sidecar 唯一约束

**决策**：event_annotations(event_id) 和 topic_annotations(topic_id) 使用 UNIQUE 约束。

- **原因**：1:1 关系，每个事件/主题只有一个人工知识记录
- 不使用 ON CONFLICT 在应用层控制，由数据库保证完整性

### 8. KnowledgeRun 运行记录

**决策**：独立 knowledge_runs 表记录每次同步调度。

- **原因**：提供可审计的同步历史，支持回放和排查
- run_key 唯一约束防止重复提交

## Risks / Trade-offs

- **[Event 识别精度]** BuildEventFromPosts 当前使用基于时间的简单策略 → 未来可能需 ML/NLP 识别事件边界。当前不做消歧，接受粗粒度合并
- **[Sidecar 空状态]** event_annotations 和 topic_annotations 为人工编辑输入，可能长期为空。应用层需正确处理 NULL/空默认值，避免 JSONB 解析错误
- **[Revision 碰撞]** 8 字符 SHA-256 前缀 (4 字节) 碰撞概率极低但理论存在 → 当前阶段可接受，后续可升级到完整 hash
- **[ExportBundle 空包]** 当所选 Theme/Event/Topic 无有效数据时，ExportBundle 可能为空 → 调用方需处理空结果
