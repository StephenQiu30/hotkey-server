# HotKey Python Agent

`agent/` 是 HotKey 唯一获批的 Python 运行面，专门处理有界数据分析。Go Core 仍是唯一业务 API、权限与事实拥有者；Agent 不连接 PostgreSQL、Redis、MinIO、Vault 或来源系统，也不直接提交业务事实。

Agent 的唯一真实模型运行时是本机 [Codex App Server](https://developers.openai.com/codex/app-server)。它通过 WebSocket JSON-RPC 执行 `initialize`、临时 `thread/start` 与 `turn/start`，不调用 Ollama、OpenAI-compatible HTTP 服务，也不需要模型 API Key、模型地址或模型名配置。`deterministic.v1` 只保留为无模型降级与契约测试实现，不是可配置的 AI 出口。

每个分析请求都使用临时线程、`never` 审批、只读沙箱、关闭网络和工具，并通过 `outputSchema` 约束最终 JSON。协议请求、工具项、模型错误、超时、Token 计量缺失或结构不合法时统一 fail closed，API 只返回稳定脱敏错误。实际模型版本与 input/output Token 用量由 App Server 响应回传。

先在宿主机启动 App Server；它只监听本机回环地址，Docker 内的 Agent 通过 `host.docker.internal` 访问：

```bash
codex app-server --listen ws://127.0.0.1:4500
```

然后从仓库根目录启动完整项目：

```bash
docker compose -f docker-compose.yml up --build --detach --wait
```

日常本机验证直接复用已安装的 Python 工具链，不重复启动项目：

```bash
uv sync --all-extras --locked
uv run ruff format --check .
uv run ruff check .
uv run mypy src tests
uv run pytest
uv run pip-audit
```

当 Go Core 同时配置 `HOTKEY_AGENT_SHADOW_ENABLED=true`、`HOTKEY_AGENT_URL` 与 `HOTKEY_AGENT_AUTH_TOKEN` 时，当前接线只执行异步 Shadow 比较：旧 Provider 的已验证结果仍是主 Run 和业务写入来源；Agent 结果、超时、失败或并发丢弃都不能覆盖主结果、消耗主预算或触发主重试。Agent 尚未进入 `ProviderName`、Model Profile 或 Live Registry；关闭 Shadow 开关即可停止新调用。

受信环境可以从 `backend/` 执行固定 Golden 双轨评估。Agent URL/Token 只通过环境配置注入，不能写进命令行或数据集；报告固定为不可激活的 `candidate`，真实质量、双人人工复核、灰度与回滚证据完成前不得进入 Live：

```bash
go run ./cmd/hotkey agent-quality evaluate \
  --dataset test/fixtures/agent-shadow/v1/golden-dataset.json \
  --baseline-runtime openai \
  --baseline-model-name '<固定模型名>' \
  --baseline-model-version '<固定模型版本>' \
  --agent-model-name hotkey-agent \
  --agent-model-version '<App Server 实际模型版本>' \
  --timeout 60s
```
