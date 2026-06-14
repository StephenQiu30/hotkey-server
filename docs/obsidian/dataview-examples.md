# Obsidian Dataview 示例

配合 HotKey 日报导出的 frontmatter 字段，在 Obsidian 中创建以下查询笔记。

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

## Frontmatter 字段说明

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

## 建议目录结构

在 Vault 中保留 HotKey 导出目录，另建 `HotKey/views/` 存放上述 Dataview 查询笔记，避免与自动生成的主题笔记混放。
