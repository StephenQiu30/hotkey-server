# 跨仓客户端生成指南

> 契约事实源：`hotkey-server/docs/openapi.json`

## 流程

```
hotkey-server (OpenAPI export) ──> hotkey-web (@umijs/openapi) ──> hotkey-miniapp
```

OpenAPI 变更必须先在 `hotkey-server` 合并，再通知下游仓库重新生成客户端。

未完成以下 server 门禁前，不允许进入下游同步：

1. `make openapi`
2. `make openapi-validate`
3. `bash scripts/validate-repository.sh`

## 1. 导出 OpenAPI Spec

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server

# 重新生成 docs/openapi.json
make openapi

# 验证 spec 完整性
make openapi-validate

# 运行仓内总体验证，确保 server 已达到可同步状态
bash scripts/validate-repository.sh
```

生成器通过 `make openapi` 输出 spec。

## 2. hotkey-web 客户端生成

在 `hotkey-web` 仓库中使用仓内 `openapi2ts.config.ts`：

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-web

# 基于 ../hotkey-server/docs/openapi.json 生成客户端
npm run openapi:generate

# 继续执行 Web 端同步门禁
npm run test
npm run typecheck
npm run build
bash scripts/validate-repository.sh
```

生成产物包含：

- `src/services/hotkey/hotkey-server/*.ts`
- `src/services/hotkey/hotkey-server/typings.d.ts`
- 与 `@/lib/request` 对接的请求入口

## 3. hotkey-miniapp 客户端生成

在 `hotkey-miniapp` 仓库中使用仓内 `openapi2ts.config.ts`：

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-miniapp

# 基于 ../hotkey-server/docs/openapi.json 生成客户端
npm run openapi:generate

# 继续执行小程序端同步门禁
npm run test
npm run typecheck
npm run build:weapp
bash scripts/validate-repository.sh
```

生成产物包含：

- `src/services/hotkey/hotkey-server/*.ts`
- `src/services/hotkey/hotkey-server/typings.d.ts`
- 与 `@/utils/request` 对接的请求入口

## 4. 验证

每次生成后按仓执行验证：

```bash
# hotkey-web
npm run test
npm run typecheck
npm run build

# hotkey-miniapp
npm run test
npm run typecheck
npm run build:weapp
```

## 5. 契约变更检查清单

1. `hotkey-server`：修改路由/DTO → `make openapi` → `make openapi-validate` → `bash scripts/validate-repository.sh` 全绿
2. `hotkey-server`：PR 合并到 main
3. `hotkey-web`：拉取最新 `docs/openapi.json` → `npm run openapi:generate` → `npm run test` → `npm run typecheck` → `npm run build`
4. `hotkey-miniapp`：拉取最新 `docs/openapi.json` → `npm run openapi:generate` → `npm run test` → `npm run typecheck` → `npm run build:weapp`
5. 对 Web 受影响主链路使用 `vercel:agent-browser` 回归
6. 若 miniapp 有浏览器可承载入口，执行等价回归；否则记录专属小程序验证路径
