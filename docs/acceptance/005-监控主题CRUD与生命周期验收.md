---
layer: Acceptance
scope: shared
doc_no: "005"
title: 监控主题CRUD与生命周期验收
status: passed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/acceptance/005-监控主题CRUD与生命周期验收.md
design: docs/design/archive/005-监控主题CRUD与生命周期设计.md
prd: docs/prd/archive/005-监控主题CRUD与生命周期.md
plan: docs/plans/archive/005-监控主题CRUD与生命周期计划.md
---

# 监控主题CRUD与生命周期验收

## 验收结论

005 通过验收。编辑者可以创建监控、修改草稿并完成发布前预览；管理员可以发布、暂停、恢复、归档、还原与软删除监控。已发布配置版本不可变，历史按修订倒序可追溯；只读用户不可见未发布草稿。暂停后调度目标为空，恢复后重新生成目标；重复状态操作幂等，过期版本返回明确冲突。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-005-1 | passed | PostgreSQL 仓储测试验证配置不可覆盖和历史倒序；真实浏览器发布两个版本后，详情显示 revision 2 为 `published`、revision 1 为 `superseded`，后续 revision 3 草稿对 viewer 不可见。 |
| AC-005-2 | passed | 真实 PostgreSQL 集成测试验证 paused 监控的调度目标数为 0，resume 后目标数恢复为 1；浏览器详情与列表同步显示暂停和恢复状态。 |
| AC-005-3 | passed | 单元与 HTTP 测试覆盖同目标 pause/resume 不增加版本、过期版本冲突；真实 API 验证过期操作返回 HTTP 409 与业务码 `30001`，editor 发布和 viewer 暂停均返回 HTTP 403。 |

## 自动化验证

```bash
cd backend
PATH=/opt/homebrew/opt/postgresql@16/bin:$PATH \
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:55432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod \
  -f docker-compose-prod.yml config --quiet
git diff --check
```

完整验证在只包含本任务暂存树的隔离工作区执行，避免开发者本地未提交配置影响仓库门禁。后端覆盖 OpenAPI 再生成、vet、PostgreSQL/Redis 运行时、全量测试、构建、架构与 Schema 检查；前端覆盖 OpenAPI 客户端一致性、类型检查、全量单元测试与 Next.js 生产构建。

## 功能与权限证据

- 页面使用 shadcn/ui 的 Card、Table、Dialog、AlertDialog、Select、Badge、Alert、Skeleton、Empty 和 Button 组合实现列表、表单、预览、详情、确认与各类状态。
- viewer 只能读取活动或暂停的监控及已发布历史；editor 可创建、编辑和预览；admin 可发布并执行全部生命周期动作。前端不向 viewer 请求来源管理接口，服务端仍独立执行权限与状态校验。
- 预览明确显示发布资格、预计请求数、配置哈希和警告；管理员必须先完成预览才能在交互中确认发布，服务端发布用例会再次校验当前草稿。
- 列表使用稳定 cursor 分页；详情使用新增的 `GET /api/v1/monitors/{id}/versions` 读取安全版本历史，只暴露状态、哈希和发布时间等必要元数据。

## 浏览器验收

使用 `agent-browser` 连接真实 Next.js、Go、PostgreSQL 与 Redis：

- 管理员完成创建、编辑、预览、两次发布、暂停/恢复、归档/还原和软删除；删除对话框先用 Escape 取消，再确认执行。
- editor 可创建新草稿并预览，不显示发布确认与生命周期动作；viewer 仅可查看详情，版本历史不包含 editor 新增的草稿。
- 1440 × 1000 与 390 × 844 视口下布局可用，移动端操作自然换行；监控列表、详情对话框和预览对话框 axe 扫描均为 0 violations，无应用 JavaScript 异常。

截图证据保存在本地 `/tmp/hotkey-005-desktop-monitors.png` 与 `/tmp/hotkey-005-mobile-monitors.png`，不作为仓库事实源提交。

## 失败、降级与回滚

- 列表失败显示可重试错误态；加载、空列表和权限状态均有独立反馈，写入失败不会丢失当前草稿。
- 生命周期操作携带期望版本，过期请求不会覆盖新状态；重复同目标操作不增加版本。
- 本条未修改 Schema，复用现有不可变配置版本结构。回滚页面与版本历史读取接口后，已有监控、配置、历史事件和调度事实保持不变。
