---
layer: Acceptance
scope: backend
doc_no: "008"
title: RSS-Atom来源连接器验收
status: passed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/acceptance/008-RSS-Atom来源连接器验收.md
design: docs/design/archive/008-RSS-Atom来源连接器设计.md
prd: docs/prd/archive/008-RSS-Atom来源连接器.md
plan: docs/plans/archive/008-RSS-Atom来源连接器计划.md
---

# RSS-Atom来源连接器验收

## 验收结论

008 通过验收。连接器支持 RSS 2.0、RSS 1.0 与 Atom，具备条件请求、同源分页、检查点、稳定外部 ID、4 MiB 响应上限和安全网络边界。正文、摘要与仅元数据的证据完整度会随 CapturedItem 持久化；RSS enclosure 与 Atom enclosure 只保留经过约束的元数据，不下载二进制内容。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-008-1 | passed | CollectionRepository 对同一运行和 external ID 的重复持久化保持单条 CapturedItem；Ingestion 回归确认重复运行不重新处理 capture，同一外部 ID 的 Content upsert 与证据对象写入均幂等。 |
| AC-008-2 | passed | RSS 连接器测试覆盖 HTTPS 入口、私网/保留地址 DNS、DNS 重绑定、跨域 cursor/分页/重定向、含凭据形态跳转和重定向上限；所有拒绝均返回安全分类，不回显地址凭据。 |
| AC-008-3 | passed | RSS description 与 Atom summary 在无完整 content 时映射为 `summary_only`；完整 content 标为 `full_body`，无正文或关闭正文保存标为 `metadata_only`。领域和 PostgreSQL 集成测试确认标记与安全 enclosure 元数据跨 CapturedItem v2 JSON 持久化。 |

## 自动化验证

```bash
cd backend
PATH=/opt/homebrew/opt/postgresql@16/bin:$PATH \
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:55432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod \
  -f docker-compose-prod.yml config --quiet
git diff --check
```

最终门禁在只包含本任务暂存树的隔离工作区执行，排除用户未提交的 Compose 配置和 IDE 文件。后端覆盖 OpenAPI 再生成、vet、数据库运行时、全量测试、构建、架构和 Schema；前端覆盖 OpenAPI 契约、类型、全量单元测试与 Next.js 生产构建。

## 浏览器验收

使用用户指定的 `$agent-browser` 连接真实 Next.js、Go、PostgreSQL 与 Redis：

- 管理员创建 HTTPS RSS/Atom 来源，保存条款地址、正文/摘要保存许可、归属要求、语言地区、保留期、配额、超时和分页上限。
- 页面明确说明只保存 Feed 实际提供的正文或摘要、不抓取原网页；对不可解析的测试域名执行探测后安全显示“不可用”，来源目录仍可读取。
- 1440×1000 桌面和 390×844 移动视口均无整页或对话框水平溢出；移动对话框初始焦点位于可滚动配置区。
- axe 4.13.0 对移动来源列表与配置对话框执行 WCAG 2 A/AA 审计，确认违规为 0；对话框背景焦点和无法判定的单个对比度仅列为 incomplete。浏览器没有应用 JavaScript 错误。

截图保存在本地 `/tmp/hotkey-008-desktop-sources.png`、`/tmp/hotkey-008-mobile-sources.png` 与 `/tmp/hotkey-008-mobile-dialog.png`，不提交为仓库事实源。

## 失败、降级与回滚

- 304 返回空增量并保留根 Feed validator；限流、认证、临时错误、永久错误和解析错误保持独立分类。
- 单页 Feed 响应超过 4 MiB 时永久拒绝；项目数、页数和超时继续使用连接配置硬上限。
- 无效或超过 32 个的 enclosure 不进入持久化；本条不下载附件，也不引入任务 018 的对象存储职责。
- 本条不增加数据库表、列或公开 API。回滚应用版本不会删除既有 CapturedItem、Content、checkpoint 或审计事实；新增 JSON 字段对旧 v1/v2 capture 保持可读兼容。
