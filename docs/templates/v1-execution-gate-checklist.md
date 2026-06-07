---
layer: template
doc_no: TPL-001
audience: developer, agent
purpose: V1 ticket 执行门禁检查清单模板
---

# V1 执行门禁检查清单

> 每个 V1 ticket 执行前必须完成以下检查。复制本模板到 ticket workpad 中使用。

## 环境依赖

- [ ] `make e2e-up` 启动 E2E 基础设施
- [ ] PostgreSQL 可连接（`127.0.0.1:15432`）
- [ ] Redis 可连接（`127.0.0.1:16379`）
- [ ] MinIO 可连接（`127.0.0.1:19000`）— 如需要

## Provider Mock 状态

- [ ] AI provider simulator 可用（`tests/e2e/ai_simulator.go`）
- [ ] Fetcher simulator 可用（`tests/e2e/fetcher_simulator.go`）
- [ ] SMTP sink 可用（`tests/e2e/smtp_simulator.go`）
- [ ] 所有 simulator 五种行为已验证（正常/限流/授权失效/schema变化/空结果）

## 测试数据准备

- [ ] `tests/fixtures/seed.sql` 包含所需种子数据
- [ ] 种子数据覆盖正常路径和边界条件
- [ ] 如需额外 fixture，在 `tests/fixtures/` 下新增

## 测试验证

- [ ] `go test -tags e2e ./tests/e2e/... -run TestHealthCheck` 全部通过
- [ ] `go test -tags e2e ./tests/e2e/... -run TestAISimulator` 全部通过
- [ ] `go test -tags e2e ./tests/e2e/... -run TestFetcherSimulator` 全部通过
- [ ] `go test -tags e2e ./tests/e2e/... -run TestSMTPSink` 全部通过
- [ ] `go test -tags e2e ./tests/e2e/... -run TestAllBehaviors` 全部通过
- [ ] 本 ticket 新增测试全部通过
- [ ] `go test ./...` 全量测试无回归（E2E 通过 build tag 排除）

## 收尾

- [ ] `make e2e-down` 清理 E2E 基础设施
- [ ] 工作区干净（`git status` 无未提交文件）

## 本 Ticket 特有检查

> 在下方添加本 ticket 的特定验收标准。

- [ ] （待填写）
