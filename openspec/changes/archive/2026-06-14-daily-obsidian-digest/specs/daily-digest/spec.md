---
layer: Specs
issue: STE-302
title: "热点日报 Obsidian 知识库 MVP — daily-digest 规格"
status: accepted
---

## 主题入选规则

### S1: 时间窗口

- 系统 SHALL 使用北京时间（`Asia/Shanghai`）定义自然日窗口
- `export_date = D` 的窗口为 `[D 00:00 CST, D+1 00:00 CST)`
- `DAILY_DIGEST_TARGET=yesterday` 时，`D` 为当前 CST 日期前一天
- `DAILY_DIGEST_TARGET=today` 时，`D` 为当前 CST 日期

### S2: 主题筛选

主题 `T` 进入 `export_date = D`，当且仅当：

1. `topics.monitor_id = M` 且 `topics.status = 'active'`
2. 存在关联帖，且 `monitor_post_hits.first_seen_at` 或 `platform_posts.published_at` ∈ 窗口 `[D 00:00 CST, D+1 00:00 CST)`
3. 按 `topics.current_heat_score DESC` 排序，取 Top N（`DAILY_DIGEST_TOP_N`，默认 20）

### S3: 代表帖

- 每个入选主题 SHALL 取 Top 3 代表帖（按热度或时间排序）
- 代表帖字段：作者、摘录（最多 500 字）、`post_url`

## Obsidian 笔记契约

### S4: 目录结构

```
{OBSIDIAN_VAULT_PATH}/HotKey/topics/{monitor-slug}/{date}-topic-{id}-{title-slug}.md
```

- `monitor-slug`：monitor 名称经 `Slugify` 处理（移除特殊字符，空格转连字符，小写）
- `title-slug`：主题标题经 `Slugify` 处理

### S5: Frontmatter

每篇笔记 SHALL 包含以下 YAML frontmatter 字段：

```yaml
---
type: hotkey-topic
date: 2026-06-14          # export_date
monitor: AI监管            # monitor 名称
monitor_id: 1              # keyword_monitors.id
topic_id: 42               # topics.id
topic_key: "ai:监管:政策"  # topics.topic_key
heat: 85.4                 # topics.current_heat_score
trend: rising              # topics.current_trend
post_count: 12             # 关联帖数量
tags:
  - hotkey
  - topic
  - monitor/{monitor-slug}
---
```

### S6: 正文结构

1. LLM 摘要（2–4 段中文，客观陈述，不编造事实）
2. 关键帖摘录（Top 3：作者、摘录、`post_url`）
3. 数据脚注（热度、趋势、帖子数、生成时间戳）

## LLM 摘要

### S7: 接口契约

```go
type Client interface {
    SummarizeTopic(ctx context.Context, in TopicSummaryInput) (string, error)
}
```

- 输入：monitor 名称/查询词、topic 标题、代表帖文本（每帖最多 500 字截断）、热度/趋势/帖子数
- 输出：客观中文摘要，2–4 段
- LLM 仅用于此摘要生成，不参与聚类

### S8: LLM 失败路径

- 单个 topic LLM 超时/限流/返回错误：该 topic `topic_daily_exports.status = 'failed'`，`error_message` 记录原因
- 其他 topic 不受影响，继续导出
- 可选 fallback：规则摘要（截取代表帖前 200 字），不阻塞流程

## 数据模型

### S9: topic_daily_exports 表

```sql
create table topic_daily_exports (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  topic_id bigint not null references topics(id),
  export_date date not null,
  summary_text text not null default '',
  markdown_path text not null default '',
  status text not null default 'pending',
  error_message text not null default '',
  published_at timestamptz,
  created_at timestamptz not null default now(),
  unique(monitor_id, topic_id, export_date)
);
```

- `status` 枚举：`pending` | `published` | `failed`
- 幂等键：`(monitor_id, topic_id, export_date)`
- UPSERT 冲突时更新 `summary_text`、`markdown_path`、`status`、`error_message`、`published_at`

## 幂等与重试

### S10: 幂等保证

- 同一 `(monitor_id, topic_id, export_date)` 重复执行 SHALL 更新而非新建记录
- 文件写入使用 `*.md.tmp` → `rename` 原子操作
- 已 `published` 的记录重复执行时 SHALL 覆盖文件内容

## 调度

### S11: 每日触发

- `daily_scheduler` 每分钟检查一次
- 当 CST 时间 ≥ `DAILY_DIGEST_TIME`（默认 `08:00`）且当日 batch 未执行时触发
- 使用 `topic_daily_exports` 记录或 advisory lock 防并发重复

## Vault 写入失败

### S12: 写权限失败

- Vault 目录无写权限时：`topic_daily_exports.status = 'failed'`，`error_message` 记录权限错误
- 打日志告警，不阻塞其他 monitor 的导出

### S13: 同步盘冲突

- 使用 `*.md.tmp` → `rename` 原子写入，避免同步盘读取半写文件
- rename 失败时标记 `status = 'failed'`

## 配置项

### S14: 必填配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `OBSIDIAN_VAULT_PATH` | — | Vault 同步目录根路径（必填） |

### S15: 可选配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `DAILY_DIGEST_TIME` | `08:00` | CST 触发时刻 |
| `DAILY_DIGEST_TIMEZONE` | `Asia/Shanghai` | 时区 |
| `DAILY_DIGEST_TARGET` | `yesterday` | `yesterday` \| `today` |
| `DAILY_DIGEST_TOP_N` | `20` | 每 monitor 最多导出主题数 |
| `LLM_PROVIDER` | `openai` | LLM 提供方 |
| `LLM_API_KEY` | — | API Key（启用 LLM 时必填） |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | 兼容网关 |
| `LLM_MODEL` | `gpt-4o-mini` | 模型 |
