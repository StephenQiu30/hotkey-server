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
├── queue/          Kafka 消息队列（生产者/消费者/调度器/去重）
├── worker/         后台任务（Obsidian 日报发布）
├── fxapp/          Fx 应用组装与生命周期
├── config/         配置加载
├── platform/http   Gin HTTP API
├── auth/           账号
├── monitor/        热点监控
├── content/        内容检索
├── topic/          主题聚类
├── trend/          趋势分析
├── hotevent/       热点事件
├── notify/         通知推送
├── llm/            LLM 内容聚合
├── repository/     数据仓库实现
└── database/       数据库连接与查询
db/                 表结构与迁移
tests/              测试
```

## Agent 规范

- `CLAUDE.md` — Claude 协作规范
- `CLAUDE.local.md` — 本项目配置
- `.claude/agents/` — 角色定义
- `.claude/skills/` — 可复用工作流
