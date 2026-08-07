---
layer: Plan
scope: backend
doc_no: "008"
title: RSS-Atom来源连接器计划
status: completed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/plans/008-RSS-Atom来源连接器计划.md
design: docs/design/008-RSS-Atom来源连接器设计.md
prd: docs/prd/008-RSS-Atom来源连接器.md
---

# RSS-Atom来源连接器计划

## 前置门禁

- Design 状态改为 `accepted`，PRD 状态改为 `approved`。
- 对照当前代码保存可复现差距；行为变化先增加失败测试。
- 涉及第三方来源时，先确认官方/授权接口、条款、配额和测试凭据边界。

## 预计变更范围

- backend/internal/modules/source/infrastructure/rss/
- backend/internal/modules/source/domain/
- backend/internal/modules/source/infrastructure/postgres/
- backend/test/
- docs/design/、docs/prd/、docs/plans/、docs/acceptance/

## 执行步骤

1. [x] 以失败测试固定 4 MiB 响应上限、证据完整度、enclosure 映射和持久化边界。
2. [x] 在既有 Source 领域与 CapturedItem v2 JSON 中增加向后兼容字段；无需新增表、Schema 列、HTTP DTO 或前端 Client。
3. [x] 完成 RSS 2.0、RSS 1.0、Atom 的正文优先与摘要降级，并限制安全附件元数据数量。
4. [x] 复用既有 ETag、Last-Modified、checkpoint、同源分页、DNS/重定向防护和来源健康诊断。
5. [x] 回归采集幂等、Ingestion Content 去重和 CapturedItem 持久化。
6. [x] 使用 `$agent-browser` 验证真实来源配置、失败降级、桌面/移动布局与 WCAG A/AA。
7. [x] 新增 `docs/acceptance/008-RSS-Atom来源连接器验收.md`；本条无新增人工生产操作，不新增 Operations。

## 验证

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 从仓库根运行两套 Compose `config --quiet` 与 `git diff --check`。
- 针对本条逐项验证 AC-008-*，保存长期证据而不是终端流水。

## 迁移与回滚

新增能力默认以未配置或关闭状态上线。Schema 必须向前兼容既有事实；回滚先停用入口和调度，再恢复上一应用版本，不删除已采集证据或审计记录。

## 依赖与功能切片

- 前置编号：007。
- 依赖未验收时，本条只允许完成不改变对外行为的基础工作。

- [x] Slice-008-1：实现「RSS/Atom 解析和增量采集」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-008-* 回归。
- [x] Slice-008-2：实现「条件请求、分页和检查点」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-008-* 回归。
- [x] Slice-008-3：实现「正文与元数据标准映射」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-008-* 回归。
- [x] Slice-008-4：实现「来源健康和诊断」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-008-* 回归。

## 完成定义

- 重复窗口不会重复创建 Content
- 恶意地址、私网解析和跨域重定向被拒绝
- 无正文 Feed 可降级为摘要且标明证据完整度
