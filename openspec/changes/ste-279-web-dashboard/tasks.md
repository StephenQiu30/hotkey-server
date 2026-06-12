# Tasks: 用户前台与管理后台

## T1: 初始化 Web 工程与 API 客户端
- Files: `web/package.json`, `web/tsconfig.json`, `web/app/layout.tsx`, `web/lib/api.ts`, `web/tests/api-client.test.ts`
- Test: `buildURL` returns correct URL
- Validation: `npm run test --prefix web`

## T2: 实现用户登录和监控任务列表页
- Files: `web/app/(dashboard)/login/page.tsx`, `web/app/(dashboard)/monitors/page.tsx`, `web/components/monitor-list.tsx`, `web/tests/monitor-list.test.tsx`
- Test: renders monitor cards with name and queryText
- Validation: `npm run test --prefix web`

## T3: 实现热点内容、主题和趋势页面
- Files: `web/app/(dashboard)/monitors/[id]/page.tsx`, `web/app/(dashboard)/monitors/[id]/topics/[topicId]/page.tsx`, `web/components/post-feed.tsx`, `web/components/topic-list.tsx`, `web/components/trend-chart.tsx`, `web/tests/topic-list.test.tsx`
- Test: shows topic heat and direction
- Validation: `npm run test --prefix web`

## T4: 实现提醒中心和管理后台
- Files: `web/app/(dashboard)/notifications/page.tsx`, `web/app/admin/page.tsx`, `web/app/admin/runs/page.tsx`, `web/app/admin/connectors/page.tsx`, `web/tests/admin-runs.test.tsx`
- Test: renders failed run state
- Validation: `npm run lint --prefix web && npm run test --prefix web`
