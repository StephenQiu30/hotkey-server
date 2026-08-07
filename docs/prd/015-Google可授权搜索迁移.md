---
layer: PRD
scope: fullstack
doc_no: "015"
title: Google可授权搜索迁移
status: approved
version: v1.1
owner: HotKey Team
phase: P2
canonical_path: docs/prd/015-Google可授权搜索迁移.md
design: docs/design/015-Google可授权搜索迁移设计.md
plan: docs/plans/015-Google可授权搜索迁移计划.md
---

# Google 可授权搜索迁移

## 用户问题

管理员需要接入 Google 官方搜索能力，但 Custom Search JSON API 已关闭新客户并将在 2027-01-01 停用。未经授权地抓取 Google 搜索页既不稳定，也不满足本项目事实来源要求。

## 产品目标

用最少功能提供可授权的 Agent Search 限定域关键词采集，准确说明它与全网搜索的差异，并阻止旧 API 或网页抓取进入新部署。

## MVP 范围

- Agent Search 来源创建、健康探测、启停和关键词采集。
- `global`、`us`、`eu` 官方 Discovery Engine v1 端点。
- ServingConfig 资源、短期 Bearer env 引用、分页检查点和安全结果映射。
- 固定能力档案、旧 API 停用提示和无全网授权门禁。
- 管理员配置与脱敏阅读者视图。

## 非目标

- 不实现 Custom Search JSON API 新连接器、Google 搜索网页抓取或验证码绕过。
- 不代替用户创建 Google Cloud 项目、数据存储、域名验证、IAM 或计费。
- 不实现 Google 单独签约的全网搜索方案，也不把 Agent Search 描述为全网搜索。
- 不保存服务账号 JSON、长期明文令牌或抓取结果页正文。

## 功能要求

- FR-015-1：管理员选择 Agent Search 后必须填写 location、完整 ServingConfig 和 `env:NAME`，连接默认禁用。
- FR-015-2：服务端必须固定官方端点并验证资源归属；健康探测通过前不得启用。
- FR-015-3：采集必须支持关键词、官方页令牌、SafeSearch 和 snippet，映射到既有证据流水线。
- FR-015-4：来源界面必须展示 provider、v1 合同、旧 API 停用日及“限定域而非全网搜索”。
- FR-015-5：缺少正式全网授权时只安全停用，不调用网页、移动端或未文档化接口。

## 非功能要求

- 所有外部请求采用 TLS 1.2+、DNS/重定向复验、超时和 4 MiB 响应限制。
- 凭据只保存环境变量引用，日志、错误、API 和前端不得回显令牌。
- 采集和检查点保持幂等；401/403、429、5xx、4xx 与解析失败准确分类。
- 使用既有 shadcn/ui 组件和页面视觉体系，不新增非必要自定义样式。

## 验收标准

- AC-015-1：新客户配置流程不出现已关闭 API，并明确 2027-01-01 停用时间。
- AC-015-2：配置、健康门禁与权限脱敏全部生效。
- AC-015-3：搜索、分页、映射和完整失败边界自动化测试通过。
- AC-015-4：无授权时不降级为 Google 网页抓取。
- AC-015-5：全量后端、前端、Compose 与 agent-browser 验收通过。
