---
layer: Plan
doc_no: 022
audience: Dev, PM, QA
feature_area: Obsidian 热点知识中台
purpose: 定义 Obsidian 热点知识中台从需求冻结到模型、沉淀、导出与回写的总体实施顺序、依赖关系和阶段门禁
canonical_path: docs/plans/022-Obsidian热点知识中台总体实施计划.md
status: draft
version: v1.0
owner: Codex
inputs:
  - docs/prd/001-obsidian热点知识中台需求.md
  - docs/design/004-热点日报Obsidian知识库设计.md
  - docs/plans/011-热点日报Obsidian知识库计划.md
  - docs/design/007-核心数据模型与数据库设计.md
outputs:
  - 知识中台实施阶段顺序
  - 子计划依赖关系
  - 统一验证门禁
triggers:
  - 需要将热点日报能力升级为知识中台
  - 需要确认 Event/Topic/导出/回写的落地顺序
downstream:
  - docs/plans/023-事件主题知识模型与同步基线计划.md
  - docs/plans/024-Obsidian知识沉淀与导出计划.md
  - docs/plans/025-双向回写与治理计划.md
---

# Obsidian Hotspot Knowledge Platform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有热点日报导出能力升级为基于 Event/Topic 双层对象、支持沉淀、查看、导出和结构化回写的 Obsidian 热点知识中台。

**Architecture:** 保持 `hotkey-server` 作为机器事实源，在其上新增事件对象、主题/导出编排对象、人工知识 sidecar 与回写审计边界；Obsidian Vault 承担知识沉淀与消费层。实施顺序必须遵循 `模型与同步基线 -> Vault 沉淀与导出 -> 双向回写与治理`，避免先写文件后补边界造成返工。

**Tech Stack:** Go, PostgreSQL, GORM, worker jobs, Markdown, YAML frontmatter, Obsidian Dataview, OpenAPI

---

# 背景

当前仓库已经有一条 `publish_daily_topics` 到 Obsidian 的日报导出链路，但它默认 `Topic` 就是最终知识对象，且以“写文件成功”为主要完成标志。这不足以支撑长期研究、统一导出和结构化回写。

本计划用于把这次能力升级拆成可执行阶段，避免在单次实现中同时修改数据模型、同步目录、导出模板和回写规则而导致逻辑漏洞。

# 范围

1. 冻结新知识中台文档链路和历史输入边界。
2. 拆分 Event/Topic 双层对象和同步基线。
3. 建立 Vault 目录、知识沉淀和导出能力。
4. 建立回写白名单、冲突检测和审计治理。

# 非目标

1. 本计划不替代具体 schema、repository 或 job 的代码细节。
2. 本计划不定义前端页面交互设计。
3. 本计划不引入多租户或分布式协同编辑。

# 阶段顺序

## Phase 0: 文档冻结与历史边界确认

目标：

1. 以 `docs/prd/001` 作为知识中台需求事实源。
2. 以 `022/023/024/025` 作为执行主线。
3. 明确 `004/011` 降级为“日报导出历史输入”，不再单独代表知识中台主线。

验证门禁：

```bash
test -f docs/prd/001-obsidian热点知识中台需求.md
test -f docs/plans/022-Obsidian热点知识中台总体实施计划.md
rg -n "004-热点日报Obsidian知识库设计|011-热点日报Obsidian知识库计划" docs/prd docs/plans
```

## Phase 1: 事件主题知识模型与同步基线

对应计划：[`023-事件主题知识模型与同步基线计划.md`](023-事件主题知识模型与同步基线计划.md)

目标：

1. 在数据库和领域层新增 `Event` 主对象。
2. 明确 `Event / Topic / DailyDigest / Theme / ExportBundle` 的边界。
3. 明确人工知识 sidecar 与 revision contract，禁止人工字段直接落入机器事实表。
4. 将现有 `publish_daily_topics` 升级为通用同步基线，而不是孤立日报 job。

验证门禁：

```bash
go test ./internal/event/... ./internal/topic/... ./internal/jobs/... ./internal/database/... -v
test ! -d db/migrations
```

## Phase 2: Obsidian 知识沉淀与统一导出

对应计划：[`024-Obsidian知识沉淀与导出计划.md`](024-Obsidian知识沉淀与导出计划.md)

目标：

1. 建立稳定 Vault 目录与 frontmatter 契约。
2. 支持日报、周报、月报、专题报告和素材清单统一导出。
3. 保证知识对象与导出结果的可重建与幂等更新。

验证门禁：

```bash
go test ./internal/obsidian/... ./internal/export/... ./internal/jobs/... -v
```

## Phase 3: 双向回写与治理

对应计划：[`025-双向回写与治理计划.md`](025-双向回写与治理计划.md)

目标：

1. 建立 Obsidian 到 HotKey 的白名单回写能力。
2. 建立冲突检测、审计记录和失败治理。
3. 建立回写后的再导出一致性验证。

验证门禁：

```bash
go test ./internal/obsidian/... ./internal/knowledge/... ./internal/jobs/... ./tests/integration/... -v
```

## Phase 4: 综合验收

目标：

1. 同一条热点事实可以完成从采集到沉淀、从沉淀到回写、从回写到再导出的完整闭环。
2. 文档、数据模型、Vault 结构和导出结果叙事一致。

验证门禁：

```bash
go test ./...
go build ./...
bash scripts/validate-repository.sh
```

# 执行纪律

1. 不允许跳过 `023` 直接扩写 Vault 目录。
2. 不允许先做任意自由回写，再补白名单和冲突规则。
3. 不允许在本轮重新引入 `db/migrations/`；`db/schema.sql` 是当前仓库唯一完整 schema 基线。
4. 每个阶段结束前都必须留下测试命令或回归证据，而不是只凭人工浏览判断完成。

# 关联文档

1. `docs/prd/001-obsidian热点知识中台需求.md`
2. `docs/design/004-热点日报Obsidian知识库设计.md`
3. `docs/plans/011-热点日报Obsidian知识库计划.md`
4. `docs/plans/023-事件主题知识模型与同步基线计划.md`
5. `docs/plans/024-Obsidian知识沉淀与导出计划.md`
6. `docs/plans/025-双向回写与治理计划.md`

# 验收门禁

1. `022` 只能作为执行入口，不替代子计划。
2. 三份子计划都必须对应唯一阶段，不得相互混写。
3. 所有新增知识对象都必须能映射到明确的测试或集成验证任务。

# 风险与边界

1. 如果 Phase 1 不先冻结对象边界，Phase 2 很容易把目录结构当成事实模型。
2. 如果 Phase 3 先做回写实现再补治理，很容易让人工字段污染机器事实。
3. 如果仍沿用 `topic_daily_exports` 作为唯一知识中台主对象，后续复杂导出会继续堆在错误的抽象上。
