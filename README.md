# hotkey-server

X 热点监控平台后端服务。自动采集热点内容、聚类主题、分析趋势，并通过邮件与 Obsidian 推送日报。

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.26 |
| HTTP | Gin |
| ORM  | GORM v2 |
| DI   | go.uber.org/fx |
| 数据库 | PostgreSQL 16 + Redis |
| 消息队列 | Kafka |
| 定时任务 | robfig/cron/v3 |
| LLM 聚合 | langchaingo |

## 本地开发

```bash
make test    # 运行测试
make lint    # 静态检查
make build   # 构建
make dev     # 本地开发
make up      # Docker 全栈启动
make ci      # 完整验证
```

## 项目结构

```
cmd/hotkey/         入口
internal/
├── fxapp/          Fx 应用组装与生命周期
├── module/         基础设施 Module（DB / Redis / Config）
├── config/         配置加载（Viper）
├── controller/     Gin HTTP 处理器和路由
├── service/        业务逻辑层（接口 + 实现）
├── repository/     数据仓库（GORM 实现）
├── content/        跨平台内容检索
├── queue/          Kafka 消息队列
├── worker/         后台定时任务
├── model/          数据模型（entity / dto / vo）
├── pkg/            共享工具类型（JSONB / Vector）
└── platform/       基础设施层（http / database / logging）
db/                 表结构与迁移
tests/              测试（unit / integration / testutil）
```

## Agent 规范

- `CLAUDE.md` — Claude 协作规范
- `CLAUDE.local.md` — 本项目配置
- `.claude/agents/` — 角色定义
- `.claude/skills/` — 可复用工作流
