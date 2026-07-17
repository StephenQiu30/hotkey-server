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

当前已实现 PostgreSQL job 唯一键、稳定 ID/version 载荷、到期判断和调度键，并支持在调用方事务中原子入队（提交 `5fc28a2`）。River 表幂等与事务回滚 integration test 已通过。

尚未完成：六类业务 Job、Worker 生命周期、Cron 扫描、取消/重试/隔离、P0 RSS/HN 端到端和恢复故障注入；保持 `pending`。

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan010_test?sslmode=disable' go test -tags=integration ./internal/platform/queue -run 'TestEnqueue' -count=1
go test ./internal/platform/scheduler ./internal/platform/queue -count=1
```
