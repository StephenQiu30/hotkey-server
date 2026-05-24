# Scripts

该目录用于放置非业务型辅助脚本，例如：

- 初始化本地开发环境
- 校验 OpenAPI 契约
- 导出结构化调试数据
- 执行门禁与工作区清洁检查（例如 `scripts/checkpoint-gate.sh`）

约束：

- 脚本不应绕过核心业务层直接写入生产业务数据
- 业务逻辑仍应沉淀在 `server/app/` 中

## 常用执行命令

- `bash scripts/checkpoint-gate.sh`  
  执行门禁检查：扫描 `apps/` 运行时残留、运行仓库治理测试，并检查 `git status --short` 是否为空。
- `PYTHONPATH=. python3 scripts/verify_ai_provider.py --json`  
  执行 #50-#53/#55 真实 AI provider 验收门禁：无凭据时返回 `missing_credentials`
  和所需 env；配置真实 provider 后触发一次 `analyze_hotspot`，校验 `trace_id`、
  `ai_orchestrator_decision` 与 `provider_trace`。脚本只输出密钥是否缺失，不输出密钥内容。

### 质量门禁约定（按轮执行）

- 每个 Issue/PR 执行前后必须保持 `git status --short` 清洁，避免中间产物留在工作区。
- 提交顺序要求：先补/跑测试（红绿循环），再提交代码（`feat`/`fix`/`test` 前缀）并再次通过门禁。
