---
layer: Plan
scope: frontend
doc_no: "002"
title: Vercel风格与shadcn-ui设计系统计划
status: planned
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/plans/002-Vercel风格与shadcn-ui设计系统计划.md
design: docs/design/002-Vercel风格与shadcn-ui设计系统设计.md
prd: docs/prd/002-Vercel风格与shadcn-ui设计系统.md
---

# Vercel风格与shadcn-ui设计系统计划

## 前置门禁

- Design 状态改为 `accepted`，PRD 状态改为 `approved`。
- 对照当前代码保存可复现差距；行为变化先增加失败测试。
- 涉及第三方来源时，先确认官方/授权接口、条款、配额和测试凭据边界。

## 预计变更范围

- frontend/components.json
- frontend/src/app/globals.css
- frontend/src/components/ui/
- frontend/src/components/dashboard/
- frontend/src/layouts/BasicLayout.tsx

## 执行步骤

1. 为验收标准建立领域、应用、Transport 和前端测试，记录预期失败。
2. 在 `backend/db/schema.sql` 完成最小数据变化；不创建 migration 目录或第二套 Schema。
3. 按 domain→application→infrastructure→transport 实现后端最小闭环，并加入审计与可观测性。
4. 生成 OpenAPI，再生成前端 Client；禁止手写 DTO 或接口路径。
5. 使用 shadcn/ui 组合实现页面的正常、加载、空、错误和权限状态。
6. 完成并发、重试、幂等、权限、可访问性和端到端回归。
7. 新增 `docs/acceptance/002-Vercel风格与shadcn-ui设计系统验收.md`；如有可重复人工操作，再新增 Operations。

## 验证

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 从仓库根运行两套 Compose `config --quiet` 与 `git diff --check`。
- 针对本条逐项验证 AC-002-*，保存长期证据而不是终端流水。

## 迁移与回滚

新增能力默认以未配置或关闭状态上线。Schema 必须向前兼容既有事实；回滚先停用入口和调度，再恢复上一应用版本，不删除已采集证据或审计记录。

## 依赖与功能切片

- 前置编号：001。
- 依赖未验收时，本条只允许完成不改变对外行为的基础工作。

- [ ] Slice-002-1：实现「颜色、字体、间距、圆角和阴影令牌」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-002-* 回归。
- [ ] Slice-002-2：实现「导航、表单、表格、筛选、对话框、空态和反馈组合规范」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-002-* 回归。
- [ ] Slice-002-3：实现「桌面/平板/移动断点」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-002-* 回归。
- [ ] Slice-002-4：实现「正常、加载、空、错误、权限不足状态」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-002-* 回归。

## 完成定义

- 业务页面不再自建可由 shadcn/ui 提供的基础组件
- 键盘、焦点、标签、对比度和减弱动效通过检查
- 全部现有页面使用统一 Header、容器、状态与反馈模式
