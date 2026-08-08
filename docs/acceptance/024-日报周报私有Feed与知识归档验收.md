---
layer: Acceptance
scope: fullstack
doc_no: "024"
title: 日报周报私有Feed与知识归档验收
status: passed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/acceptance/024-日报周报私有Feed与知识归档验收.md
design: docs/design/archive/024-日报周报私有Feed与知识归档设计.md
prd: docs/prd/archive/024-日报周报私有Feed与知识归档.md
plan: docs/plans/archive/024-日报周报私有Feed与知识归档计划.md
---

# 日报周报私有Feed与知识归档验收

## 验收结论

024 于 2026-08-08 通过验收。报告草稿按时区周期选取最新 EventUpdate，并冻结摘要、热度、理由码和证据集哈希；发布在一个事务中冻结报告、创建幂等交付与待审知识提案。日报/周报订阅支持邮件和私有 RSS/Atom，Token 只展示一次且轮换后旧地址立即失效。知识文档只有在管理员批准提案、基线校验和快照保存成功后才更新 Vault 自动区域，对账可识别外部漂移。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-024-1 | passed | 应用、仓储和 HTTP 测试覆盖周期边界、最新 EventUpdate、证据字段、幂等草稿、只读预览、已发布不可重建与不可重发；真实预览固定到 EventUpdate #2/#3 和对应证据哈希。 |
| AC-024-2 | passed | PostgreSQL 集成测试覆盖发布事务与数据库分配 ID；真实发布后严格得到 1 个 Report、2 个 ReportItem、1 个 Delivery、1 个 KnowledgeDocument、1 个 pending Proposal 和 1 个 River Job。 |
| AC-024-3 | passed | 浏览器创建邮件与私有 Feed 订阅；RSS/Atom 均返回 200，ETag 条件请求返回 304，Last-Modified 使用发布时间；轮换后旧 Token 返回 404，新 Token 的 RSS/Atom 均可用。 |
| AC-024-4 | passed | 知识应用测试覆盖未审批、修订冲突、快照失败不写 Vault 和完整文件哈希；真实流程完成 preview→approve→apply→reconcile，文档修订号升至 1，MinIO 快照存在，对账为“扫描 1，冲突 0”。 |
| AC-024-5 | passed | Admin 完成创建、预览、发布、订阅、知识审批应用；Editor 有草稿入口但无发布或知识归档；Viewer 只有查看与订阅。错误态可重试恢复，桌面/移动、键盘焦点与自动可访问性均通过。 |
| AC-024-6 | passed | 后端 CI、OpenAPI 漂移、前端类型/177 项单测/Webpack 构建、开发与生产 Compose、差异检查及 `$agent-browser` 主故事全部通过。 |

## 自动化验证

```bash
cd backend
HOTKEY_TEST_DSN="${HOTKEY_TEST_DSN}" \
HOTKEY_TEST_REDIS_URL="${HOTKEY_TEST_REDIS_URL}" make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build -- --webpack

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet
git diff --check
```

后端在 PostgreSQL 18 上通过 OpenAPI 再生成一致性、vet、数据库运行时与容量夹具、全部 Go 包、构建、架构、Repository 和 71 表 Schema 门禁。前端通过 44 个测试文件共 177 项测试、生成 Client 一致性、TypeScript 检查和 Next.js 16.3.0 Webpack 生产构建，共生成 21 个路由。

## 浏览器验收

按用户指定的 `$agent-browser` 连接真实 Next.js、Go、PostgreSQL、Redis 和隔离 MinIO，完成以下验收：

- 匿名访问安全跳转登录；Admin 创建当前日报草稿，列表显示 2 个条目，预览显示 EventUpdate #2/#3、理由码和证据集哈希。
- 创建邮件与私有 Feed 订阅后发布，数据库确认报告、条目、投递、知识提案和 River Job 原子落库；Feed 内容只含两个冻结条目。
- Admin 完成提案查看、批准、应用和对账，Vault 自动区域写入成功且人工区域边界保留；快照对象存在，对账无冲突。
- Editor 可创建草稿但无发布和知识归档入口；Viewer 无创建、发布和知识入口，仍可预览已发布报告与管理自己的订阅。
- 拦截报告 API 后页面显示中文错误与“重试”，解除拦截即可恢复；浏览器无应用 JavaScript 错误。
- 1440×900 和 390×844 均无横向溢出；Radix Dialog 初始聚焦报告周期，Tab 保持在对话框内，Escape 关闭并恢复“新建报告”焦点。
- 桌面与移动稳定页的 axe-core 4.12.1 WCAG 2 A/AA 均为 0 violations、0 incomplete。验收发现并修复未选中 Tabs 的 4.34:1 对比度后复测通过。

截图保存在本地 `/tmp/hotkey-024-desktop.png` 与 `/tmp/hotkey-024-mobile.png`，不进入仓库。

## 安全与产品边界

- 私有 Feed 仅持久化 Token 摘要，明文只在创建和轮换响应中出现；轮换立即使旧摘要失效。
- 发布只生成知识提案，禁止 Worker 或报告服务直接覆盖 Vault；快照失败时 Vault 保持不变。
- Feed 只公开已发布冻结快照，不回读可变事件正文；知识 API 使用生成 OpenAPI DTO，不暴露存储实现细节。
- 本条未引入富文本编辑器、第二套搜索、模型调用或自定义 Feed 模板。

## 回滚

先停止报告构建、发布和交付任务生产者，等待或取消可重试任务，再回退应用。已发布报告、条目、交付、知识提案、修订和快照作为已发生事实保留；不自动删除 Vault 文件或对象存储快照。
