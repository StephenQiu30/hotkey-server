# 认证状态响应与完整链路验收

**日期：** 2026-07-12

## 验收结论

统一响应契约已固定为 `code + error_code + data`。响应体不包含 `message` 或 `request_id`；请求追踪 ID 仅存在于 `X-Request-Id` 响应头和日志。

## 关键缺陷修复

1. 移除无验证码 Ticket 的直接注册路径。
2. 认证 ErrorCode 由 Server 中央注册表定义并发布到 OpenAPI。
3. Web 使用生成的 ErrorCode 类型集中映射中文提示。
4. 注册成功直接接管 Server 返回的 Session，不再重复登录。
5. 修复用户事务未提交时创建 Session 导致的外键失败。
6. 修复 Session Access Token 缺少用户 Subject、受保护写操作得到用户 `0` 的问题。

## 自动化验证

Server：

```text
make openapi-validate                       PASS
go test -race ./...                        PASS
go vet ./...                               PASS
go build ./cmd/hotkey                      PASS
bash scripts/validate-architecture-boundaries.sh PASS
```

使用本机 PostgreSQL 与 Redis 执行：

```text
TestIntegrationSmoke                       PASS
TestIntegrationRegisterRejectsLegacyPayload PASS
```

真实依赖链路覆盖：发送验证码、确认验证码、Ticket 注册、创建 Session、密码登录、Access Token 鉴权、受保护查询与写入。

Web：

```text
npm run openapi:generate                   PASS
npm run typecheck                          PASS
npm run test:unit                          PASS (18 tests)
npm run build                              PASS
```

Miniapp：

```text
python3 -m unittest discover -s tests       PASS (10 tests)
npm run typecheck                          PASS
npm run build:h5                           PASS（仅包体积建议警告）
```

## 浏览器与运行时证据

- `/login` 与 `/register` 均正常渲染，无 Next.js 错误覆盖层。
- 错误邮箱登录在页面展示“邮箱或密码错误”。
- 同一请求的 API 响应为：

```json
{"code":401,"error_code":"AUTH_INVALID_CREDENTIALS","data":null}
```

- 响应头保留 `X-Request-Id`，响应体没有 `message` 和 `request_id`。

## 已知边界

浏览器验收未向真实 163 邮箱发送邮件，避免产生外部邮件副作用；验证码发送、确认与 Ticket 注册通过真实 PostgreSQL/Redis和确定性测试 Mailer 完成。SMTP 模板与传输由独立单元测试覆盖。
