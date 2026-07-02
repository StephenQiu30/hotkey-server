# daily-digest Specification — Knowledge Baseline Delta

## MODIFIED Requirements

### Requirement: 每日调度

系统 SHALL 使用北京时间（`Asia/Shanghai`）定义自然日窗口。

- `export_date = D` 的窗口为 `[D 00:00 CST, D+1 00:00 CST)`
- `DAILY_DIGEST_TARGET=yesterday` 时，`D` 为当前 CST 日期前一天
- `DAILY_DIGEST_TARGET=today` 时，`D` 为当前 CST 日期
- `publish_daily_topics` SHALL 保留为历史兼容入口，内部委托给 `PublishKnowledgeSnapshotJob`

#### Scenario: publish_daily_topics 委托给新 job
- **WHEN** publish_daily_topics job 被调度执行
- **THEN** 它 SHALL 内部调用 PublishKnowledgeSnapshotJob 而非直接执行 digest 逻辑
- **THEN** KnowledgeRun 记录本次执行结果

## ADDED Requirements

### Requirement: 知识同步基线

系统 SHALL 定义 `PublishKnowledgeSnapshotJob` 作为新的知识同步主线。

- Job SHALL 分三步执行：BuildDigest → BuildEvents → Publish
- KnowledgeRun SHALL 记录每次执行的 run_key、status、error_message
- Job SHALL 在导出前检查 Theme / ExportBundle 的最小契约

#### Scenario: 知识同步流水线成功执行
- **WHEN** PublishKnowledgeSnapshotJob.Run 输入有效日期
- **THEN** 返回 KnowledgeRunResult 包含 EventsPublished 计数
- **THEN** knowledge_runs 表新增一行 completed 状态记录

### Requirement: 同步契约 — Theme/ExportBundle/Revision

系统 SHALL 在进入导出阶段前验证 Theme / ExportBundle / Revision 的最小契约。

- `KnowledgeRevision` SHALL 包含 object_type, object_id, revision (SHA-256 前缀), source_path
- `ExportBundleSeed` SHALL 包含 bundle_kind, theme_ids, topic_ids, event_ids
- Revision 格式 SHALL 为 `{object_type}:{object_id}:{hex(sha256(content)[:8])}`

#### Scenario: Revision 契约验证
- **WHEN** BuildRevision 接收 object_type, object_id 和 content
- **THEN** 返回的 revision 字符串格式为 `{object_type}:{object_id}:{hex_prefix}`
- **THEN** 相同 content 产生相同的 revision
