---
layer: Plan
doc_no: "33"
audience:
  - Tech-Lead
  - Dev
  - Ops
feature_area: "area:infra area:devops"
purpose: "提供 Docker Compose 一键部署能力，支持本地开发和线上部署。"
canonical_path: "docs/plans/33-Docker部署能力实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/26-系统端到端可运行与基础设施对接PRD.md
  - docs/superpowers/specs/2026-05-28-system-running-design.md
outputs:
  - Docker 部署实现任务
  - Docker 部署验证证据
triggers:
  - "docs/product/prd/26-系统端到端可运行与基础设施对接PRD.md 变更"
  - "对应 GitHub issue 状态变更"
downstream:
  - docs/acceptance/
---

# 33-Docker部署能力 实现计划

## 1. 目标

为 HotKey 提供 Docker Compose 一键部署能力，包含 PostgreSQL（pgvector）、Redis、Go 后端和 Next.js 前端四个服务。

## 2. 文件清单

- PRD：`docs/product/prd/26-系统端到端可运行与基础设施对接PRD.md`
- Plan：`docs/plans/33-Docker部署能力实现计划.md`
- 根目录：`docker-compose.yml`
- 后端：`hotkey-server/Dockerfile`
- 前端：`hotkey-web/Dockerfile`
- Makefile：根目录 `Makefile`

## 3. 任务拆解

**Task 33-1：后端 Dockerfile**
- `hotkey-server/Dockerfile`
- 多阶段构建：golang:1.25-alpine 构建 → alpine:3.20 运行
- 复制二进制和迁移文件
- 验收：`docker build -t hotkey-server ./hotkey-server` 成功

**Task 33-2：前端 Dockerfile**
- `hotkey-web/Dockerfile`
- 多阶段构建：node:22-alpine 构建 → node:22-alpine 运行
- 使用 Next.js standalone 输出模式
- 验收：`docker build -t hotkey-web ./hotkey-web` 成功

**Task 33-3：docker-compose.yml**
- 根目录 `docker-compose.yml`
- 4 个服务：postgres（pgvector:pg16）、redis（7-alpine）、server、web
- 健康检查、依赖顺序、数据卷持久化
- 环境变量通过 `${VAR:-default}` 引用
- 验收：`docker compose up` 全部服务健康

**Task 33-4：根目录 Makefile**
- 快捷命令：up、down、build、logs、dev-server、dev-web
- 验收：`make up` 启动全部服务

**Task 33-5：环境变量模板**
- 根目录 `.env.example`
- 包含所有变量说明和默认值
- 验收：复制为 `.env` 后 `docker compose up` 可正常启动

**Task 33-6：Next.js standalone 配置**
- `hotkey-web/next.config.ts` 添加 `output: 'standalone'`
- 验收：`npm run build` 生成 `.next/standalone` 目录

## 4. TDD 与验证

- Docker 构建：每个 Dockerfile 独立构建验证
- Compose 启动：`docker compose up` 后检查所有服务健康
- 端到端：通过 `http://localhost:3000` 访问 Web，通过 `http://localhost:18080/healthz` 检查后端
- 数据持久化：`docker compose down && docker compose up` 后数据不丢失

## 5. 执行顺序

```
33-1 → 33-2（Dockerfile，并行）
33-6（Next.js 配置，与 33-2 同步）
33-3 → 33-4 → 33-5（Compose + Makefile + 环境变量）
```

## 6. 回滚策略

- Docker 文件为新增文件，删除即可回滚
- Next.js standalone 配置变更可 revert

## 7. 验收标准

- `docker compose up` 全部 4 个服务健康启动
- `http://localhost:3000` 可访问 Web 工作台
- `http://localhost:18080/healthz` 返回 200
- 数据持久化验证通过
- `make up`、`make down`、`make logs` 命令正常

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-28 | StephenQiu30 | 1.0.0 | 初版，Docker 部署能力实现计划 |
