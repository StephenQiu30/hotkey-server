---
layer: Acceptance
scope: shared
doc_no: "015"
title: Google可授权搜索迁移验收
status: passed
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/acceptance/015-Google可授权搜索迁移验收.md
design: docs/design/015-Google可授权搜索迁移设计.md
prd: docs/prd/015-Google可授权搜索迁移.md
plan: docs/plans/015-Google可授权搜索迁移计划.md
---

# Google 可授权搜索迁移验收

## 验收结论

015 于 2026-08-07 通过验收。HotKey 新增 `google_agent_search`，只调用 Google Discovery Engine v1 的 Agent Search 官方区域端点，搜索管理员已配置的限定域数据存储。新来源默认停用，必须通过真实 ServingConfig 搜索权限探测后才能启用；系统没有新增 Custom Search JSON API 连接器，也没有 Google 搜索结果页、移动接口或第三方代理回退。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-015-1 | passed | `project`、`location`、区域端点、ServingConfig 完整资源名、Bearer 环境变量引用、Cloud Terms、正文存储和归属标记均由领域与数据库双重约束；来源创建默认 disabled，健康状态非 healthy 时启用返回 409。 |
| AC-015-2 | passed | Connector fixture 验证 Discovery Engine v1 `:search` POST、SafeSearch、snippet、页大小、分页、稳定文档 ID、HTTPS 链接、标题/摘要清理、查询签名绑定游标和幂等映射。 |
| AC-015-3 | passed | 固定官方区域主机、TLS 1.2、DNS 重验证、公共 IP、禁止重定向和响应体上限均有测试；401/403、429、5xx、其他非 2xx、私网 DNS 和畸形结果均安全分类且不泄露上游正文。架构门禁禁止 Custom Search 和 Google 网页搜索地址。 |
| AC-015-4 | passed | OpenAPI、生成 Client、Schema、管理员创建/探测/启用门禁、Viewer 脱敏、未登录跳转、移动端布局与 axe 验收全部通过。 |
| AC-015-5 | passed | 界面明确告知 Custom Search 新客户关闭、2027-01-01 存量停用、Agent Search 仅覆盖配置的数据存储，以及全网搜索需要另行取得 Google 正式方案。 |

## 自动化验证

```bash
cd backend
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:55435/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:56382/15' make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet
git diff --check
```

后端全量 CI 使用 `pgvector/pgvector:pg18`，通过 OpenAPI 再生成一致性、vet、PostgreSQL 18 运行时与容量门禁、数据库升降级、68 表 Schema 指纹、全包测试、构建、架构和仓库门禁。前端 OpenAPI 契约、类型检查、37 个测试文件共 142 项测试及 Next.js 生产构建通过。Compose 开发/生产配置通过；生产配置检查使用本地占位环境变量，没有启动或修改生产服务。

多 TSX 变更按 React 最佳实践复核：新增交互保持在既有受控表单中，没有增加 Effect、派生状态同步、请求瀑布、动态重依赖或无必要组件层；界面继续组合 shadcn/ui Select、Input、Alert、Checkbox 和 Button。

## 浏览器验收

使用用户指定的 `$agent-browser` 连接真实 Next.js、Go、PostgreSQL、Redis 与 MinIO：

- 未认证访问 `/dashboard/sources` 跳转到 `/login?redirect=%2Fdashboard%2Fsources`。
- 管理员能选择“Google Agent Search（限定域）”；区域会绑定对应官方端点，Bearer、Cloud Terms、正文存储和归属标记不可降级修改，ServingConfig 必须与位置匹配。
- 完整表单创建的是停用来源。验收环境未配置测试 Token 时，探测显示凭据不可用；随后启用请求返回 409，来源仍为停用，证明不能绕过健康门禁。
- Viewer 能看到来源名称、`google_agent_search` 类型和停用状态，但看不到新增、探测、启用、删除按钮、官方端点或环境变量引用。
- 390×844 下弹窗宽度与页面同为 390px，无横向溢出。Google 模态 axe-core 4.12.1 为 0 violations；2 个 incomplete 项来自 Radix 模态背景焦点守卫和覆盖层颜色计算，人工复核通过。Viewer 来源页为 0 violations、0 incomplete。
- 浏览器页面错误为空，控制台只有 React 开发提示和 HMR 连接信息。

截图和快照保存在本地 `/tmp/hotkey-015-dogfood/`，不提交为仓库事实源。

## 官方依据与已知限制

- [Custom Search JSON API 概览](https://developers.google.com/custom-search/v1/overview)说明 API 已关闭新客户，并要求存量客户在 2027-01-01 前迁移。
- [Agent Search 官方文档](https://cloud.google.com/generative-ai-app-builder/docs/enterprise-search-introduction)说明该产品基于 Discovery Engine API，并面向配置的数据存储提供搜索能力。
- [Discovery Engine REST search](https://cloud.google.com/generative-ai-app-builder/docs/reference/rest/v1/projects.locations.collections.dataStores.servingConfigs/search)定义 ServingConfig 资源名、区域端点、OAuth/IAM、分页与结果结构。
- 仓库和验收环境没有运营方 Google Cloud 项目、已配置的数据存储或真实访问令牌，因此不会向 Google 生产接口发起计费调用。发布前管理员必须配置数据存储、IAM、短期 Token 环境变量，并让健康探测真实通过；fixture 只验证官方请求/响应契约，不能代替云端权限验收。
- Agent Search 在本项目中是限定域数据存储搜索，不代表 Google 全网搜索。若产品需要全网范围，必须取得 Google 正式商业方案并重新走 Design → PRD → Plan → Acceptance，不得以旧 Custom Search 或网页抓取补齐。

## 回滚

回滚时先停用所有 `google_agent_search` 来源和调度，再回退应用。Schema 只扩展受控来源枚举、配置字段和约束，没有新增表；历史 Content、检查点、CollectionRun 与诊断记录保留，不通过删除事实回滚。
