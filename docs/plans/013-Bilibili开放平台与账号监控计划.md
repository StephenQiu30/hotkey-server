---
layer: Plan
scope: shared
doc_no: "013"
title: Bilibili开放平台与授权账号监控计划
status: completed
version: v1.1
owner: HotKey Team
phase: P1
canonical_path: docs/plans/013-Bilibili开放平台与账号监控计划.md
design: docs/design/013-Bilibili开放平台与账号监控设计.md
prd: docs/prd/013-Bilibili开放平台与账号监控.md
---

# Bilibili开放平台与授权账号监控计划

## 前置门禁

- [x] Design 已 accepted，PRD 已 approved。
- [x] 已核实官方 OAuth、scopes、v2 签名、视频/专栏列表、Webhook 与隐私条款。
- [x] 已确认开放平台不支持任意公共账号搜索，MVP 不以非官方接口绕过。

## 执行步骤

1. [x] 扩展 Source Domain、固定配置、Schema 约束和启用健康门禁。
2. [x] 实现 Bilibili v2 签名连接器、scopes 健康探测、视频/专栏分页游标和安全错误分类。
3. [x] 实现 Webhook 原始请求体验签、challenge、撤销停用、幂等回执和审计。
4. [x] 更新 Source API、OpenAPI Client 与 shadcn/ui 来源表单/能力说明。
5. [x] 完成领域、连接器、数据库、传输、权限、脱敏、并发/重试测试。
6. [x] 运行后端 CI、前端全量测试与构建、Compose、diff 检查及 agent-browser 桌面/移动验收。
7. [x] 新建并通过同编号 Acceptance，按中文 Conventional Commits 提交。

## 测试清单

- 正常：scopes、视频、专栏、分页游标、challenge、撤销、管理员配置。
- 失败：缺少凭据、错误 OpenID、scope 不足、401/403/429/5xx、超时、畸形 JSON、恶意重定向、错误签名、过大回调。
- 幂等：重复轮询、重复撤销事件、相同内容跨轮次。
- 权限与脱敏：Viewer 无写入口；API、日志、回执、错误均无凭据和原始回调。
- UI：1440×900 与 390×844 无横向溢出，关键流程 axe 零违规。

## 迁移与回滚

新来源默认 disabled。回滚时先停用 Bilibili 调度与 Webhook 路由，再回退应用；保留历史 Content、CollectionRun、回执和审计记录。Schema 采用可重复执行的增量约束，不删除既有事实。

## 完成定义

- [x] AC-013-1 至 AC-013-5 全部通过。
- [x] 自动化测试、构建、Compose、浏览器验收全部通过。
- [x] Acceptance 记录证据与清理结果，工作树仅包含本任务提交。
