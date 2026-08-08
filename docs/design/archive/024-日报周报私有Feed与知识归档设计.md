---
layer: Design
scope: shared
doc_no: "024"
title: 日报周报私有Feed与知识归档设计
status: accepted
version: v1.1
owner: HotKey Team
phase: P0
canonical_path: docs/design/archive/024-日报周报私有Feed与知识归档设计.md
prd: docs/prd/archive/024-日报周报私有Feed与知识归档.md
plan: docs/plans/archive/024-日报周报私有Feed与知识归档计划.md
---

# 日报周报私有Feed与知识归档设计

## 现状

后端已有报告、订阅、私有 Feed 和知识提案的基础能力，但报告只读取“当前事件”，条目没有固定到 EventUpdate 与证据哈希；发布、投递和归档也不在同一事务边界。前端 `/dashboard/reports` 缐失，现有订阅表单无法配置周报、Feed、监控范围和时区。

## 目标

把高价值事件组织成可预览、可发布、可订阅、可归档的日报周报，并保留每项证据和发布版本。

## 非目标

不在本条复制相邻领域的数据或服务，不以纯前端假数据宣称功能完成，也不引入第二套事实源。

## 核心决策

- Report 构建产生草稿快照，预览不修改事实，发布冻结内容与排序；同一范围和周期保持幂等。
- 构建按报告时区推导 `[period_start, period_end)`，只选择该周期内最新 EventUpdate；每个 ReportItem 固定引用 EventUpdate、摘要、证据集哈希与入选原因。
- 订阅支持 email 与私有 RSS/Atom；Feed Token 可轮换、仅保存摘要，不进入日志。
- 发布事务同时冻结报告、创建幂等投递事实并生成知识提案；任何一步失败都不得暴露“已发布”事实。
- 知识归档通过 proposal→approve/reject→apply→reconcile 流程更新 Vault。发布任务只能创建提案，禁止直接写入人工维护文件。

## 数据、接口与交互

- `POST /api/v1/reports` 由编辑者或管理员按类型、时区、可选监控创建/刷新当前周期草稿；列表与详情对全部已认证用户开放，发布仅管理员可用。
- 报告列表使用 ID 游标；详情包含不可变的条目证据快照。预览只读，已发布报告的构建和发布返回冲突。
- 订阅可选择日报/周报、email/RSS、监控范围、时区和发送时间。RSS 创建或轮换只返回一次明文 Token，页面同时给出 RSS 与 Atom 地址。
- `/feeds/:token` 支持 RSS/Atom、ETag、Last-Modified 与 304；Token 轮换立即使旧地址失效。
- 管理员可查看知识文档与提案、审批或拒绝待审提案、应用已审批提案并执行只读对账；应用前校验文档修订号与 Vault 内容哈希。
- 报告工作台使用 shadcn/ui 的 Tabs、Card、Table、Dialog、Select、Alert、Skeleton 与 Empty 组合，覆盖空态、错误态、键盘和移动端。

所有状态变化保存明确版本、时间和操作者；跨端字段由 Schema、后端 DTO 和生成 OpenAPI 驱动，前端不得手写重复类型。报告候选读取最多 100 条，不引入新搜索引擎、富文本编辑器或模型调用。

## 安全、失败与降级

报告与知识库若共享可变正文会失去审计能力；发布快照与人类文件必须分层。

外部 AI、邮件、实时连接或归档不可用时，业务事实先可靠落库，异步重试或降级读取，不在请求链路丢失结果。

## 设计验收边界

- 发布报告不可静默修改
- 报告条目能追溯到周期内 EventUpdate 与证据集哈希
- Feed Token 轮换后旧地址失效
- 重复构建、发布和投递不产生重复事实，发布失败不留下部分状态
- 报告发布只创建知识提案，人工文件只能经审批应用
