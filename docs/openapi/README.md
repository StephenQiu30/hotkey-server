# openapi

FastAPI 根据路由、Pydantic 请求/响应模型、状态码和安全声明自动生成接口文档：

- Swagger UI：运行后访问 `http://localhost:8867/docs`
- ReDoc：运行后访问 `http://localhost:8867/redoc`
- 运行时契约：`http://localhost:8867/openapi.json`
- 可复现发布快照：[openapi.json](openapi.json)

不得手写 `openapi.json`、生成的 TypeScript 类型或端点函数。修改后端 API 后，从仓库根目录执行：

```sh
uv run --directory backend/src python -m tools.export_openapi
npm run generate --prefix frontend
```

生成客户端位于 `frontend/src/generated/api/`。业务代码只调用其中的端点函数；`frontend/src/shared/api/request.ts` 是唯一 Axios 传输封装。CI 运行后端 OpenAPI 漂移检查与前端重新生成逐文件比较，任一手工改动都会失败。
