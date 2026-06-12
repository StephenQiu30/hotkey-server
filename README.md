# hotkey-server

HotKey 后端服务：API、数据库、采集、AI 摘要、榜单、通知、搜索与 OpenAPI 契约事实源。

## Agent 规范

- `CLAUDE.md` — Claude 协作规范
- `CLAUDE.local.md` — 本项目局部配置
- `WORKFLOW.md` — Symphony / Linear 调度契约
- `.claude/agents/` — 角色定义
- `.claude/skills/` — 可复用工作流

## 验证

```bash
bash scripts/validate-repository.sh
```
