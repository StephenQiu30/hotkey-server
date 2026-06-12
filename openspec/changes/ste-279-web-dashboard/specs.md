# Specs: 用户前台与管理后台

## Requirements

### R1: API 客户端
- MUST 提供 `buildURL(path)` 函数拼接 API base URL
- MUST 支持通过 `NEXT_PUBLIC_API_BASE_URL` 环境变量配置
- MUST 默认使用 `http://localhost:8080`

### R2: 监控任务列表
- MUST 渲染监控任务卡片，显示 `name` 和 `queryText`
- MUST 支持从 API 获取监控列表

### R3: 主题列表
- MUST 显示主题标题、热度和趋势方向
- MUST 支持从 API 获取主题列表

### R4: 提醒中心
- MUST 显示通知列表
- MUST 支持已读/未读状态

### R5: 管理后台 - 运行记录
- MUST 显示运行 ID、状态和采集数量
- MUST 支持 `failed`、`running`、`success` 状态展示

### R6: 管理后台 - 连接器状态
- MUST 显示连接器列表和健康状态

## Success path

1. 用户登录后可看到监控任务列表
2. 点击任务可查看内容流和关联主题
3. 主题页显示热度和趋势方向
4. 提醒中心展示通知列表
5. 管理后台展示运行记录和连接器状态

## Failure path

- API 不可用时显示错误状态
- 未登录时重定向到登录页

## Validation evidence

```bash
npm run lint --prefix web
npm run test --prefix web
```
