---
layer: Acceptance
scope: shared
doc_no: "011"
title: Bing-Grounding来源适配验收
status: passed
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/acceptance/011-Bing-Grounding来源适配验收.md
design: docs/design/011-Bing-Grounding来源适配设计.md
prd: docs/prd/011-Bing-Grounding来源适配.md
plan: docs/plans/011-Bing-Grounding来源适配计划.md
---

# Bing-Grounding来源适配验收

## 验收结论

011 通过验收。实现只连接 Microsoft Foundry Toolbox Web Search MCP，不调用已停用的 Bing Search API。连接器以标准 MCP 初始化、工具清单和流式工具调用完成查询，把模型生成回答保存为单条 `summary_only` 派生证据，把 URL 引用保存为附件；不把回答标记为原始网页正文，也不生成来源指标。新来源强制停用，只有显式确认数据边界且健康探测成功后才可启用。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-011-1 | passed | 架构门禁扫描生产代码中的退休 Bing 主机和 `/v7.0/search` 路径；连接器只接受版本固定、带 `api-version=v1` 的 Foundry Toolbox HTTPS MCP 地址。 |
| AC-011-2 | passed | 连接器与 UI 回归确认标题、作者和列表均明确标识“模型生成的派生证据”，正文保留派生回答，URL 引用作为附件和首个证据地址保存。 |
| AC-011-3 | passed | 新建来源固定为 disabled；运行时环境变量缺失时健康探测返回安全凭据诊断，其他 RSS、Hacker News 与 X 连接器注册和测试不受影响。 |
| AC-011-4 | passed | Domain、应用集成与连接器测试覆盖数据边界未确认、凭据缺失、工具清单不唯一、未探测、401/403、429、5xx、私网解析、跨地址重定向和超限响应；错误不含 Token、凭据值或上游正文。 |
| AC-011-5 | passed | SSE 与 JSON 契约回归确认每次查询只调用唯一 `web_search` 工具，只产生一个 `summary_only` 证据，保留内联回答与 URL 引用，指标能力明确拒绝该来源。 |
| AC-011-6 | passed | 真实桌面/移动页面完成合规确认、创建、探测失败、启用门禁和 viewer 只读验收；无横向溢出，axe WCAG 2A/2AA 为 0 violations。 |

## 自动化验证

```bash
cd backend
PATH=/opt/homebrew/opt/postgresql@16/bin:$PATH \
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:55441/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:56381/15' make ci

go run ./test/runner test -race \
  ./internal/modules/source/infrastructure/binggrounding -count=1

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

后端全量 CI 通过 OpenAPI 再生成一致性、vet、PostgreSQL 16 升降级、67 表 Schema 指纹、全量测试、构建、架构与仓库门禁；Bing Grounding 包额外通过竞态检测。前端 OpenAPI 契约、类型检查、37 个测试文件共 138 项单元测试和 Next.js 生产构建全部通过，两套 Compose 配置有效。

## 浏览器验收

使用用户指定的 `$agent-browser` 连接真实 Next.js、Go、PostgreSQL 与 Redis：

- 未认证访问来源页会跳转到带安全 redirect 的登录页；管理员可选择 Microsoft Foundry Web Search，授权方式固定为 Bearer，版本化 Toolbox MCP 地址可配置，正文存储、归属标记和单页限制固定为安全值。
- 数据边界确认前创建按钮不可用；明确确认 Microsoft DPA 不适用、可能跨越合规与地理边界以及额外条款和费用后才允许创建。创建结果固定为“已停用”。
- 健康探测前启用被拒绝；服务端缺少令牌环境变量时，探测显示“来源凭据不可用，请检查服务端环境变量”，不回显引用名、Token 或上游正文，来源保持停用。
- viewer 只看到只读目录、条款链接和“模型生成的派生证据 · 保留引用”，看不到新增、探测、启用或删除操作。
- dogfood 发现并修复两项文案问题：Grounding 曾沿用 Feed 正文说明，Foundry 地址曾沿用 RSS 占位符；修复后都按来源类型显示并有单元回归。
- 1440×1000 桌面和 390×844 移动视口均无水平溢出；来源页和 Foundry 弹窗 axe-core 4.12.1 WCAG 2 A/AA 均为 0 violations。弹窗仅有 Radix 焦点保护与重叠背景导致的自动审计 incomplete，人工确认焦点受限于弹窗且内容可滚动；页面错误为空，控制台仅有开发模式与 HMR 信息。

截图和 dogfood 报告保存在本地 `/tmp/hotkey-011-dogfood/`，不提交为仓库事实源。

## 失败、降级与回滚

- 健康探测只执行 MCP 初始化与 `tools/list`，不发起计费搜索；实际采集才调用唯一的 `web_search`，查询参数固定为 `search_query`。
- 每次请求都发送 `Foundry-Features: Toolboxes=V1Preview`，使用运行时 `env:NAME` Bearer 凭据并接受 SSE/JSON；会话、工具名和工具参数均通过契约校验。
- 每次拨号复验公开 DNS，禁止代理、非 443 端口、用户信息、跨端点重定向、私网地址和超过 4 MiB 响应；敏感查询在请求凭据和访问网络前拒绝。
- 回滚时先停用 `bing_grounding` 来源并等待运行结束，再回退应用；保留来源、采集运行、派生证据和 checkpoint 历史，不影响其他连接器。
