# 18-复杂消息队列与 Worker 池验收

## 验收范围

- 异步任务入队：`POST /api/v1/admin/work-queue/jobs`。
- 待处理任务查询：`GET /api/v1/admin/work-queue/jobs`。
- Worker 池执行：`POST /api/v1/admin/work-queue/run`。
- 失败补偿查询：`GET /api/v1/admin/work-queue/compensations`。
- 支持任务类型：
  - `collect`
  - `analyze`
  - `report`
- 支持优先级、失败重试和补偿记录。

## 非目标

- 不在本任务接入 Kafka、RabbitMQ、Redis Stream 或云消息队列。
- 不实现独立 Worker 进程部署，该范围属于 #89。
- 不引入跨服务追踪、死信队列运维面板或复杂分片调度。

## 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 验收结果

- 队列服务测试覆盖高优先级任务优先调度。
- 队列服务测试覆盖失败重试和补偿记录。
- Worker 池测试覆盖采集、分析、日报任务处理统计。
- HTTP 测试覆盖任务入队、执行和列表查询。
- OpenAPI 测试覆盖复杂消息队列与 Worker 池契约路径。
