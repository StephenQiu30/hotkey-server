# daily-digest Specification

## Purpose

将每个 `keyword_monitor` 下的热点按北京时间自然日沉淀为 LLM 摘要 Markdown，写入 Obsidian 同步目录，并通过 `topic_daily_exports` 保证幂等与可审计。

## Requirements

### Requirement: 时间窗口

系统 SHALL 使用北京时间（`Asia/Shanghai`）定义自然日窗口。

- `export_date = D` 的窗口为 `[D 00:00 CST, D+1 00:00 CST)`
- `DAILY_DIGEST_TARGET=yesterday` 时，`D` 为当前 CST 日期前一天
- `DAILY_DIGEST_TARGET=today` 时，`D` 为当前 CST 日期

### Requirement: 每日调度

`daily_scheduler` 每分钟检查；CST 时间 ≥ `DAILY_DIGEST_TIME`（默认 `08:00`）且当日 batch 未执行时触发 `publish_daily_topics`。
- `publish_daily_topics` SHALL 保留为历史兼容入口，内部委托给 `PublishKnowledgeSnapshotJob`

### Requirement: 主题筛选

主题 `T` 进入 `export_date = D`，当且仅当：

1. `topics.monitor_id = M` 且 `topics.status = 'active'`
2. 存在关联帖，且 `monitor_post_hits.first_seen_at` 或 `platform_posts.published_at` ∈ 窗口
3. 按 `topics.current_heat_score DESC` 排序，取 Top N（`DAILY_DIGEST_TOP_N`，默认 20）

### Requirement: 代表帖

每个入选主题 SHALL 取 Top 3 代表帖；字段含作者、摘录（最多 500 字）、`post_url`。

### Requirement: Obsidian 目录与 Frontmatter

笔记路径矩阵 SHALL 使用显式 switch-case 路径生成器 `BuildKnowledgePath`（位于 `internal/obsidian/pathing.go`）。

存放区域：

| 知识类型 | 路径 | 文件命名 |
|----------|------|---------|
| Event | `{root}/HotKey/events/{slug}/` | `{date}-{id}-{title}.md` |
| Topic | `{root}/HotKey/topics/{slug}/` | `{date}-topic-{id}-{slug}.md` |
| DailyDigest | `{root}/HotKey/digests/daily/{slug}/` | `{date}-{id}.md` |
| Theme | `{root}/HotKey/themes/` | `{id}-{slug}.md` |
| Export | `{root}/HotKey/exports/{kind}/{slug}/` | `{date}-{id}.md` |

每篇笔记 SHALL 包含 YAML frontmatter：

- **Topic**: `type: hotkey-topic`, `date`, `monitor`, `monitor_id`, `topic_id`, `topic_key`, `heat`, `trend`, `post_count`, `tags`
- **Event**: `type: hotkey-event`, `event_id`, `event_key`, `date`, `topic_ids`, `tags`
- **DailyDigest**: `type: hotkey-digest`, `digest_id`, `date`, `monitor`, `monitor_id`, `topic_count`, `event_count`, `tags`
- **Theme**: `type: hotkey-theme`, `theme_id`, `title`, `related_topics`, `event_count`, `tags`
- **Export**: `type: hotkey-export`, `export_kind`, `title`, `tags`

Topic 笔记仍使用 `BuildPath` 保持向后兼容。

### Requirement: LLM 摘要

```go
type Client interface {
    SummarizeTopic(ctx context.Context, in TopicSummaryInput) (string, error)
}
```

LLM 仅用于摘要生成，不参与聚类。单 topic 失败时标记 `failed`，不影响其他 topic。

### Requirement: topic_daily_exports 幂等

表 `topic_daily_exports` 以 `(monitor_id, topic_id, export_date)` 为幂等键；`status` 为 `pending` | `published` | `failed`。

### Requirement: 原子写盘

文件写入使用 `*.md.tmp` → `rename`；Vault 无写权限或 rename 失败时标记 `failed`。

### Requirement: 配置项

`OBSIDIAN_VAULT_PATH` 必填；`DAILY_DIGEST_*` 与 `LLM_*` 可选，默认值见 `docs/design/004-热点日报Obsidian知识库设计.md`。
