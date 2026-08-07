---
layer: PRD
scope: backend
doc_no: "009"
title: Hacker-News官方来源连接器
status: implemented
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/prd/009-Hacker-News官方来源连接器.md
design: docs/design/009-Hacker-News官方来源连接器设计.md
plan: docs/plans/009-Hacker-News官方来源连接器计划.md
---

# Hacker-News官方来源连接器

## 用户问题

运营人员需要合法、稳定地获取 Hacker News 新项目，并能从采集记录判断已完成范围、线程来源、公开指标和失败原因。系统不能因一个 item 暂时不可用而丢弃整批成功结果，也不能越过失败 ID 造成永久漏采。

## 产品目标

通过官方 Firebase API 提供可断点续传、可诊断且证据可回链的 Hacker News 增量来源，为关键词监控和后续趋势判断提供稳定输入。

## 范围

- 官方 API 的 `maxitem`/`item` 增量采集
- 五类官方 item、作者、时间、正文、URL 和父项关系映射
- `score` 与 `descendants` 的可选指标快照
- 有界分页、4 worker 并发、错误分类、部分成功和未完成窗口重试
- 复用来源管理、健康探测和采集记录界面

## 用户故事

- 作为普通用户，我能从来源目录确认数据来自 Hacker News 官方连接，并查看健康状态。
- 作为编辑者，我能在监控主题中使用已启用的 Hacker News 来源，而不配置任意抓取地址。
- 作为管理员，我能创建固定官方端点的 Hacker News 来源、执行探测，并从采集记录区分部分成功与全量失败。

## 功能要求

- FR-009-1：Hacker News 根端点固定为 `https://hacker-news.firebaseio.com/v0`；非官方地址、跨主机重定向、私网或保留地址解析必须在连接前拒绝。
- FR-009-2：以单调 item ID 作为 checkpoint；初次和后续窗口均有硬上限，按 ID 升序输出，已完成连续前缀不重复请求。
- FR-009-3：映射 `story`、`job`、`poll`、`comment`、`pollopt`，保留作者、时间、标题、URL、正文，以及评论或投票选项的 `parent_external_id`。
- FR-009-4：`score` 与 `descendants` 仅在官方字段存在时写入指标；字段缺失保持未知，显式零值保持已知零值。
- FR-009-5：`deleted`、`dead`、`null`、无效项目和未知类型形成安全诊断而不生成内容；原始响应和传输错误文本不持久化。
- FR-009-6：单项请求失败时只提交首个失败前的连续完成前缀并保留分类诊断；下一窗口从首个失败继续。全部 item 请求失败时运行失败且 checkpoint 不前移。
- FR-009-7：上游 `429` 保留 `Retry-After`；认证、限流、临时、解析和永久错误保持稳定、确定性的分类。

## 非功能要求

- 单页最多 4 个并发 item 请求，并响应调用方取消和来源超时。
- CollectionRun、CapturedItem、checkpoint 与下游任务仍在既有事务和幂等边界内持久化。
- 来源页面覆盖正常、加载、空、错误和权限不足状态，桌面与移动端均可键盘操作并具有可见焦点。

## 非目标

不抓取 Hacker News 网页，不实现 Firebase 实时订阅，不把 item ID 增长解释为热度，也不为本条新增队列、数据库列或公开 API。

## 验收标准

- AC-009-1：断点续传只重试未完成窗口，连续成功前缀不漏不重。
- AC-009-2：非官方端点、跨主机重定向和非公网解析全部拒绝。
- AC-009-3：部分 item 失败形成带拒绝诊断的成功 CollectionRun；全部 item 请求失败形成不推进 checkpoint 的失败 CollectionRun。
