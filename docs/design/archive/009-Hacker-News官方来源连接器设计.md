---
layer: Design
scope: backend
doc_no: "009"
title: Hacker-News官方来源连接器设计
status: accepted
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/design/archive/009-Hacker-News官方来源连接器设计.md
prd: docs/prd/archive/009-Hacker-News官方来源连接器.md
plan: docs/plans/archive/009-Hacker-News官方来源连接器计划.md
---

# Hacker-News官方来源连接器设计

## 现状

来源控制面、共享 CollectionRun、checkpoint、CapturedItem v2 和 Ingestion 幂等入口已经存在。Hacker News 连接器已具备官方端点、`maxitem` 增量窗口和并发 item 读取，但原实现未保存评论父项，无法区分可选指标缺失与显式零值，且任一 item 请求失败都会放弃整页。

## 目标

仅通过 Hacker News 官方 Firebase API 持续获取公开项目与指标；让每个完成窗口具备单调检查点、线程父项、可解释指标和可重试的部分失败边界。

## 非目标

- 不抓取 Hacker News HTML 页面，不绕过官方 API，也不允许用户替换 API 主机。
- 不实现任务 018 的原始证据归档、任务 019 的指标权重或任务 020 的事件热度算法。
- 不增加第二套调度、检查点、来源事实或 HTTP API。

## 官方契约依据

- 唯一协议依据为 [Hacker News 官方 API](https://github.com/HackerNews/API)。
- 根端点固定为 `https://hacker-news.firebaseio.com/v0`；项目读取使用 `/item/<id>.json`，高水位使用 `/maxitem.json`。
- 官方字段均按可选字段处理；客户端忽略未知新增字段，不假设 `score`、`descendants` 或 `parent` 必然存在。

## 核心决策

- SourceConnection 只能使用固定官方根端点。DNS 每次连接重新解析并拒绝私网、回环、链路本地、保留地址和混合解析；重定向只能停留在同一 HTTPS 官方主机。
- 初次读取只覆盖最新的有界 ID 窗口；后续从单调 item ID checkpoint 的下一项继续，按升序稳定输出，单次最多读取 Collection 请求上限。
- item 读取最多使用 4 个 worker。`story`、`job`、`poll` 映射为 `article`；`comment`、`pollopt` 映射为 `comment`。
- `comment.parent` 与 `pollopt.poll` 统一保存为 CapturedItem 的 `parent_external_id`；根项目不虚构父项。
- `score` 映射为已知 `like_count`，`descendants` 映射为已知 `comment_count`。字段缺失保持 `nil`，显式 `0` 保持已知零值，最终权重由已发布的来源能力档案解释。
- `deleted`、`dead`、`null`、无效项目和未知类型返回安全诊断而不生成内容；它们是已完成读取，可以推进 checkpoint。
- item 请求失败时只持久化首个失败 ID 之前的连续完成前缀，checkpoint 停在该前缀末尾；首个失败及其后项目由下一窗口重试。至少一个 item 有有效响应时 CollectionRun 记为部分成功并保留诊断；全部 item 请求失败时 CollectionRun 记为失败且不移动 checkpoint。

## 数据、接口与交互

- `SourceItem` 与 `CapturedItem` 增加可选 `ParentExternalID`，随 CapturedItem v2 JSON 以 `parent_external_id` 持久化；旧 v1/v2 JSON 缺少该字段时仍可读取。
- CollectionRun 使用既有 `accepted_count`、`rejected_count`、`error_code`、`next_cursor` 和 target 状态表达部分成功与全量失败，不增加表或列。
- 来源创建、编辑、探测和采集记录继续复用任务 007 已交付的管理 API 与页面。本条不改变 OpenAPI 和生成前端 Client。

## 安全、失败与降级

- `maxitem` 只表示当前最大 item ID，不是浏览量、传播量或热度信号。
- `429` 保留 `Retry-After`，认证、限流、临时、解析和永久错误保持稳定分类；并发取消竞态按确定性优先级保留根因。
- 部分成功绝不越过首个未完成 ID；后续已响应项目不提前持久化，避免 checkpoint 出现空洞。
- 原始上游响应和传输错误文本不进入持久化诊断，历史 CapturedItem 与 Content 不因重试被删除。

## 设计验收边界

- AC-009-1：断点续传只重试未完成窗口，连续成功前缀不漏不重。
- AC-009-2：非官方端点、跨主机重定向和非公网解析全部拒绝。
- AC-009-3：部分 item 失败形成带拒绝诊断的成功 CollectionRun；全部 item 请求失败形成不推进 checkpoint 的失败 CollectionRun。
