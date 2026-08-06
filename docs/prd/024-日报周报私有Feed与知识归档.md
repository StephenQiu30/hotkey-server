---
layer: PRD
scope: shared
doc_no: "024"
title: 日报周报私有Feed与知识归档
status: draft
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/prd/024-日报周报私有Feed与知识归档.md
design: docs/design/024-日报周报私有Feed与知识归档设计.md
plan: docs/plans/024-日报周报私有Feed与知识归档计划.md
---

# 日报周报私有Feed与知识归档

## 用户问题

日报/周报构建、预览、冻结、发布、SMTP、私有 RSS/Atom、Obsidian Vault 和知识提案审批已在后端实现，前端只有订阅页，缺少完整报告工作台。

## 产品目标

把高价值事件组织成可预览、可发布、可订阅、可归档的日报周报，并保留每项证据和发布版本。

## 范围

- 报告列表、详情、构建、预览和发布
- 日报/周报订阅
- 邮件与私有 Feed
- 知识提案、审批、同步和对账

## 用户故事

- 作为阅读者，我能快速看到重要变化、依据、可信程度和更新时间。
- 作为编辑者，我能从发现、研判到处理和交付完成闭环。
- 作为管理员，我能查看成本、失败、权限和审计并安全恢复。

## 功能要求

- FR-024-1：Report 构建产生草稿快照，预览不修改事实，发布冻结内容与排序；同一范围和周期保持幂等。
- FR-024-2：每个 ReportItem 引用 EventUpdate、摘要和证据，不复制不可追溯的模型文本。
- FR-024-3：订阅支持 email 与私有 RSS/Atom；Feed Token 可轮换、仅保存摘要，不进入日志。
- FR-024-4：知识归档通过 proposal→approve/reject→apply→reconcile 流程更新 Vault，不让任务直接覆盖人工文件。

## 非功能要求

- 写操作具备认证授权、审计、幂等或乐观锁；异步操作可追踪和重试。
- 列表使用稳定分页，慢任务不阻塞 HTTP；错误使用统一业务码。
- 前端使用 shadcn/ui/Radix 组合，覆盖响应式、键盘、焦点、对比度与减弱动效。

## 非目标

不以静态 Mock、不可追溯模型文本或仅桌面可用页面通过验收。

## 验收标准

- AC-024-1：发布报告不可静默修改
- AC-024-2：Feed Token 轮换后旧地址失效
- AC-024-3：重复构建、发布和投递不产生重复事实
