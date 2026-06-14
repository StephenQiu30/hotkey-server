# 跨仓客户端生成指南

> 契约事实源：`hotkey-server/docs/openapi.json`

## 流程

```
hotkey-server (OpenAPI export) ──> hotkey-web (@umijs/openapi) ──> hotkey-miniapp
```

OpenAPI 变更必须先在 `hotkey-server` 合并，再通知下游仓库重新生成客户端。

## 1. 导出 OpenAPI Spec

```bash
# 重新生成 docs/openapi.json
make openapi

# 验证 spec 完整性
make openapi-validate
```

生成器通过 `make openapi` 输出 spec。

## 2. hotkey-web 客户端生成

在 `hotkey-web` 仓库中使用 `@umijs/openapi`：

```bash
# 安装依赖（如未安装）
npm install -D @umijs/openapi

# 从 hotkey-server 导入 spec 并生成客户端
npx openapi --input <path-to>/hotkey-server/docs/openapi.json --output ./src/api --name hotkey
```

生成产物包含：
- TypeScript 类型定义（请求/响应 DTO）
- 带类型的 API 调用函数
- 请求/响应 schema 校验

## 3. hotkey-miniapp 客户端生成

miniapp 仓库使用相同流程，按需调整输出路径。

## 4. 验证

每次生成后执行 smoke 测试确认客户端可用：

```bash
# hotkey-web / hotkey-miniapp
npm run build   # 类型检查通过即为 smoke 通过
```

## 5. 契约变更检查清单

1. `hotkey-server`：修改路由/DTO → `make openapi` → `make openapi-validate` 全绿
2. `hotkey-server`：PR 合并到 main
3. `hotkey-web`：拉取最新 `docs/openapi.json` → 重新生成客户端 → `npm run build`
4. `hotkey-miniapp`：同步步骤 3
5. 回归测试通过
