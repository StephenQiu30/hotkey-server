# CLAUDE.local.md — hotkey-server 局部项目规范

本文件用于记录 hotkey-server 项目中的局部规范性配置。

## 使用边界

1. `CLAUDE.md` 存放长期稳定的全局规则、角色协作原则和交付格式。
2. `CLAUDE.local.md` 存放当前项目特有的规范、路径、命令、环境约束和临时协作约定。
3. 当两者冲突时，以更具体、更贴近当前项目的规则为准。

## 当前项目规范

### 职责

- FastAPI 后端：账号、热点、榜单、AI 摘要、选题生成、收藏关注、通知、搜索、数据源采集。
- OpenAPI/Swagger 是跨仓契约事实源；`hotkey-web` 与 `hotkey-miniapp` 均从本仓生成客户端。

### 常用命令

```bash
make test              # 运行测试
make lint              # 静态检查
make build             # 构建
uvicorn app.main:app --reload   # 本地开发（以实际入口为准）
```

### OpenAPI 输出

- 规范路径：`docs/openapi.json`（或项目实际导出路径）
- 契约变更必须先在本仓稳定并合并，再通知 web/miniapp 重新生成客户端

### 角色与流程

- 角色配置：`.claude/agents/`
- 可复用流程：`.claude/skills/`
- Symphony 调度：`WORKFLOW.md`

### 跨仓顺序

接口、数据模型或 OpenAPI 变更默认顺序：`hotkey-server` -> `hotkey-web` -> `hotkey-miniapp` -> 回归。
