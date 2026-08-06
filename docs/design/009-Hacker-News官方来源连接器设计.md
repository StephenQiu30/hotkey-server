---
layer: Design
scope: backend
doc_no: "009"
title: Hacker-News官方来源连接器设计
status: proposed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/design/009-Hacker-News官方来源连接器设计.md
prd: docs/prd/009-Hacker-News官方来源连接器.md
plan: docs/plans/009-Hacker-News官方来源连接器计划.md
---

# Hacker-News官方来源连接器设计

## 现状

基于 Hacker News 官方 Firebase API 的 maxitem/item 分页采集、并发读取、检查点和故障分类已经实现。

## 目标

持续获取 HN 新内容与公开指标，支持关键词监控、趋势判断和来源证据回链。

## 非目标

本条不跨越相邻编号实现其业务细节，也不建立第二套身份、任务、数据、API 或 UI 事实源。

## 核心决策

- 端点固定为 https://hacker-news.firebaseio.com/v0，用户不得覆盖主机。
- 以单调 item id 作为检查点，按窗口限制读取范围；deleted、dead 或不可用项目保留诊断而不生成内容。
- 映射 story、comment 等类型、作者、时间、标题、URL、文本与 parent；公开指标按能力档案解释。
- 部分项目失败允许目标部分成功，重试只覆盖未完成窗口并保持幂等。

## 数据、接口与交互

- 官方 API 增量采集
- 项目类型和线程关系映射
- 公开指标快照
- 分页、并发、限流和重试

所有新增跨端字段先进入 `backend/db/schema.sql` 和后端 DTO/Swaggo 注解，再生成 `docs/openapi/swagger.json` 与前端 Client。页面同时提供正常、加载、空、错误和权限不足状态。

## 安全、失败与降级

maxitem 与内容可见性并不等价；热度算法不能把 ID 增长或缺失项目误当成传播指标。

失败必须被分类、可观测且不破坏历史事实；可选外部依赖不可用时，核心读取链路继续工作。

## 设计验收边界

- 断点续传不漏不重
- 官方端点之外的地址全部拒绝
- 部分失败和全量失败在 CollectionRun 中可区分
