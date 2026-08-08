---
layer: Acceptance
scope: shared
doc_no: "013"
title: Bilibili开放平台与授权账号监控验收
status: passed
version: v1.0
owner: HotKey Team
phase: P1
canonical_path: docs/acceptance/013-Bilibili开放平台与账号监控验收.md
design: docs/design/archive/013-Bilibili开放平台与账号监控设计.md
prd: docs/prd/archive/013-Bilibili开放平台与账号监控.md
plan: docs/plans/archive/013-Bilibili开放平台与账号监控计划.md
---

# Bilibili开放平台与授权账号监控验收

## 验收结论

013 于 2026-08-07 通过验收。系统只监控明确授权给 Bilibili 开放平台应用的创作者账号，使用官方 scopes、视频稿件和专栏接口轮询；任意公共 UID、`@账号`、主页 URL、网页抓取和私有接口均不在实现中。来源创建后默认停用，只有 OpenID 与 scopes 健康探测通过才能启用；撤销授权会通过签名 Webhook 幂等停用来源。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-013-1 | passed | 连接器先调用官方 scopes 接口，严格比对授权 OpenID，并要求 `USER_INFO` 与 `ARC_BASE`/`ATC_BASE`；缺少凭据、身份不一致或 scope 不足均不能读取内容或启用。 |
| AC-013-2 | passed | 视频与专栏 fixture 覆盖分页游标、字段映射和稳定外部 ID；既有内容唯一约束与采集幂等测试保证重复轮询不重复入库。 |
| AC-013-3 | passed | 传输与领域测试覆盖原始请求体 SHA1 验签、`verify_webhooks` challenge、错误签名、未知事件、过大请求和重复撤销投递。challenge 无状态响应，撤销回执以请求体摘要幂等。 |
| AC-013-4 | passed | 数据库集成测试确认首次撤销会停用匹配 OpenID 的来源、置为 unavailable 并写入系统审计；重复摘要不再次更新版本。回执只保存摘要、事件类型、OpenID、状态和时间，不保存原始请求体。 |
| AC-013-5 | passed | 管理员可配置固定官方来源，Viewer 仅见公开状态；OpenID、端点和凭据引用对 Viewer 隐藏。桌面/移动无横向溢出，浏览器流程 axe 为 0 violations。 |

## 自动化验证

```bash
cd backend
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:55433/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:56380/15' make ci

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

后端全量 CI 使用 `pgvector/pgvector:pg18`，通过 OpenAPI 再生成一致性、vet、PostgreSQL 升降级、68 表 Schema 指纹、全量测试、构建、架构和仓库门禁。前端 OpenAPI 契约、类型检查、37 个测试文件共 140 项单元测试及 Next.js 生产构建全部通过。Compose 开发与生产配置均可解析，diff 检查通过。

## 浏览器验收

使用用户指定的 `$agent-browser` 连接真实 Next.js、Go、PostgreSQL、Redis 与 MinIO：

- 未认证访问来源页会跳转到带安全 redirect 的登录页。
- 管理员新增来源时可选“Bilibili 开放平台”；端点固定为官方地址，授权方式固定为 OAuth 2.0，必须填写授权 OpenID 和 `env:NAME`，正文保存、归属标记和删除同步均被强制启用。
- 页面明确说明只支持已授权账号，不解析公共 UID、`@账号` 或主页地址。完整表单可创建停用来源；服务端环境变量缺失时，探测和启用均安全失败，来源保持停用。
- Viewer 能看到 Bilibili 来源名称与公开状态，但看不到新增、探测或启用操作，也看不到 OpenID、官方端点或凭据引用。
- 1440×1000 与 390×844 下页面和表单均无横向溢出。axe-core 4.10.3 自动扫描为 0 violations；2 个 incomplete 项来自 Radix 模态背景的 `aria-hidden-focus` 与需人工确认的文本对比度，目视和键盘路径复核通过。
- 页面错误为空，控制台只有 React 开发提示与 HMR 连接信息。

截图、快照和 dogfood 证据保存在本地 `/tmp/hotkey-013-dogfood/`，不提交为仓库事实源。

## 官方依据与已知限制

- [Bilibili 开放平台文档](https://openhome.bilibili.com/doc)是能力总入口。
- [OAuth 与 Token](https://openhome.bilibili.com/doc/4/eaf0e2b5-bde9-b9a0-9be1-019bb455701c)、[授权 scopes](https://openhome.bilibili.com/doc/4/08f935c5-29f1-e646-85a3-0b11c2830558)定义授权身份和权限。
- [开放平台 v2 请求签名](https://open.bilibili.com/doc/4/8673959e-f7bb-56e6-6e68-d225f971b81b)、[视频稿件列表](https://openhome.bilibili.com/doc/4/a24030b7-6b8f-b36c-32d8-a4aae67fcc35)、[专栏列表](https://openhome.bilibili.com/doc/4/c8057666-2b92-fc72-3607-f4199933dc13)构成采集事实源。
- [Webhook 事件订阅](https://openhome.bilibili.com/doc/4/b369a652-0e26-8ddb-74f0-20c74234fcd6)用于验证回调和授权撤销；它不提供视频或专栏发布事件，因此内容更新仍需轮询。
- MVP 不托管 OAuth 授权页，不自动刷新 Token，不接直播长连接，也不支持公共关键词或任意账号搜索。上线前必须由运营方在官方开放平台完成应用、回调地址、授权和凭据配置。

## 回滚

回滚时先停止 Bilibili 来源调度并移除 Webhook 路由，再回退应用。新来源默认停用；历史 Content、CollectionRun、回执和审计继续保留。Schema 为可重复执行的增量定义，不通过删除历史事实回滚。
