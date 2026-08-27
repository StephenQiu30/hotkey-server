# HotKey Python Agent

`agent/` 是 HotKey 唯一获批的 Python 运行面，专门处理有界数据分析。Go Core 仍是唯一业务 API、权限与事实拥有者；Agent 不连接 PostgreSQL、Redis、MinIO、Vault 或来源系统，也不直接提交业务事实。

当前 `deterministic` Runtime 用于验证跨语言契约、部署、降级和安全边界，不代表真实模型质量已经验收。真实模型只能按 `docs/plans/003-智能研判事件热度与人工治理计划.md` 的 Golden、Shadow、Live 与回滚门禁接入。

当 Go Core 同时配置 `HOTKEY_AGENT_SHADOW_ENABLED=true`、`HOTKEY_AGENT_URL` 与 `HOTKEY_AGENT_AUTH_TOKEN` 时，当前接线只执行异步 Shadow 比较：旧 Provider 的已验证结果仍是主 Run 和业务写入来源；Agent 结果、超时、失败或并发丢弃都不能覆盖主结果、消耗主预算或触发主重试。Agent 尚未进入 `ProviderName`、Model Profile 或 Live Registry；将开关改回 `false` 即停止新 Shadow 调用。

受信环境可以从 `backend/` 执行固定 Golden 双轨评估；Agent URL/Token 只通过环境配置注入，不能写进命令行或数据集：

```bash
go run ./cmd/hotkey agent-quality evaluate \
  --dataset test/fixtures/agent-shadow/v1/golden-dataset.json \
  --baseline-runtime openai \
  --baseline-model-name '<固定模型名>' \
  --baseline-model-version '<固定模型版本>' \
  --agent-model-name hotkey-agent \
  --agent-model-version deterministic.v1 \
  --timeout 60s
```

旧路径也可选择 `deepseek`、`ollama`，或使用 `codex` 并显式传入受信任的绝对 `--codex-executable`。如需比较成本，必须同时传入 baseline 的 input/output 每百万 Token 单价。输出只含数据集 Hash、结构/Evidence/人工复核/延迟/Token/成本/失败分类等有界事实，不含 Golden 输入、模型输出或 Provider 错误文本，并固定为不可激活的 `candidate`。

本地验证：

```bash
uv sync --all-extras --locked
uv run ruff format --check .
uv run ruff check .
uv run mypy src
uv run pytest
uv run pip-audit
```
