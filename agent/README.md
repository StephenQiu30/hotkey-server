# HotKey Python Agent

`agent/` 是 HotKey 唯一获批的 Python 运行面，专门处理有界数据分析。Go Core 仍是唯一业务 API、权限与事实拥有者；Agent 不连接 PostgreSQL、Redis、MinIO、Vault 或来源系统，也不直接提交业务事实。

当前 `deterministic` Runtime 用于验证跨语言契约、部署、降级和安全边界，不代表真实模型质量已经验收。真实模型只能按 `docs/plans/003-智能研判事件热度与人工治理计划.md` 的 Golden、Shadow、Live 与回滚门禁接入。

本地验证：

```bash
uv sync --all-extras --locked
uv run ruff format --check .
uv run ruff check .
uv run mypy src
uv run pytest
uv run pip-audit
```
