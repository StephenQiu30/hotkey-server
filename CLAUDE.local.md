# CLAUDE.local.md — hotkey-server 局部项目规范

本文件用于记录放在 hotkey-server 项目中的局部规范性配置。

## 使用边界

1. `CLAUDE.md` 应存放长期稳定的全局规则、角色协作原则和交付格式
2. `CLAUDE.local.md` 则负责**当前项目特有的规范、路径、命令、环境约束和临时协作约定**
3. 当两者冲突时，**应优先确认项目上下文，并以更具体、更贴近当前项目的规则为准**

## 当前项目规范

### 技术栈
- Go 1.25 + Gin 框架
- PostgreSQL + pgvector
- Redis
- 阿里云 DashScope（Qwen 模型、text-embedding-v2）

### 项目结构
```
hotkey-server/
├── cmd/server/          # 主入口
├── internal/            # 内部包
│   ├── httpapi/         # HTTP API
│   ├── config/          # 配置
│   ├── openapi/         # OpenAPI 规范
│   ├── keyword/         # 关键词管理
│   ├── source/          # 来源采集
│   ├── content/         # 内容标准化
│   ├── event/           # 事件处理
│   ├── eventgraph/      # 事件图谱
│   ├── hotspot/         # 热点排名
│   ├── report/          # 日报生成
│   ├── trust/           # 信任评估
│   ├── propagation/     # 传播分析
│   ├── realtime/        # 实时处理
│   ├── redisinfra/      # Redis 基础设施
│   ├── adminapi/        # 管理 API
│   ├── tenant/          # 租户管理
│   ├── rbac/            # 权限控制
│   ├── billing/         # 计费管理
│   └── workqueue/       # 工作队列
├── db/                  # 数据库
│   └── schema.sql       # 数据库模式
├── n8n/                 # n8n 工作流
└── .env.example         # 环境变量示例
```

### 常用命令
```bash
go run ./cmd/server                          # 启动服务器
HOTKEY_HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server  # 自定义地址
go test ./...                                # 运行所有测试
go test ./internal/hotspot/...               # 运行单个包测试
curl http://127.0.0.1:18080/healthz          # 健康检查
curl http://127.0.0.1:18080/openapi.json     # 导出 OpenAPI 规范
```

### 角色配置
角色配置存放于 `.claude/agents/` 目录

### 可复用流程
可复用流程存放于 `.claude/skills/` 目录
