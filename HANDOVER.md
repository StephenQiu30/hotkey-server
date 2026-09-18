# HotKey Server 交接

更新日期：2026-09-18。交接范围：仓库结构与用户确定的技术选型。

## 1. 阅读入口

[PROJECT](PROJECT.md) → [AGENTS](AGENTS.md) → [BACKLOG](BACKLOG.md) → [文档台账](docs/README.md) → 当前切片的 Design/PRD/Plan。PROJECT 是本仓库技术选型主文档，BACKLOG 是产品进度的唯一台账。

## 2. 本次完成

- 将误放于 `.codex/PROJECT.md` 的技术文档移至仓库根 `PROJECT.md`，并重写根 `HANDOVER.md`，修正相对引用。
- 建立 `backend/README.md`、`frontend/README.md`，让两个工程目录可随 Git 保存。
- 固定 Web 为 pnpm + Next.js + shadcn/ui + Radix UI + Tailwind CSS + Axios + ESLint + Prettier，补充 React/TypeScript 和已有 OpenAPI 生成链。
- 固定后端为 Python + SQLAlchemy 2 + FastAPI + PostgreSQL + Redis + Kafka；保留 Alembic、Pydantic、MinIO 和质量工具。
- 同步 AGENTS、贡献规范、问题模板、BACKLOG 和受影响的 Design/Plan，清除旧消息栈的现行要求；保留所有需求编号、验收目标及未完成状态。
- Web 归本仓库 `frontend/`；同级 `hotkey-web` 本地重命名为 `hotkey-app`，固定 Flutter，前端归属冲突已关闭。

## 3. 当前状态与边界

本轮起点：本地 `main`，HEAD `a74b3e92`；起点已有未跟踪 `.codex/PROJECT.md`、`HANDOVER.md`，本轮承接处理。当前修改尚未提交/推送；未刷新远端或检查远端 CI。

43 项逐项 PRD 与 45 份 Plan 仍为规划基线；总体设计及 Redis/Kafka 详细执行设计待补齐，产品验收未推进。`backend/`、`frontend/` 目前只有目录说明，没有应用、依赖锁、迁移、Compose 或测试脚本。本次没有安装依赖、启动服务、连接现有数据、实采或运行应用测试。

Kafka 的选型已经确定，但不会自动提供定时调度、任务取消、数据库幂等和重试策略；这些仍由 031/042 实施。Redis 与 Kafka 均需隔离测试，不能用旧 RabbitMQ/Celery 证据替代。

## 4. 下一步

1. 按 001 S01 完成总体设计，登记 server Web 与 Flutter App 的契约边界；不重复询问已确定的技术栈。
2. 在 031/042 中冻结 Kafka topic/分区键/消费组/提交位点、重试/死信/定时任务、Redis 失效策略、客户端及依赖版本。
3. 按 042 S01 初始化 backend/frontend、依赖锁、迁移、根 Compose、OpenAPI 生成和验证入口，再推进受控任务与真实业务切片。
4. Flutter 初始化由 `hotkey-app` 承接；目标平台、Dart 生成器与鉴权传输需在对应切片落实。

## 5. 本轮检查

已通过：两仓 `git diff --check`；61 份变更 Markdown 的 570 个本地链接检查；规划状态、文档编号、验收 checklist 编号/勾选状态保持不变；两仓 HEAD 与重命名前一致。技术残留检查确认旧消息栈只出现在停用/历史说明中。Flutter 源码与锁文件可跟踪、构建缓存与签名材料被忽略。

应用构建、数据库/消息集成、浏览器/设备、部署和来源均未验证。

后续交接更新日期、提交、改动、实际命令与结果、阻塞和下一动作；技术变更同步 PROJECT，产品进度回写 BACKLOG 与对应 Plan。不要将本轮目录/文档完成记为产品功能完成。
