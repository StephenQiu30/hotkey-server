---
layer: Plan
doc_no: "013"
audience: [Dev, QA, Ops]
feature_area: 可靠任务编排
purpose: 用 Cron 与 River 编排 P0 热点事件主链路
canonical_path: docs/plans/013-Cron与River主链路编排计划.md
status: review
execution_status: backlog
review_status: pending
version: v1.1
owner: HotKey Server Team
inputs:
  - docs/prd/013-Cron与River主链路编排.md
  - docs/plans/archive/006-查询规划与RSS-HN采集计划.md
  - docs/plans/archive/007-内容标准化去重与MinIO证据计划.md
  - docs/plans/archive/008-AIProvider与Embedding基础计划.md
  - docs/plans/archive/009-多语言相关性匹配与反馈计划.md
  - docs/plans/archive/010-事件聚类生命周期与人工治理计划.md
  - docs/plans/archive/011-热度趋势与监控排序计划.md
  - docs/plans/archive/012-证据化事件摘要实体与主张计划.md
outputs:
  - Cron 调度与六类 P0 River Job
  - 可恢复 P0 主链路
triggers:
  - PRD-013 accepted 且 ready
downstream:
  - docs/acceptance/013-Cron与River主链路编排验收.md
depends_on: [PLAN-006, PLAN-007, PLAN-008, PLAN-009, PLAN-010, PLAN-011, PLAN-012]
---

# Cron 与 River 主链路编排计划

## 计划目标

把 006–012 的同步能力接入持久化 Job，使重复、崩溃、取消和单来源故障不会破坏业务事实。

## 开工条件

