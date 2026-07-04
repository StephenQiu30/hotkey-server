# 跨仓 Swagger 客户端生成

`hotkey-server` 的 `docs/swagger.json` 是 **hotkey-web** 与 **hotkey-miniapp** 的契约事实源。

## 工程主线（当前）

- HTTP：Gin（`internal/platform/http`）
- 契约来源：Gin handler Swagger 注释自动生成 `docs/swagger.json`
- 入口：`cmd/hotkey`（API + Worker 单进程）

## Server 侧：校验与暴露

```bash
cd hotkey-server
make swagger
make swagger-validate
bash scripts/validate-repository.sh
```

运行中的服务会直接暴露 Swagger 页面与文档：

- `/swagger/index.html`
- `/swagger/doc.json`

## 下游同步顺序

固定为：

```text
hotkey-server → hotkey-web → hotkey-miniapp → 回归
```

1. 在 **hotkey-server** 稳定契约并合并。
2. 在 **hotkey-web** / **hotkey-miniapp** 用客户端生成工具从 `docs/swagger.json` 重新生成客户端。
3. 不得手写漂移的后端 API 类型。

## Web 生成（hotkey-web）

以该仓 `package.json` / OpenAPI 配置为准，典型流程：

```bash
cd hotkey-web
# 从 ../hotkey-server/docs/swagger.json 生成客户端（具体脚本见 web 仓 README）
npm run openapi
npm run typecheck
npm run test
```

## 小程序生成（hotkey-miniapp）

```bash
cd hotkey-miniapp
# 从 ../hotkey-server/docs/swagger.json 生成客户端（具体脚本见 miniapp 仓 README）
npm run openapi
npm run typecheck
npm run test
```

## 错误响应契约

所有 API 错误体统一为：

```json
{ "error": "human readable message", "code": "optional_machine_code" }
```

Web 与小程序请求层必须消费 `{ error, code }`，不得仅依赖 HTTP 状态码文案。

## 何时禁止进入下游

以下情况 **不得** 在 web/miniapp 开始同步：

1. 未运行 `make swagger` / `make swagger-validate`
2. `docs/swagger.json` 未更新或与路由不一致
3. `bash scripts/validate-repository.sh` 未通过
