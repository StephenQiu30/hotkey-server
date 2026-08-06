---
layer: Plan
scope: shared
doc_no: "013"
title: Bilibili开放平台与账号监控计划
status: planned
version: v1.0
owner: HotKey Team
phase: P1
canonical_path: docs/plans/013-Bilibili开放平台与账号监控计划.md
design: docs/design/013-Bilibili开放平台与账号监控设计.md
prd: docs/prd/013-Bilibili开放平台与账号监控.md
---

# Bilibili开放平台与账号监控计划

## 前置门禁

- Design 已 accepted，PRD 已 approved。
- 官方/授权接口、应用 scope、条款、配额和测试凭据已核实；阻塞项不得用非官方接口绕过。
- 先保存 Connector 契约、查询编译、分页、限流、失败分类和 SSRF 的失败测试。

## 预计变更范围

- backend/internal/modules/source/infrastructure/bilibili/
- backend/internal/modules/source/domain/
- backend/internal/modules/monitor/
- backend/db/schema.sql
- frontend/src/app/dashboard/sources/
- frontend/src/app/dashboard/settings/

## 执行步骤

1. 在 Source Domain 增加最小能力枚举与严格配置，不泄漏第三方 SDK 类型。
2. 在唯一 Schema 增加必要字段和约束，保持既有 RSS/HN 数据兼容。
3. 实现绑定不可变 SourceConnection 的 Connector、分页检查点、限流和错误分类。
4. 将 Capture 接入既有 normalize→relevance→cluster 流水线，不建立旁路存储。
5. 更新来源/监控 API 注解，生成 OpenAPI 与前端 Client。
6. 用 shadcn/ui 组合来源配置、授权状态、健康诊断和能力预览。
7. 运行契约、集成、并发、重试、权限和端到端测试，新增同编号 Acceptance。

## 验证

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 使用可控官方沙盒或 HTTP fixture 验证成功、分页、429、401、超时、恶意重定向和撤销授权。
- 根目录运行 Compose 配置检查与 `git diff --check`。

## 迁移与回滚

新来源默认 disabled，并以能力开关逐一启用。回滚时先停止该来源的新调度、等待运行任务结束，再回退应用；保留检查点、采集运行和历史证据。

## 依赖与功能切片

- 前置编号：005、006、007。
- 依赖未验收时，本条只允许完成不改变对外行为的基础工作。

- [ ] Slice-013-1：实现「B 站应用授权和账号绑定」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-013-* 回归。
- [ ] Slice-013-2：实现「账号目标解析与确认」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-013-* 回归。
- [ ] Slice-013-3：实现「稿件、专栏、直播事件采集」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-013-* 回归。
- [ ] Slice-013-4：实现「Webhook 验签、轮询补偿和指标快照」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-013-* 回归。

## 完成定义

- 未授权账号数据不会被读取
- 相同内容在 Webhook 与轮询并发时只生成一条 Content
- 授权撤销后停止采集并保留合规所需审计
