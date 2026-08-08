---
layer: Plan
scope: fullstack
doc_no: "015"
title: Google可授权搜索迁移计划
status: completed
version: v1.1
owner: HotKey Team
phase: P2
canonical_path: docs/plans/archive/015-Google可授权搜索迁移计划.md
design: docs/design/archive/015-Google可授权搜索迁移设计.md
prd: docs/prd/archive/015-Google可授权搜索迁移.md
---

# Google 可授权搜索迁移计划

## 前置门禁

- [x] Design 已 accepted，PRD 已 approved。
- [x] 官方停用政策、Discovery Engine v1、location、OAuth/IAM、分页和 snippet 契约已核实。
- [x] 确认仓库不存在需迁移的既有 Google 连接，因此不建立旧 API 兼容器。

## 执行步骤

1. [x] 先补严格配置、Registry、Connector、旧端点禁止与服务启用门禁的失败测试。
2. [x] 增加 `google_agent_search` 类型和最小 Google 配置，更新唯一 Schema 与 API 契约。
3. [x] 实现固定官方端点的健康探测、关键词搜索、分页、映射和错误分类。
4. [x] 用 shadcn/ui 组合来源配置、迁移告警和能力边界，保持创建默认禁用。
5. [x] 生成 OpenAPI 与前端 Client，补齐领域、集成、权限和界面测试。
6. [x] 运行后端/前端全量回归、Compose 检查和 `git diff --check`。
7. [x] 使用 agent-browser 验收管理员、阅读者、未登录、移动端、错误态和可访问性。
8. [x] 新建同编号 Acceptance，按中文 Conventional Commits 独立提交。

## 验证命令

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 根目录运行 Compose 开发/生产配置检查与 `git diff --check`。

## 回滚

新来源默认 disabled。回滚时先停用来源并等待运行任务结束，再回退应用；保留采集运行、检查点和历史证据，不转用 Custom Search 或网页抓取。

## 完成定义

- AC-015-1..5 全部绑定自动化或浏览器证据。
- 没有真实凭据、临时验收文件或生成漂移进入提交。
- 工作区只包含第 015 项变更并完成独立中文提交。
