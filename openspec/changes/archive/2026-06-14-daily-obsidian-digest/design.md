---
layer: Design
issue: STE-302
title: "热点日报 Obsidian 知识库 MVP — 设计"
status: accepted
---

## 系统架构

```mermaid
flowchart TB
  subgraph worker [Worker]
    sched[daily_scheduler]
    job[publish_daily_topics]
    sched --> job
  end

  subgraph modules [New Modules]
    digest[digest]
    llm[llm]
    obsidian[obsidian]
  end

  subgraph db [PostgreSQL]
    topics[topics]
    exports[topic_daily_exports]
  end

  subgraph vault [Obsidian Sync]
    md[HotKey/topics/*.md]
  end

  job --> digest
  digest --> topics
  job --> llm
  job --> obsidian
  job --> exports
  obsidian --> md
```text

## 模块职责

| 包 | 职责 | 关键类型/函数 |
|----|------|---------------|
| `internal/digest` | CST 自然日窗口、主题入选规则、代表帖聚合 | `DayWindow()`, `ResolveExportDate()`, `ListTopicsForDay()`, `FetchRepresentativePosts()` |
| `internal/llm` | `SummarizeTopic` 接口、OpenAI 兼容实现、prompt 模板 | `Client` interface, `OpenAIClient`, `MockClient`, `PromptBuilder` |
| `internal/obsidian` | frontmatter 渲染、slug 生成、原子写文件 | `Slugify()`, `RenderTopicNote()`, `BuildPath()`, `WriteAtomic()` |
| `internal/jobs/publish_daily_topics.go` | 编排：monitors → digest → LLM → export → write | `PublishDailyTopicsJob.Run()` |
| `internal/jobs/daily_scheduler.go` | 每分钟 gate，判断是否到达触发时刻 | `DailyScheduler.ShouldRun()` |
| `internal/database/digestrepo.go` | `topic_daily_exports` CRUD | `DigestRepo.Upsert()`, `DigestRepo.GetByTopicDate()` |

## 数据流

```text
daily_scheduler (每分钟)
  └─ ShouldRun(now, lastRunDate)?
       └─ true → PublishDailyTopicsJob.Run(ctx)
            ├─ 遍历 active monitor IDs
            ├─ exportDate = ResolveExportDate(now, cfg.DailyDigestTarget)
            ├─ topics = digest.ListTopicsForDay(monitorID, exportDate)
            └─ 对每个 topic:
                 ├─ posts = digest.FetchRepresentativePosts(topicID, 3)
                 ├─ summary = llm.SummarizeTopic(monitor, topic, posts)
                 ├─ digestRepo.Upset(monitorID, topicID, exportDate, summary)
                 ├─ content = obsidian.RenderTopicNote(topic, summary, posts)
                 ├─ path = obsidian.BuildPath(vaultRoot, monitorSlug, filename)
                 ├─ obsidian.WriteAtomic(path, content)
                 └─ digestRepo.UpdateStatus(published)
```text

## 数据模型

### topic_daily_exports

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
```text

- 状态机：`pending` → `published` | `failed`
- 幂等键：`(monitor_id, topic_id, export_date)`
- 索引建议：`(export_date, status)`, `(monitor_id, export_date)`

## Obsidian 文件结构

```text
{OBSIDIAN_VAULT_PATH}/
  HotKey/
    topics/
      {monitor-slug}/
        {date}-topic-{id}-{title-slug}.md
```text

## 配置项

| 环境变量 | 默认值 | 必填 | 说明 |
|----------|--------|------|------|
| `OBSIDIAN_VAULT_PATH` | — | 是 | Vault 同步目录根路径 |
| `DAILY_DIGEST_TIME` | `08:00` | 否 | CST 触发时刻 |
| `DAILY_DIGEST_TIMEZONE` | `Asia/Shanghai` | 否 | 时区 |
| `DAILY_DIGEST_TARGET` | `yesterday` | 否 | `yesterday` \| `today` |
| `DAILY_DIGEST_TOP_N` | `20` | 否 | 每 monitor 最多导出主题数 |
| `LLM_PROVIDER` | `openai` | 否 | LLM 提供方 |
| `LLM_API_KEY` | — | 启用 LLM 时必填 | API Key |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | 否 | 兼容网关 |
| `LLM_MODEL` | `gpt-4o-mini` | 否 | 模型 |

## 关键决策

1. **LLM 仅用于摘要，不参与聚类** — 遵守 001 设计约束，`internal/topic.Cluster()` 不变
2. **DB 记录 + 文件发布（方案 B）** — `topic_daily_exports` 存摘要与发布状态，Job 先落库再写 Vault，兼顾幂等、重试与未来 Web/API 复用
3. **原子写盘** — `*.md.tmp` → `rename`，避免同步盘读取半写文件
4. **scheduler gate** — 复用现有 `jobs.Runner` interval 机制，内部判断是否到达每日触发时刻

## 失败路径

| 场景 | 行为 |
|------|------|
| Vault 无写权限 | exports `failed`，打日志 |
| LLM 超时/限流 | 单 topic 失败，不影响其他 topic |
| 同步盘冲突 | `*.md.tmp` → `rename` 原子写 |
| 无热点 | 跳过，不写空文件 |

## 回滚策略

- 删除 `topic_daily_exports` 表即可回滚
- Obsidian Vault 中的 `.md` 文件可手动清理
- 不影响现有 `topics` 表和聚类逻辑
