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

登录后，管理员可在“来源管理”中直接填写和轮换第三方来源凭据。来源参数与加密密文以 PostgreSQL 为事实源；`.env` 只保留数据库、身份、存储等部署配置和独立的 `HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY`。旧 `env:NAME` 来源继续兼容，任何读取接口都不会返回凭据明文或引用。

### 配置加载与工作目录

后端从**进程工作目录**读取 `.env`；当配置的 `HOTKEY_ENV=production` 时，再叠加读取同目录 `.env.prod`（进程环境变量优先级最高）。因此：

- 从 `backend/` 目录执行 `go run ./cmd/hotkey` 时，读到的是 `backend/.env`。
- 若在仓库根目录另建了 `.env`，请确保启动后端时工作目录与预期读取的 `.env` 一致，或把配置通过进程环境变量注入。
- `HOTKEY_VAULT_PATH` 等相对路径按进程工作目录解析，建议本地填写绝对路径以避免歧义。

### 数据库初始化

```bash
cd backend
go run ./cmd/hotkey db init --empty-only --confirm-empty
go run ./cmd/hotkey db verify
```

`db init` 只接受**全新空库**（public schema 中无任何对象）。若目标库已有旧数据或已安装 extension，会拒绝执行。现有库与当前 `backend/db/schema.sql` 存在结构漂移时**无法增量升级**——请对全新空库初始化，或将现有数据备份后重建。若目标库由非超级用户持有，需先以超级用户安装 `pg_trgm` 与 `vector` 扩展，或临时授予该用户创建扩展的权限。

## Docker Compose

根 `docker-compose.yml` 统一定义前端、后端、PostgreSQL、Redis、MinIO、默认环境配置、健康检查和卷；生产文件只覆盖生产环境差异。默认环境：

```bash
cp backend/.env.example backend/.env
cp .env.example .env
docker compose -f docker-compose.yml up --build --detach --wait --wait-timeout 240
curl --fail --silent http://127.0.0.1:8080/readyz
curl --fail --silent http://127.0.0.1:8080/metrics | grep 'hotkey_runtime_metrics_collection_success'
```

生产环境：

```bash
cp .env.prod.example .env.prod
docker compose --env-file .env.prod -f docker-compose-prod.yml up --build --detach --wait --wait-timeout 240
```

公共服务、初始化命令、依赖和健康检查均在默认 Compose 文件中完整定义，生产文件只覆盖镜像、凭据和运行环境。子项目目录不维护 Compose；Dockerfile 与 `.dockerignore` 仍跟随各自应用，以保持最小构建上下文。

## 质量检查

```bash
cd backend
make ci
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm audit --omit=dev --audit-level=high
npm run build
```

后端 OpenAPI 发布契约位于 [`docs/openapi/swagger.json`](docs/openapi/swagger.json)，运行时注册代码位于 `backend/openapi/docs.go`，生成的前端客户端位于 `frontend/src/services/hotkey/hotkey-server/`。

部署升级、备份恢复、密钥轮换、来源故障和容量阈值见 [Operations 手册](docs/operations/README.md)。生产更新前必须完成恢复演练；回滚不删除事实、任务、审计或持久卷。

贡献和安全报告请参阅 [CONTRIBUTING.md](CONTRIBUTING.md) 与 [SECURITY.md](SECURITY.md)。
