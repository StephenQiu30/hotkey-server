---
layer: Plan
scope: shared
doc_no: "024"
title: 日报周报私有Feed与知识归档计划
status: completed
version: v1.1
owner: HotKey Team
phase: P0
canonical_path: docs/plans/archive/024-日报周报私有Feed与知识归档计划.md
design: docs/design/archive/024-日报周报私有Feed与知识归档设计.md
prd: docs/prd/archive/024-日报周报私有Feed与知识归档.md
---

# 日报周报私有Feed与知识归档计划

## 前置门禁

- Design 已 accepted，PRD 已 approved，依赖编号达到所需状态。
- 先建立当前实现清单和可复现差距；行为变化保存失败测试。
- UI 开发前确认真实 OpenAPI 能力，禁止用 Mock 掩盖缺失后端。

## 预计变更范围

- backend/internal/modules/report/
- backend/internal/modules/delivery/
- backend/internal/modules/knowledge/
- backend/db/schema.sql
- frontend/src/app/dashboard/reports/
- frontend/src/app/dashboard/notifications/page.tsx

## 执行步骤

1. 先以失败测试固定周期 EventUpdate 快照、发布事务、Token 轮换和知识审批边界。
2. 为 `report_items` 增加 EventUpdate 与证据字段，以周期候选读取器替换“当前事件”读取；新增幂等草稿创建 API。
3. 让报告保存与交付仓储复用调用方事务，发布原子完成冻结、投递事实和知识提案，移除报告直写 Vault 路径。
4. 补齐知识文档/提案读接口与操作者字段，更新 Swaggo 注解并生成 OpenAPI 与前端 Client。
5. 用 shadcn/ui 官方组件组合报告、订阅、知识归档三个工作台标签，补齐角色、空态、错误态、响应式和可访问性。
6. 执行后端、前端、Compose 与差异门禁；以 `$agent-browser` 验证 Viewer/Editor/Admin 主故事、Feed 轮换、错误恢复和移动端。
7. 写入 024 Acceptance，把 Design/PRD/Plan 归档并使用 `feat(report): <中文动宾主题>` 独立提交。

## 验证

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 启动完整环境，以浏览器验证登录→监控→采集→事件→告警→报告主故事。
- 根目录运行开发/生产 Compose 配置检查及 `git diff --check`。

## 迁移与回滚

新行为通过配置或 capability 逐步启用。回滚先关闭新入口和任务生产者，等待/取消可重试任务，再回退应用；保留事实、审计、投递和模型运行记录。

## 依赖与功能切片

- 前置编号：020、023。
- 依赖未验收时，本条只允许完成不改变对外行为的基础工作。

- [x] Slice-024-1：周期 EventUpdate 报告快照、幂等创建、预览与不可变发布（AC-024-1、AC-024-2）。
- [x] Slice-024-2：原子投递、日报/周报 email/RSS 订阅与私有 Feed 轮换缓存语义（AC-024-2、AC-024-3）。
- [x] Slice-024-3：知识提案列表、审批、应用、对账及禁止直写 Vault（AC-024-2、AC-024-4）。
- [x] Slice-024-4：报告工作台、角色权限、响应式、错误恢复和完整质量门禁（AC-024-5、AC-024-6）。

## 完成定义

- 发布报告不可静默修改
- 报告条目能追溯周期内 EventUpdate 与证据哈希
- Feed Token 轮换后旧地址失效
- 重复构建、发布和投递不产生重复事实，发布失败整体回滚
- 知识文件只经审批提案应用
- Acceptance 与索引完成后使用 `feat(report): <中文动宾主题>` 提交
