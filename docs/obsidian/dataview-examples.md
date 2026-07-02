# Obsidian Dataview 示例

配合 HotKey 知识中台导出的 frontmatter 字段，在 Obsidian 中创建以下查询笔记。

## 昨日热点榜

```dataview
TABLE heat, trend, monitor, post_count
FROM "HotKey/topics"
WHERE date = date(today) - dur(1 day)
SORT heat DESC
```

## 按监控筛选

```dataview
TABLE date, heat, trend, post_count
FROM "HotKey/topics"
WHERE monitor = "AI监管"
SORT date DESC, heat DESC
LIMIT 30
```

## 本周热点趋势

```dataview
TABLE rows.heat AS "热度", rows.trend AS "趋势"
FROM "HotKey/topics"
WHERE date >= date(today) - dur(7 days)
GROUP BY monitor
```

## 上升话题

```dataview
LIST
FROM "HotKey/topics"
WHERE trend = "rising" AND date >= date(today) - dur(3 days)
SORT heat DESC
```

## 近期事件

```dataview
TABLE date, event_key
FROM "HotKey/events"
WHERE date >= date(today) - dur(7 days)
SORT date DESC
```

## 本周日报

```dataview
LIST
FROM "HotKey/digests/daily"
WHERE date >= date(today) - dur(7 days)
SORT date DESC
```

## 专题概览

```dataview
TABLE title, event_count
FROM "HotKey/themes"
SORT event_count DESC
```

## 最近导出

```dataview
TABLE export_kind, title, period
FROM "HotKey/exports"
SORT file.ctime DESC
LIMIT 10
```

## Frontmatter 字段说明

### Topic 笔记

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 固定 `hotkey-topic` |
| `date` | date | CST 自然日 |
| `monitor` | string | 监控名称 |
| `monitor_id` | number | 监控 ID |
| `topic_id` | number | 主题 ID |
| `topic_key` | string | 主题键 |
| `heat` | number | 当前热度 |
| `trend` | string | `rising` / `falling` / `stable` |
| `post_count` | number | 关联帖数 |
| `tags` | list | 含 `hotkey`、`topic`、`monitor/{slug}` |

### Event 笔记

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 固定 `hotkey-event` |
| `event_id` | number | 事件 ID |
| `event_key` | string | 事件键 |
| `date` | date | 事件日期 |
| `topic_ids` | list | 关联主题 ID 列表 |
| `tags` | list | 含 `hotkey`、`event` |

### Digest 笔记

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 固定 `hotkey-digest` |
| `digest_id` | number | 摘要 ID |
| `date` | date | CST 自然日 |
| `monitor` | string | 监控名称 |
| `monitor_id` | number | 监控 ID |
| `topic_count` | number | 主题数 |
| `event_count` | number | 事件数 |
| `tags` | list | 含 `hotkey`、`digest`、`daily` |

### Theme 笔记

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 固定 `hotkey-theme` |
| `theme_id` | number | 专题 ID |
| `title` | string | 专题标题 |
| `related_topics` | list | 相关主题列表 |
| `event_count` | number | 关联事件数 |
| `tags` | list | 含 `hotkey`、`theme` |

### Export 笔记（报告/素材）

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 固定 `hotkey-export` |
| `export_kind` | string | `daily` / `weekly` / `monthly` / `thematic` / `material` |
| `title` | string | 报告标题 |
| `period` / `theme_title` | string | 周期标签或专题标题 |
| `tags` | list | 含 `hotkey`、`export` |

## 建议目录结构

```text
HotKey/
  events/{monitor-slug}/{date}-{id}-{title}.md
  topics/{monitor-slug}/{date}-topic-{id}-{slug}.md
  digests/daily/{monitor-slug}/{date}-{id}.md
  themes/{id}-{slug}.md
  exports/
    daily/{monitor-slug}/{date}-{id}.md
    weekly/{monitor-slug}/{date}-{id}.md
    monthly/{monitor-slug}/{date}-{id}.md
    thematic/{date}-{id}-{slug}.md
    material/{date}-{id}.md
```

另建 `HotKey/views/` 存放上述 Dataview 查询笔记，避免与自动生成的知识笔记混放。
