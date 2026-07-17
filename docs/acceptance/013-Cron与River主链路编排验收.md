---
layer: Acceptance
doc_no: "013"
audience: [Dev, QA, Ops]
feature_area: 可靠任务编排
purpose: 记录 PLAN-013 的可复核验收证据
canonical_path: docs/acceptance/013-Cron与River主链路编排验收.md
status: review
version: v0.1
owner: HotKey Server Team
result: pending
---

# Cron 与 River 主链路编排验收

已完成 PLAN-013 Task 1（提交 `3c53744`）：队列只接受已登记 kind，载荷要求正数实体版本、合法时间窗，并限制 JSON 载荷为 4096 字节、唯一键为 256 字节；Worker 保留完整任务信封，统一记录可重试、永久失败和取消状态。PostgreSQL 证据覆盖唯一键幂等、调用方事务回滚、SKIP LOCKED claim、成功完成、达到上限隔离和永久/取消分类。

已完成 PLAN-013 Task 2（提交 `2ea981f`）：`worker`/`all` 角色在 Bootstrap 中按 `worker_poll_interval`、`worker_concurrency` 和 `worker_lease_timeout` 启动受限循环，启动前回收过期租约，停机传播取消并等待循环退出；`api` 角色不提供 Worker。Fx 生命周期、配置、数据库启动和过期 running Job 回收测试通过，完整 `make ci` 在 PostgreSQL 与 Redis 可用时通过。

已完成 PLAN-013 Task 3（提交 `8cc7e88`）：复用 Monitor 的只读 `ListDue` 适配器，排除 draft/paused/disabled/deleted/future checkpoint，只将 immutable published target 转成 source/signature/window 任务；`worker`/`all` 按 `cron_interval` 启动扫描，`source_connection_id + query_signature + UTC window` 生成稳定 key。重复扫描只返回已存在 job，不写 `collection_runs`、Connector 或 checkpoint。真实 PostgreSQL 测试和完整质量门禁通过。

已完成 PLAN-013 Task 4（提交待本次代码提交）：六类 P0 Handler 已接入 Bootstrap。Job 只携带 ID/版本/窗口/哈希，Source Handler 重读 published target 后调用 CollectionService；Ingestion Handler 复用 IngestRun 并在 Content 事务内入队 evaluate；Evaluate 写入确定性 relevance snapshot 后入队 cluster；Cluster、Heat、Summary 复用 Event Application 用例。Source capture→normalize 与 Content bind→evaluate 的下游入队通过 transaction context 与业务事实同事务提交。无 MinIO 时不会阻断 Worker 启动，运行时以可重试 unavailable 分类。

本次 Task 4 回归证据：

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' go run ./test/runner test ./internal/modules/source/... ./internal/modules/ingestion/... ./internal/modules/event/... ./internal/modules/intelligence/... -count=1
sh test/tools/validate-architecture.sh
sh test/tools/validate-repository.sh
```

尚未完成：取消/重试管理 API、P0 RSS/HN 端到端和恢复故障注入；保持 `pending`。

```bash
go run ./test/runner test ./internal/platform/queue -run 'Test(Payload|Job|ErrorClassification)' -count=1
HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' go run ./test/runner test -tags=integration ./internal/platform/queue -run 'Test(Enqueue|Worker)' -count=1
go run ./test/runner test ./internal/platform/scheduler ./internal/platform/queue -count=1
```
