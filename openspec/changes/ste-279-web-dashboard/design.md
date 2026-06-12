# Design: 用户前台与管理后台

## Architecture

单个 Next.js App Router 工程，路由分组：
- `(marketing)/` — 营销页（首页）
- `(dashboard)/` — 用户仪表盘（登录、监控、内容、主题、趋势、提醒）
- `admin/` — 管理后台（运行记录、连接器）

## Tech stack

- Next.js 15 (App Router)
- TypeScript
- React 19
- Vitest + @testing-library/react
- ESLint

## API layer

`web/lib/api.ts` 封装 base URL 构建和 fetch 请求，后端 API 契约参考 `docs/plans/002-*` 和 `docs/plans/004-*`。

## Testing

- Vitest 配合 jsdom 环境
- 测试文件放在 `web/tests/` 目录
- 组件测试使用 @testing-library/react

## File structure

```
web/
├── app/
│   ├── layout.tsx          # 根布局
│   ├── page.tsx            # 首页
│   ├── (dashboard)/
│   │   ├── login/page.tsx
│   │   ├── monitors/page.tsx
│   │   ├── monitors/[id]/page.tsx
│   │   ├── monitors/[id]/topics/[topicId]/page.tsx
│   │   └── notifications/page.tsx
│   └── admin/
│       ├── page.tsx
│       ├── runs/page.tsx
│       └── connectors/page.tsx
├── components/
│   ├── monitor-list.tsx
│   ├── post-feed.tsx
│   ├── topic-list.tsx
│   ├── trend-chart.tsx
│   └── admin-run-table.tsx
├── lib/
│   └── api.ts
├── tests/
│   ├── api-client.test.ts
│   ├── monitor-list.test.tsx
│   ├── topic-list.test.tsx
│   └── admin-runs.test.tsx
├── package.json
└── tsconfig.json
```