- 当前 Plan 的 status 为 accepted、review_status 为 approved、execution_status 为 ready
- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/platform/queue/river.go | River 客户端与 Worker |
| 创建 | internal/platform/scheduler/cron.go | 到期扫描与唯一入队 |
| 创建 | internal/bootstrap/worker.go | worker/all 装配 |
| 创建 | internal/modules/source/infrastructure/jobs/collect.go | collect_source |
| 创建 | internal/modules/ingestion/infrastructure/jobs/*.go | normalize 与 relevance |
| 创建 | internal/modules/event/infrastructure/jobs/*.go | cluster 与 heat |
| 创建 | internal/modules/intelligence/infrastructure/jobs/summary.go | summary |
| 创建 | internal/modules/operations/transport/http/jobs.go | 运行、重试和取消 API |
| 修改 | db/schema.sql | 运行状态与 River 基础设施 |
| 修改 | internal/platform/config/config.go | 队列、并发、超时和 Cron |
| 创建 | test/integration/pipeline_test.go | P0 端到端与恢复测试 |

## 任务与执行标准

每个任务必须先让对应 RED 测试失败，再完成最小实现；结束时运行列出的回归、提交并推送 `main`，工作区必须为空。本文不增加 Markdown 解析或文档内容 CI；计划、代码、OpenAPI 和 Acceptance 通过明确文件改动与测试结果保持同步。

### Task 1：收紧版本化 Job 信封与持久化队列契约

**文件：** 修改 `internal/platform/queue/{job_types,queue,worker}.go`；创建 `test/_suite/internal/platform/queue/*_test.go`。

- [x] **RED：** 未知 Job、缺少版本/关联 ID、无界 payload、重复唯一键、事务回滚和永久/临时错误分类失败。
- [x] **GREEN：** 所有 P0 Job 使用版本化、小型 ID 信封；`river_job` 入队与调用方事务一致，唯一键重复返回同一执行。
- [x] **重构：** 统一重试、隔离和取消结果，不让业务 Handler 直接操作 River 表。
- [x] **回归：** `go run ./test/runner test ./internal/platform/queue -run 'Test(Payload|Job|ErrorClassification)' -count=1`；`HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' go run ./test/runner test -tags=integration ./internal/platform/queue -run 'Test(Enqueue|Worker)' -count=1`。
- [x] **提交：** `3c53744 feat: harden durable job envelope`。

### Task 2：装配可取消、可恢复的 Worker 生命周期

**文件：** 修改 `internal/bootstrap/{app,lifecycle}.go`、`internal/platform/{config,queue/worker}.go`；创建 `internal/bootstrap/worker.go` 与相关测试。

- [x] **RED：** `worker`/`all` 未启动处理循环、停机未取消、过期 running Job 不能重领时失败。
- [x] **GREEN：** Worker 按配置并发和轮询间隔启动，停机传播 context，启动时回收过期 lease；`api` 角色不启动 Worker。
- [x] **重构：** 生命周期只在 Bootstrap 装配，Queue 保持无业务模块依赖。
- [x] **回归：** `go run ./test/runner test ./internal/bootstrap ./internal/platform/config -count=1`；`HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' go run ./test/runner test ./internal/bootstrap -run 'TestConfiguredWorkerVerifiesDatabaseOnStart' -count=1`；`HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' go run ./test/runner test -tags=integration ./internal/platform/queue -run 'TestWorkerReclaimsExpiredRunningJobs' -count=1`；`HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' make ci && make clean`。
- [x] **提交：** `2ea981f feat: run recoverable worker lifecycle`。

### Task 3：扫描已发布配置并提交唯一采集任务

**文件：** 创建 `internal/platform/scheduler/source_due_reader.go`；修改 `internal/platform/scheduler/cron.go`、`internal/modules/source/infrastructure/postgres/*`、Bootstrap 装配；创建 scheduler/PostgreSQL 测试。

- [x] **RED：** draft、paused、archived、disabled source 或未来 checkpoint 被调度，以及重复扫描产生重复 Job 时失败。
- [x] **GREEN：** Cron 仅消费 active Monitor 的 immutable published target，按 UTC 时间片提交 `collect_source` 唯一任务。
- [x] **重构：** 调度器只扫描和入队，不调用 Connector 或推进 checkpoint。
- [x] **回归：** `go run ./test/runner test ./internal/platform/scheduler -run 'TestCollectionScheduler' -count=1`；`HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' go run ./test/runner test -tags=integration ./internal/modules/monitor/infrastructure/postgres -run 'Test(CollectionScheduler|PublishedCollectionTargetReader)' -count=1`；`HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' make ci && make clean`。
- [x] **提交：** `8cc7e88 feat: schedule due source collection`。

### Task 4：实现 P0 Job 链路与事务下游入队

**文件：** 创建 `internal/modules/{source,ingestion,event,intelligence}/infrastructure/jobs/*.go`；修改各自 Application port、Repository 与 Bootstrap provider。

- [ ] **RED：** 六类 Job 的非法版本、重复执行、单来源暂态故障、无 Provider 和取消边界失败。
- [ ] **GREEN：** `collect_source → normalize_content → evaluate_relevance → cluster_content → recompute_event_heat → generate_event_summary` 只传稳定 ID/版本/哈希，重新读取事实，事务中提交下游 Job。
- [ ] **重构：** 每个 Job 依赖所属 Application 用例；Job 包不跨模块读取表、不回传 Provider 原始结果。
- [ ] **回归：** `go run ./test/runner test ./internal/modules/source/... ./internal/modules/ingestion/... ./internal/modules/event/... ./internal/modules/intelligence/... -count=1`。
- [ ] **提交：** `feat: orchestrate p0 event pipeline jobs`。

### Task 5：提供受限运行查询、取消与重试控制

**文件：** 创建 `internal/modules/operations/{application,infrastructure/postgres,transport/http}/jobs.go`；修改 `internal/bootstrap/app.go`、`docs/openapi/swagger.json` 与 `test/architecture/openapi_test.go`。

- [ ] **RED：** 未认证、非管理员、未知 Job、已完成 Job、非法重试/取消和敏感 payload 泄露失败。
- [ ] **GREEN：** 管理员可查询安全运行状态、取消未开始任务并受限重试可恢复任务；API 不暴露正文、密钥、Prompt 或 Provider 原始响应。
- [ ] **重构：** 统一 Result、错误码与审计，Transport 不直接访问 Queue 表。
- [ ] **回归：** `go run ./test/runner test ./internal/modules/operations/... ./test/architecture -run 'Job|OpenAPI|Result' -count=1`。
- [ ] **提交：** `feat: manage durable job runs`。

### Task 6：固定可恢复 P0 端到端验收

**文件：** 创建 `test/integration/pipeline_test.go`、`test/fixtures/pipeline/v1/*`；修改 `docs/acceptance/013-Cron与River主链路编排验收.md`。

- [ ] **RED：** 重复时间片、Worker 崩溃、单来源失败、停用 Monitor、Provider 不可用和 checkpoint 不完整 fixture 失败。
- [ ] **GREEN：** RSS/HN fixture 跑通 Content、Match、Event、Heat 与可降级 Summary；重跑不重复事实且恢复后继续。
- [ ] **重构：** fixture 不依赖网络、真实模型或开发数据，删除重复辅助代码。
- [ ] **回归：** `HOTKEY_TEST_DSN="$HOTKEY_TEST_DSN" HOTKEY_TEST_REDIS_URL="$HOTKEY_TEST_REDIS_URL" go run ./test/runner test -tags=integration ./test/integration -count=1`，随后 `make ci && make clean`。
- [ ] **提交：** `test: add recoverable p0 pipeline acceptance`。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | `go run ./test/runner test -tags=integration ./test/integration -run Pipeline -count=1` | 因队列与 Job 缺失失败 |
| 绿灯 | `go run ./test/runner test ./internal/platform/queue ./internal/platform/scheduler ./internal/modules/... -count=1` | Job 单元测试通过 |
| 恢复 | `go run ./test/runner test -tags=integration ./test/integration -run PipelineRecovery -count=1` | 重启与重复执行通过 |
| 主链路 | `go run ./test/runner test -tags=integration ./test/integration -run RSSHNPipeline -count=1` | P0 链路通过 |
| 全量 | `make ci && make clean` | 全部通过 |

## 验收清单

- 业务写入与下游 Job 同事务
- 相同幂等键只存在一个有效执行
- 单来源失败不阻塞其他来源
- 任意 Job 前后退出可恢复
- 暂停 Monitor 不再提交新采集
- 无 LLM、Vault 或 SMTP 时 P0 Event API 可用
- 95% 正常内容在 60 分钟内形成或更新 Event

## 提交边界

- test: 定义 River 幂等与恢复门禁
- impl: 接入 River、Cron 和 Worker
- feat: 编排 P0 热点主链路


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
