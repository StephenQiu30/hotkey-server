# Scripts

该目录用于放置非业务型辅助脚本，例如：

- 初始化本地开发环境
- 校验 OpenAPI 契约
- 导出结构化调试数据
- 执行门禁与工作区清洁检查（例如 `scripts/checkpoint-gate.sh`）

约束：

- 脚本不应绕过核心业务层直接写入生产业务数据
- 业务逻辑仍应沉淀在 `backend/core/` 中

## 常用执行命令

- `bash scripts/checkpoint-gate.sh`  
  执行门禁检查：扫描 `apps/` 运行时残留、运行仓库治理测试，并检查 `git status --short`。
