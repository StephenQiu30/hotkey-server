---
layer: Design
scope: shared
doc_no: "024"
title: 日报周报私有Feed与知识归档设计
status: proposed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/design/024-日报周报私有Feed与知识归档设计.md
prd: docs/prd/024-日报周报私有Feed与知识归档.md
plan: docs/plans/024-日报周报私有Feed与知识归档计划.md
---

# 日报周报私有Feed与知识归档设计

## 现状

日报/周报构建、预览、冻结、发布、SMTP、私有 RSS/Atom、Obsidian Vault 和知识提案审批已在后端实现，前端只有订阅页，缺少完整报告工作台。

## 目标

把高价值事件组织成可预览、可发布、可订阅、可归档的日报周报，并保留每项证据和发布版本。

## 非目标

不在本条复制相邻领域的数据或服务，不以纯前端假数据宣称功能完成，也不引入第二套事实源。

## 核心决策

- Report 构建产生草稿快照，预览不修改事实，发布冻结内容与排序；同一范围和周期保持幂等。
- 每个 ReportItem 引用 EventUpdate、摘要和证据，不复制不可追溯的模型文本。
- 订阅支持 email 与私有 RSS/Atom；Feed Token 可轮换、仅保存摘要，不进入日志。
- 知识归档通过 proposal→approve/reject→apply→reconcile 流程更新 Vault，不让任务直接覆盖人工文件。

## 数据、接口与交互

- 报告列表、详情、构建、预览和发布
- 日报/周报订阅
- 邮件与私有 Feed
- 知识提案、审批、同步和对账

所有状态变化保存明确版本、时间和操作者；跨端字段由 Schema、后端 DTO 和生成 OpenAPI 驱动，前端不得手写重复类型。

## 安全、失败与降级

报告与知识库若共享可变正文会失去审计能力；发布快照与人类文件必须分层。

外部 AI、邮件、实时连接或归档不可用时，业务事实先可靠落库，异步重试或降级读取，不在请求链路丢失结果。

## 设计验收边界

- 发布报告不可静默修改
- Feed Token 轮换后旧地址失效
- 重复构建、发布和投递不产生重复事实
