## MODIFIED Requirements

### Requirement: 主题筛选扩展至 HotEvent (modified)
The daily digest SHALL include HotEvents from all platforms, not only X Topics.

#### Scenario: HotEvent 进入日报
- **WHEN** building the daily digest for export_date = D
- **THEN** the system SHALL query active HotEvents whose last_seen_at falls within the time window [D 00:00 CST, D+1 00:00 CST)
- **AND** HotEvents SHALL be sorted by heat_score DESC, limited to `DAILY_DIGEST_TOP_N` (same limit as Topics)
- **AND** tied Topics and HotEvents from the same monitor SHALL be deduplicated by content similarity

### Requirement: 日报内容扩展 (modified)
The daily digest Markdown SHALL contain both X Topic summaries and a multi-platform hot trend summary.

#### Scenario: 日报包含多平台汇总
- **WHEN** the daily digest is generated
- **THEN** the note SHALL include a `## 各平台热点汇总` section listing the top HotEvents with per-platform heat breakdown
- **AND** each HotEvent SHALL list which platforms it was observed on (X / 微博 / 知乎 / 百度)

### Requirement: 日报 frontmatter 扩展 (modified)
The DailyDigest frontmatter SHALL include multi-platform metadata.

#### Scenario: 日报 frontmatter 含 heat-event 字段
- **WHEN** the daily digest is written to Obsidian
- **THEN** the YAML frontmatter SHALL include: `type: hotkey-digest`, `digest_id`, `date`, `monitor`, `monitor_id`, `topic_count`, `event_count`, `platforms`, `tags`
- **AND** `platforms` SHALL be a list of platform names that contributed to this digest

### Requirement: HotEvent 笔记写入 (modified)
HotEvent SHALL land as standalone notes in the Obsidian vault.

#### Scenario: HotEvent 写入 Obsidian
- **WHEN** a HotEvent is exported
- **THEN** the note SHALL be written to `{root}/HotKey/events/{slug}/{date}-{id}-{title}.md`
- **AND** the frontmatter SHALL include `type: hotkey-event`, `event_id`, `heat_score`, `platforms`, `trend`, `category`
- **AND** the body SHALL contain the event summary and links to related posts from each platform

### Requirement: 幂等键扩展 (modified)
The daily export idempotency SHALL cover HotEvent exports.

#### Scenario: HotEvent export idempotency
- **WHEN** exporting a HotEvent to Obsidian
- **THEN** the SHALL use `topic_daily_exports` with `(monitor_id, topic_id, export_date)` for Topic exports
- **AND** SHALL use a similar idempotency mechanism with `(hot_event_id, export_date)` for HotEvent exports
