# HotKey

[![CI](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml)

HotKey 采用单体仓库管理后端与 Web 工作台。

| 目录 | 内容 |
| --- | --- |
| [`backend/`](backend/README.md) | Go 后端、PostgreSQL Schema、采集与 AI 任务、运行时 OpenAPI 和镜像配置 |
| [`frontend/`](frontend/README.md) | Next.js Web 工作台、页面组件和生成的 API 客户端 |
| [`docs/`](docs/README.md) | 不按应用分层的统一正式文档与 OpenAPI 发布契约 |

## 开始开发

克隆仓库后，在对应子目录执行命令：

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server

# 后端
cd backend
cp .env.example .env
go run ./cmd/hotkey

# 前端（另开终端）
cd frontend
npm ci
npm run dev
```

后端默认监听 `http://127.0.0.1:8080`，前端默认通过自身的 Next.js 服务访问后端。环境变量与部署方式分别见两个子项目的 README。

## Docker Compose

根 `docker-compose.yml` 统一定义前端、后端、PostgreSQL、Redis、MinIO、默认环境配置、健康检查和卷；生产文件只覆盖生产环境差异。默认环境：

```bash
cp backend/.env.example backend/.env
cp .env.example .env
docker compose -f docker-compose.yml up --build -d
```

生产环境：

```bash
cp .env.prod.example .env.prod
docker compose --env-file .env.prod -f docker-compose-prod.yml up --build -d
```

公共服务、初始化命令、依赖和健康检查均在默认 Compose 文件中完整定义，生产文件只覆盖镜像、凭据和运行环境。子项目目录不维护 Compose；Dockerfile 与 `.dockerignore` 仍跟随各自应用，以保持最小构建上下文。

## 质量检查

```bash
cd backend
make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build
```

后端 OpenAPI 发布契约位于 [`docs/openapi/swagger.json`](docs/openapi/swagger.json)，运行时注册代码位于 `backend/openapi/docs.go`，生成的前端客户端位于 `frontend/src/services/hotkey/hotkey-server/`。

贡献和安全报告请参阅 [CONTRIBUTING.md](CONTRIBUTING.md) 与 [SECURITY.md](SECURITY.md)。
