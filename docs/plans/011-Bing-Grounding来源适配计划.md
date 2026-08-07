---
layer: Plan
scope: backend
doc_no: "011"
title: Bing-Grounding来源适配计划
status: completed
version: v1.1
owner: HotKey Team
phase: P2
canonical_path: docs/plans/011-Bing-Grounding来源适配计划.md
design: docs/design/011-Bing-Grounding来源适配设计.md
prd: docs/prd/011-Bing-Grounding来源适配.md
---

# Bing-Grounding来源适配计划

## 前置门禁

- Design 已 accepted，PRD 已 approved。
- 官方 Foundry Toolbox MCP 契约、Entra scope、Web Search 参数、数据边界、条款和配额已核实；阻塞项不得用非官方接口绕过。
- 先保存 Connector 契约、查询编译、分页、限流、失败分类和 SSRF 的失败测试。

## 预计变更范围

- backend/internal/modules/source/infrastructure/binggrounding/
- backend/db/schema.sql
- frontend/src/app/dashboard/sources/
- backend/test/

## 执行步骤

1. 在 Source Domain 增加最小能力枚举与严格配置，不泄漏第三方 SDK 类型。
2. 在唯一 Schema 增加必要字段和约束，保持既有 RSS/HN 数据兼容。
3. 实现绑定不可变 SourceConnection 的流式 MCP Connector、一次查询、限流和错误分类。
4. 将 Capture 接入既有 normalize→relevance→cluster 流水线，不建立旁路存储。
5. 更新来源/监控 API 注解，生成 OpenAPI 与前端 Client。
6. 用 shadcn/ui 组合来源配置、授权状态、健康诊断和能力预览。
7. 运行契约、集成、并发、重试、权限和端到端测试，新增同编号 Acceptance。

## 验证

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 使用可控 HTTP fixture 验证 MCP 握手、工具契约、SSE/JSON 成功响应、429、401、超时、恶意重定向、超限响应和撤销授权。
- 使用 agent-browser 验证管理员和只读用户的桌面/移动端创建、合规门禁、探测、启用、错误态、无横向溢出和可访问性。
- 根目录运行 Compose 配置检查与 `git diff --check`。

## 迁移与回滚

新来源默认 disabled，并以能力开关逐一启用。回滚时先停止该来源的新调度、等待运行任务结束，再回退应用；保留检查点、采集运行和历史证据。

## 依赖与功能切片

- 前置编号：006、007。
- 依赖未验收时，本条只允许完成不改变对外行为的基础工作。

- [x] Slice-011-1：实现「Foundry 凭据与能力配置」，绑定 AC-011-1/3/4。
- [x] Slice-011-2：实现「带引用回答到候选证据的映射」，绑定 AC-011-2/5。
- [x] Slice-011-3：实现「查询、成本和数据边界审计」，绑定 AC-011-4/5。
- [x] Slice-011-4：实现「停用旧 Bing 选项和迁移提示」，绑定 AC-011-1/6。

## 完成定义

- 代码中不存在对已停用 Search API 的调用
- 所有 Bing 派生内容明确显示模型生成和引用
- 无 Foundry 配置时来源保持 disabled 而不影响其他连接器
