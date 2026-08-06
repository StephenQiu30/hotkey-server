---
layer: Design
scope: backend
doc_no: "015"
title: Google可授权搜索迁移设计
status: proposed
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/design/015-Google可授权搜索迁移设计.md
prd: docs/prd/015-Google可授权搜索迁移.md
plan: docs/plans/015-Google可授权搜索迁移计划.md
---

# Google可授权搜索迁移设计

## 现状

参考项目包含 Google 搜索实现；Google Custom Search JSON API 已对新客户关闭，存量客户也将在 2027-01-01 停用。

## 目标

为存量 Custom Search 客户提供短期兼容，同时把新部署迁移到可获得授权的限定域搜索或官方全网搜索方案。

## 非目标

不使用未授权网页抓取、浏览器会话、验证码绕过或不稳定私有接口补齐能力；本条不实现其他编号的业务细节。

## 核心决策

- 新部署不得依赖 Custom Search JSON API；存量适配器必须带明确 sunset 日期和迁移告警。
- 限定域场景优先评估 Vertex AI Search 等官方产品；全网搜索必须取得 Google 提供的正式方案与条款。
- 旧适配器只保存返回的标题、链接、摘要、时间和查询元数据，不抓取结果页。
- 能力档案记录 provider、contract_version 和 deprecation_at，到期自动阻止新 Monitor 发布。

## 数据、接口与交互

- 存量 Custom Search 兼容连接器
- 弃用检测和迁移提示
- 限定域/全网搜索能力抽象
- 成本、配额和合同审计

跨端契约先修改 Schema 与后端注解，再生成 OpenAPI 和前端 Client。来源能力必须在发布监控前可检测，禁用或阻塞状态不能进入调度。

## 安全、失败与降级

替代产品不一定提供与旧 API 等价的全网原始结果；产品界面必须准确表达差异。

第三方失败按认证、限流、临时、解析和永久错误分类；关闭单个来源不影响历史证据和其他来源。

## 设计验收边界

- 新客户配置流程不会引导创建已关闭 API
- 2027 停用日前有可观测迁移门禁
- 无替代授权时来源安全停用而非转网页抓取

## 官方依据

- https://developers.google.com/custom-search/v1/overview
