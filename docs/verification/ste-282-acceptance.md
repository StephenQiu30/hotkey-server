# STE-282 Acceptance Verification

## 验收门禁

| # | Criteria | Status | Evidence |
|---|---|---|---|
| 1 | 单次轮询能够生成 `monitor_runs` | ✅ PASS | `TestPollMonitorCreatesSuccessfulRun` |
| 2 | X 返回内容能标准化入库 | ✅ PASS | `TestSearchPostsParsesFixtures` + `TestNormalizeCreatesStableHash` |
| 3 | 重复内容不会重复插入 `platform_posts` | ✅ PASS | `unique(platform, platform_post_id)` + `TestNormalizeDeduplicatesIdenticalContent` |
| 4 | 监控任务与内容命中关系可以查询和更新 | ✅ PASS | `TestUpsertMonitorHitDeduplicates` |

## 验证命令

```bash
go test ./internal/platform/x/... -v    # 2 tests PASS
go test ./internal/content/... -v       # 8 tests PASS
go test ./internal/jobs/... -v          # 10 tests PASS
go test ./...                           # 14 packages, all PASS
```

## 交付物

- `internal/platform/x/` — connector、类型、fixture 测试
- `internal/content/` — 标准化、去重、命中关系
- `internal/jobs/poll_monitor.go` — 轮询任务编排
- `db/schema.sql` — monitor_runs、platform_authors、platform_posts、monitor_post_hits
