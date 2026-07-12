# API Response Contract

## ADDED Requirements

### Requirement: 数字状态码响应

所有 JSON API 响应体的 `code` MUST 为数字，并与实际 HTTP status 相同。

#### Scenario: 成功响应

- **WHEN** 接口返回 HTTP 200
- **THEN** 响应体为 `{"code":200,"message":"success","data":...}`

#### Scenario: 创建成功响应

- **WHEN** 接口返回 HTTP 201
- **THEN** 响应体中的 `code` 为 `201`

### Requirement: 响应体不暴露请求追踪 ID

所有业务 JSON 响应体 MUST NOT 包含 `request_id`；链路追踪 ID MUST 仅通过 `X-Request-Id` 响应头和日志上下文提供。

### Requirement: 验证后注册

Web 注册请求 MUST 提交验证码确认接口返回的 `verification_ticket`，不得在已验证流程中回退到直接邮箱注册。
