# CLAUDE.local.md — hotkey-server 局部项目规范

本文件用于记录 hotkey-server 项目中的局部规范性配置。

## 使用边界

1. `CLAUDE.md` 存放长期稳定的全局规则、角色协作原则和交付格式。
2. `CLAUDE.local.md` 存放当前项目特有的规范、路径、命令、环境约束和临时协作约定。
3. 当两者冲突时，以更具体、更贴近当前项目的规则为准。

## 当前项目规范

### 职责

- Go 后端（Gin + GORM）：账号、热点监控、内容采集、话题聚合、趋势、通知、Obsidian 日报。

### 常用命令

```bash
make test              # 运行测试
make lint              # 静态检查
make build             # 构建 ./hotkey-server
make dev               # 本地开发（go run ./cmd/hotkey）
make up                # Docker Compose 全栈
```

### 工程主线

- HTTP：`internal/platform/http`（Gin）
- 持久化：`internal/database`（GORM）
- 入口：`cmd/hotkey`（API + Worker 单进程）

### 角色与流程

- 角色配置：`.claude/agents/`
- 可复用流程：`.claude/skills/`
- Symphony 调度：`WORKFLOW.md`

### 跨仓顺序

接口、数据模型或 Swagger 契约变更默认顺序：`hotkey-server` -> `hotkey-web` -> `hotkey-miniapp` -> 回归。
