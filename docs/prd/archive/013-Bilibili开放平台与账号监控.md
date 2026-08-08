---
layer: PRD
scope: shared
doc_no: "013"
title: Bilibili开放平台与授权账号监控
status: implemented
version: v1.1
owner: HotKey Team
phase: P1
canonical_path: docs/prd/archive/013-Bilibili开放平台与账号监控.md
design: docs/design/archive/013-Bilibili开放平台与账号监控设计.md
plan: docs/plans/archive/013-Bilibili开放平台与账号监控计划.md
---

# Bilibili开放平台与授权账号监控

## 用户问题

管理员无法通过官方、可撤销和可审计的方式接入一个已经授权的 Bilibili 创作者账号，监控其视频与专栏更新。

## 产品目标

交付一个最小但完整的 Bilibili 授权来源：管理员配置 OpenID 与环境凭据引用、探测授权、启用轮询；系统可验证撤销 Webhook 并立即停用来源。

## 用户故事

- 作为管理员，我能看到官方能力限制，配置授权 OpenID 和 `env:NAME`，且系统不回显密钥。
- 作为管理员，我只有在授权身份和 scope 健康时才能启用来源。
- 作为编辑者，我能把已启用来源加入监控，让视频和专栏进入既有采集流水线。
- 作为审计者，我能确认重复轮询/重复 Webhook 幂等，并追溯授权撤销造成的停用。
- 作为 Viewer，我只能查看公开状态，不能操作来源或读取管理配置。

## 功能要求

- FR-013-1：新增 `bilibili` 来源，固定官方端点与 OAuth2，创建时强制 disabled。
- FR-013-2：来源配置必须包含合法 `bilibili_open_id`；凭据仅接受 `env:NAME` 指向的 `client_id/app_secret/access_token` JSON。
- FR-013-3：健康探测必须调用官方 scopes 接口，校验 OpenID、`USER_INFO` 及至少一个内容 scope。
- FR-013-4：按官方分页接口采集已授权账号的视频与专栏，游标可恢复，内容映射与 URL 严格校验。
- FR-013-5：Webhook 必须按原始请求体验签，支持 `verify_webhooks` 和 `deauthorize`，重复投递幂等。
- FR-013-6：撤销授权后停用对应来源并保留最小回执/审计，后续采集被拒绝。
- FR-013-7：前端只用既有 shadcn/ui 组件展示配置、官方限制与健康门禁。

## 非功能要求

- 官方 API 只允许 TLS 1.2+、固定 DNS 主机、443 端口、受限重定向、超时、限流和 4 MiB 响应上限。
- Webhook 请求体上限 256 KiB，使用常量时间比较；数据库不保存原始载荷与密钥。
- 错误只返回稳定安全码，不回显 Token、secret、OpenID 或第三方响应正文。
- 采集与回调可追踪、可重试、幂等，不建立旁路内容表。

## 非目标

- 任意公共 UID、`@账号`、主页 URL 的搜索或监控。
- 非官方网页抓取、私有接口、浏览器 Cookie、验证码处理。
- 直播长连接、公共关键词搜索、内容发布 Webhook、OAuth 授权页托管和 Token 自动刷新。

## 验收标准

- AC-013-1：未授权、OpenID 不一致或 scope 不足时不能读取内容或启用来源。
- AC-013-2：视频/专栏分页与游标恢复正确，重复轮询只生成一条 Content。
- AC-013-3：Webhook challenge、签名拒绝和重复投递行为正确。
- AC-013-4：撤销事件停用来源并产生无敏感载荷的回执与审计。
- AC-013-5：管理员/Viewer 权限、响应脱敏、桌面/移动布局和可访问性验收全部通过。
