---
layer: Design
scope: shared
doc_no: "013"
title: Bilibili开放平台与授权账号监控设计
status: accepted
version: v1.1
owner: HotKey Team
phase: P1
canonical_path: docs/design/archive/013-Bilibili开放平台与账号监控设计.md
prd: docs/prd/archive/013-Bilibili开放平台与账号监控.md
plan: docs/plans/archive/013-Bilibili开放平台与账号监控计划.md
---

# Bilibili开放平台与授权账号监控设计

## 问题与目标

HotKey 尚不能以可审计的方式监控 Bilibili 内容。本设计只接入用户明确授权给 HotKey 所属开放平台应用的账号，采集其视频稿件与专栏；授权范围、身份和撤销状态均由官方接口确认。

## 官方能力边界

- 开放平台 OAuth 返回 `access_token`、`refresh_token`、过期时间和 scopes；HotKey 运行时只解析 `env:NAME` 指向的 JSON 凭据包，不持久化或回显密钥。
- `GET /arcopen/fn/user/account/scopes` 是授权身份事实源；配置中的 `bilibili_open_id` 必须与其返回值一致。
- 视频稿件使用官方 `GET /arcopen/fn/archive/viewlist` 轮询，专栏使用官方 `GET /arcopen/fn/article/list` 轮询；分页游标由 HotKey 保存，内容依靠既有唯一键幂等入库。
- Webhook 使用 `x-bilibili-signature = SHA1(app_secret + 原始请求体)` 验签。`verify_webhooks` 原样回显 challenge，`deauthorize` 触发来源停用并记录幂等回执与审计。
- 官方开放平台未提供任意公共账号的 UID、`@账号` 或主页 URL 检索契约，也未提供视频/专栏发布 Webhook。MVP 不解析任意公共账号，不抓网页，不调用私有接口；内容 Webhook、直播长连接和公共关键词搜索均不在本条范围。

## 核心设计

1. 新增固定来源类型 `bilibili`，固定端点 `https://member.bilibili.com/arcopen/fn`，认证类型必须为 `oauth2`。
2. 新建来源默认停用；只有健康探测确认 OpenID 一致且至少具备 `USER_INFO` 与 `ARC_BASE`/`ATC_BASE` 之一后才允许启用。
3. `bilibili_open_id` 是授权结果的不可猜测标识，不是用户输入的公共 UID。前端明确提示先完成开放平台授权，再从授权结果复制 OpenID。
4. 请求采用开放平台 v2 签名：固定主机、请求体 MD5、十分钟时间戳、唯一 nonce 与 HMAC-SHA256；禁止跟随到非官方主机。
5. 凭据包最小结构为 `client_id`、`app_secret`、`access_token`。日志、错误、API 和证据均不得包含这些字段的值。
6. 视频映射为 `video:<BV号>`，专栏映射为 `article:<id>`；未知、越权、非法 URL 或畸形时间字段被拒绝并输出安全诊断。
7. Webhook 回执以原始请求体摘要唯一，重复投递不重复改变状态。撤销后来源立即停用、健康状态置为 unavailable，后续调度无法采集。

## 数据与交互

- `source_connections.config.bilibili_open_id` 保存已授权 OpenID；合规配置强制正文归档、来源署名和删除同步。
- `bilibili_webhook_receipts` 只保存事件摘要、事件类型、OpenID、处理状态和时间，不保存原始请求体或 Token。
- 来源弹窗使用既有 shadcn/ui 的 Select、Input、Alert、Checkbox 和 Button 组合；不新增自绘控件。
- Viewer 只能查看来源类型、启停和健康状态，不能看到 OpenID、端点、凭据引用或诊断细节。

## 安全与失败

- DNS、每次连接和重定向均复验；只允许 `member.bilibili.com:443`，TLS 最低 1.2，限制响应体、超时和每轮页数。
- 401/403 或 scope/OpenID 不匹配属于认证错误；429 属于限流错误；5xx/网络超时属于临时错误；畸形响应属于解析错误。
- Webhook 缺少签名、签名不匹配、未知事件、过大请求体或未知 OpenID 均拒绝且不改变来源。
- 授权撤销前已经合法采集的历史证据按既有留存策略保留；后续采集停止。

## 验收边界

- AC-013-1：未配置授权、OpenID 不一致或 scope 不足时健康探测失败，来源不能启用且不读取内容。
- AC-013-2：视频与专栏轮询分页可恢复，重复轮询只生成一条 Content。
- AC-013-3：Webhook 验签与 challenge 回显正确，重复事件幂等。
- AC-013-4：收到 `deauthorize` 后来源停用、不可继续采集，并保留不含原始载荷的回执与审计。
- AC-013-5：管理端可完成 Bilibili 来源配置和健康探测；Viewer 无写入口；桌面与移动端无横向溢出且关键流程无可访问性违规。

## 官方依据

- [Bilibili 开放平台文档](https://openhome.bilibili.com/doc)
- [OAuth 与 Token](https://openhome.bilibili.com/doc/4/eaf0e2b5-bde9-b9a0-9be1-019bb455701c)
- [开放平台 v2 请求签名](https://open.bilibili.com/doc/4/8673959e-f7bb-56e6-6e68-d225f971b81b)
- [授权 scopes](https://openhome.bilibili.com/doc/4/08f935c5-29f1-e646-85a3-0b11c2830558)
- [视频稿件列表](https://openhome.bilibili.com/doc/4/a24030b7-6b8f-b36c-32d8-a4aae67fcc35)
- [专栏列表](https://openhome.bilibili.com/doc/4/c8057666-2b92-fc72-3607-f4199933dc13)
- [Webhook 事件订阅](https://openhome.bilibili.com/doc/4/b369a652-0e26-8ddb-74f0-20c74234fcd6)
- [隐私政策](https://openhome.bilibili.com/agreement/privacy-policy)
