---
layer: Design
scope: shared
doc_no: "031"
title: Hacker-News热门榜单与持续观测设计
status: accepted
version: v1.1
owner: HotKey Team
phase: P0
canonical_path: docs/design/archive/031-Hacker-News热门榜单与持续观测设计.md
prd: docs/prd/archive/031-Hacker-News热门榜单与持续观测.md
plan: docs/plans/archive/031-Hacker-News热门榜单与持续观测计划.md
---

# Hacker-News热门榜单与持续观测设计

## 现状与问题

009 已通过 Hacker News 官方 Firebase API 按 `maxitem` 增量采集新项目，但已完成的 item 不会再次读取。系统因此只能发现新内容，不能持续观察热门项目的 `score` 与 `descendants` 变化，数据库也没有可供事件热度和趋势计算使用的连续指标快照。

## 目标

- 通过官方 `topstories`、`beststories` 列表获取有序热门候选，并在每轮定时采集中重复观测榜单内项目。
- 复用既有 Content 幂等更新和指标快照，不增加第二套调度、采集记录或热点事实源。
- 在来源 UI 中显式选择“热门榜单 / 最佳榜单 / 最新项目”，新建 Hacker News 来源默认使用热门榜单。
- 用真实官方数据跑通来源、采集、内容、监控匹配、事件与通知链路。

## 非目标

- 不抓取 Hacker News HTML，不绕过官方 API，不允许自定义 API 主机。
- P0 不持久化榜单名次，不把榜单顺序伪装成浏览量，也不复制参考项目的网页抓取或进程内定时器。
- 不改变 020 的事件热度或聚类公式；本条只补足连续官方指标输入。

## 核心决策

- `SourceConfig.hacker_news_mode` 只允许 `new`、`top`、`best`。缺失时默认 `new`，保证历史来源行为不变；管理 UI 新建 HN 来源时默认提交 `top`。
- `new` 继续使用 009 的 `maxitem` 单调 checkpoint。`top` 与 `best` 分别读取 `/topstories.json` 与 `/beststories.json`，按官方列表顺序去重并截取 Collection 请求上限。
- 榜单模式不使用消费型 cursor，每轮都返回当前榜单项目，`has_more=false`。Ingestion 以 `(source_connection_id, external_id)` 幂等更新 Content 的公开指标，并为每次观测追加 metric snapshot。
- item 读取仍限制为最多 4 个 worker。部分 item 失败时保留成功项目与安全诊断；全部请求失败时本轮失败。`429`、超时、解析和永久错误沿用既有稳定分类。
- 健康探测按配置模式检查对应官方列表端点，避免 `maxitem` 正常但实际榜单契约不可用时误报健康。
- 标准化任务必须沿 captured-item cursor 排空本轮全部页；cursor 不前进时永久失败，避免单轮上限大于默认 50 时静默遗漏后续内容。
- HN 官方 item 不提供语言或地区。`und` 和空地区作为“未知”保持中性，只有已知且不同的元数据才触发语言/地区硬拒绝，显式规则仍保持精确匹配。

## 数据、API 与 UI

- `source_connections.config` 白名单增加 `hacker_news_mode`，数据库触发器与领域层都校验枚举。
- 来源管理 DTO 与 OpenAPI 暴露该非敏感配置；生成前端 Client 保持单一契约来源。
- 来源创建对话框仅在 HN 类型下展示榜单模式，说明热门/最佳模式会重复观测官方 score 与评论数。

## 验收边界

- AC-031-1：热门与最佳模式访问正确官方列表、保持榜单顺序、受请求上限和 4 worker 并发约束。
- AC-031-2：同一榜单项目连续两轮均被返回，第二轮公开指标可变化；最新项目模式仍不重复读取已完成 ID。
- AC-031-3：真实 HN 热门来源产生 Content 和指标快照，并进入现有监控匹配、事件、趋势与通知流水线；浏览器可完成来源配置并查看结果。
