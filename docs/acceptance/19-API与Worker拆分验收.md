# 19-API 与 Worker 拆分验收

## 验收范围

- 服务边界拓扑查询：`GET /api/v1/admin/service-boundaries`。
- API 服务和 Worker 服务可独立设置副本数。
- API 服务保留 OpenAPI 事实源职责。
- Worker 服务只承担采集、分析和日报任务消费职责。
- 任务消息契约明确声明必填字段：
  - `id`
  - `type`
  - `tenantId`
  - `priority`
  - `payload`
  - `maxAttempts`

## 非目标

- 不在本任务拆出独立二进制、容器镜像或部署编排。
- 不引入 Kafka、RabbitMQ、Redis Stream 或云消息队列。
- 不实现跨服务追踪、服务网格、动态扩缩容控制器或多服务发布流水线。

## 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 验收结果

- 服务层测试覆盖 API 与 Worker 独立扩缩容契约。
- 服务层测试覆盖任务消息必填字段和 schema version。
- HTTP 测试覆盖服务边界拓扑与任务消息契约查询。
- OpenAPI 测试覆盖 `GET /api/v1/admin/service-boundaries` 路径。
- README 已补充 API 列表和当前能力边界说明。
