# Design: 动态 Swagger 接入收敛

## 目标

1. 通过访问后端接口动态获取 Swagger 文档数据和页面。
2. 文档来源统一为代码注释/注解驱动，不再依赖静态 JSON/YAML 文件。
3. 请求模型采用 `XXXRequest` 命名，响应模型采用 `XXXResponse` 命名。
4. 注释量控制在"正常接入 Swagger"的水平。

## 非目标

1. 不自研运行时 OpenAPI 拼装器。
2. 不为 Swagger 建立额外的"契约层""映射层""文档 DTO 层"。
3. 不在本次设计中处理前端客户端代码生成策略。

## 推荐方案

采用 **swaggo/swag + gin-swagger** 运行时动态暴露。

**理由**：
- 最接近 Go Gin 生态标准 Swagger 使用体验
- 不需要自己维护整份 OpenAPI 结构
- 注释天然和 handler、请求模型、响应模型同源

## 架构边界

Swagger 只属于 `hotkey-server` 的 HTTP 入口层。

- 全局元信息：`cmd/hotkey/main.go`
- Handler 注释：`internal/platform/http/`
- 请求模型：`XXXRequest` 命名
- 响应模型：`XXXResponse` 命名

## 目录/文件职责

| 路径 | 职责 |
|------|------|
| `cmd/hotkey/main.go` | 全局 Swagger 元信息注释 + gin-swagger 路由注册 |
| `internal/platform/http/` | 命名 handler + Swagger 注释 |
| `internal/platform/http/request.go` | `XXXRequest` 模型 |
| `internal/platform/http/response.go` | `XXXResponse` 模型 |
| `docs/docs.go` | swaggo 自动生成的运行时桥接代码（不手工维护） |

## 决策记录

1. **采用 swaggo/gin-swagger 而非自定义组装** — 原因：避免重发明半套 Swagger，长期维护成本低。
2. **废弃静态文件作为事实源** — 原因：避免两套机制不一致，docs/README.md 绑定。
3. **运行时桥接代码允许存在但不视为事实源** — 原因：swaggo 技术限制，但维持正确的职责认知。
4. **生产环境默认关闭 Swagger UI** — 原因：安全考虑，通过 `SWAGGER_ENABLED` 环境变量控制。

## 状态流

```
服务启动 → gin-swagger 路由注册（非生产环境或 SWAGGER_ENABLED=true）
         → /swagger/index.html 可访问
         → /swagger/doc.json 从 docs.go（swaggo 生成）读取
         → 开发者浏览器访问 Swagger UI → 页面加载 /swagger/doc.json 展示文档
```

## 失败路径

见设计文档 `docs/superpowers/specs/2026-07-04-dynamic-swagger-design.md` 第 224-260 行的 4 个运行时异常场景。

## 回滚影响

- 阶段三（清理旧链路）的删除操作必须在一个独立 commit 中完成，支持独立回滚。
- 旧 `docs/openapi.json` 在阶段三之前始终保持可用，作为回滚兼容基线。
