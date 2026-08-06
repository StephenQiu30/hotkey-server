---
layer: PRD
scope: backend
doc_no: "009"
title: Hacker-News官方来源连接器
status: draft
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/prd/009-Hacker-News官方来源连接器.md
design: docs/design/009-Hacker-News官方来源连接器设计.md
plan: docs/plans/009-Hacker-News官方来源连接器计划.md
---

# Hacker-News官方来源连接器

## 用户问题

基于 Hacker News 官方 Firebase API 的 maxitem/item 分页采集、并发读取、检查点和故障分类已经实现。

## 产品目标

持续获取 HN 新内容与公开指标，支持关键词监控、趋势判断和来源证据回链。

## 范围

- 官方 API 增量采集
- 项目类型和线程关系映射
- 公开指标快照
- 分页、并发、限流和重试

## 用户故事

- 作为普通用户，我能够看懂当前状态、结果来源和下一步动作。
- 作为编辑者，我能够在权限范围内完成配置或处理任务，并获得明确反馈。
- 作为管理员，我能够治理配置、失败、成本与审计，而不直接修改数据库事实。

## 功能要求

- FR-009-1：端点固定为 https://hacker-news.firebaseio.com/v0，用户不得覆盖主机。
- FR-009-2：以单调 item id 作为检查点，按窗口限制读取范围；deleted、dead 或不可用项目保留诊断而不生成内容。
- FR-009-3：映射 story、comment 等类型、作者、时间、标题、URL、文本与 parent；公开指标按能力档案解释。
- FR-009-4：部分项目失败允许目标部分成功，重试只覆盖未完成窗口并保持幂等。

## 非功能要求

- 所有写操作具备认证、授权、输入校验、幂等或乐观锁保护和审计。
- 列表支持稳定分页；第三方调用具备超时、配额、重试和降级。
- Web 覆盖桌面与移动端，并满足键盘操作、可见焦点和减弱动效。

## 非目标

不使用未授权抓取补齐来源能力，不为本条引入与仓库规范冲突的新基础设施。

## 验收标准

- AC-009-1：断点续传不漏不重
- AC-009-2：官方端点之外的地址全部拒绝
- AC-009-3：部分失败和全量失败在 CollectionRun 中可区分
