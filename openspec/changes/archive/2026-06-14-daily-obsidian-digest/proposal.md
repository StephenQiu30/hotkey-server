---
layer: Proposal
issue: STE-302
title: "热点日报 Obsidian 知识库 MVP"
status: accepted
---

## Summary

在 hotkey-server 中引入热点日报沉淀能力：按北京时间自然日筛选每个 `keyword_monitor` 的活跃热点主题，调用 LLM 生成中文摘要，渲染为带 YAML frontmatter 的 Markdown 笔记，原子写入 Obsidian 同步目录，并用 `topic_daily_exports` 表保证幂等、可重试、可审计。

## In Scope

- `topic_daily_exports` 数据表与 migration
- 配置扩展（`OBSIDIAN_VAULT_PATH`、`DAILY_DIGEST_*`、`LLM_*`）
- `internal/digest` — CST 自然日窗口、主题入选规则、代表帖聚合
- `internal/llm` — `SummarizeTopic` 接口、OpenAI 兼容实现、prompt 模板
- `internal/obsidian` — frontmatter 渲染、slug 生成、原子写文件
- `internal/jobs/publish_daily_topics.go` — 编排 job
- `internal/jobs/daily_scheduler.go` — 每日定时 gate
- `internal/database/digestrepo.go` — exports CRUD

## Out of Scope

- hotkey-web 配置页、手动触发、预览（后续迭代）
- Obsidian 官方插件
- 多用户各自 Vault 路径（MVP 全局 `OBSIDIAN_VAULT_PATH`）
- 用户级汇总日报、PDF/邮件推送
- 修改 Jaccard 聚类算法
- OpenAPI 变更
- 生产部署与运维文档

## 与 001 设计的 LLM 范围扩展说明

[`001-x热点监控平台设计.md`](../../docs/design/001-x热点监控平台设计.md) 规定：

> 主题聚合和趋势判断不能设计成强依赖 LLM，否则成本和可控性都过高。

本功能作为**独立的日报沉淀层**引入 LLM，严格遵守该约束：

1. **LLM 仅用于 digest 摘要生成**：`internal/llm.Client.SummarizeTopic` 接收已聚类完成的主题数据，生成 2–4 段中文摘要。
2. **不参与聚类**：`internal/topic.Cluster()` 行为不变，主题聚类仍由 Jaccard 相似度完成。
3. **失败隔离**：LLM 超时或限流时，单个 topic 标记 `status=failed`，不影响其他 topic 的导出。
4. **可选降级**：LLM 不可用时可 fallback 为规则摘要（截取代表帖前 200 字），不阻塞日报流程。

## 验证方式

- 文档评审：对照 `docs/design/004` 与 `docs/plans/011` 逐项核对
- 无需代码测试（本 ticket 为文档门禁）
