# 修复认证与统一响应契约 — Proposal

**Date:** 2026-07-12

## Summary

修复 Web 注册未提交验证票据、登录错误信息无法正确展示的问题，并将 API 响应统一为数字状态码，移除业务响应体中的 `request_id`。

## Scope

- 响应统一为 `{ code: number, message: string, data: any }`
- `code` 与实际 HTTP status 一致
- `request_id` 仅保留在 `X-Request-Id` 响应头和日志中
- 注册接口只接受已确认的 `verification_ticket`
- Web 同步生成最新 OpenAPI 客户端，Miniapp 同步通用错误响应适配

## Non-goals

- 不移除服务端请求链路追踪
- 不改变 JWT、Refresh Cookie 或验证码安全策略
