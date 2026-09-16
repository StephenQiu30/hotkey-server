import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;

if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("active monitor starts one server-derived batch through the generated API", async ({
  page,
}) => {
  const monitorId = "00000000-0000-0000-0000-000000000401";
  const jobId = "00000000-0000-0000-0000-000000000402";
  const runId = "00000000-0000-0000-0000-000000000403";
  let created = false;
  const submissions: Array<Record<string, unknown>> = [];

  await page.route(/\/api\/monitors(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({
      json: {
        items: [
          {
            id: monitorId,
            title: "AI 热点",
            state: "active",
            current_version: 3,
            query_spec: {
              include_any: ["AI"],
              include_all: [],
              exclude: [],
              aliases: ["人工智能"],
            },
            source_ids: ["bilibili"],
            schedule: { interval_minutes: 60, retention_days: 7 },
            budget: { daily_requests: 192, content_purchase_cost: 0 },
            created_at: "2026-09-16T00:00:00Z",
            updated_at: "2026-09-16T00:00:00Z",
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route(`**/api/monitors/${monitorId}/runs`, async (route) => {
    submissions.push(route.request().postDataJSON());
    created = true;
    await route.fulfill({
      status: 201,
      json: {
        items: [collectionRun(runId, jobId)],
        replayed: false,
      },
    });
  });
  await page.route(/\/api\/collection-runs(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: {
        items: created ? [collectionRun(runId, jobId)] : [],
        next_cursor: null,
      },
    });
  });

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const monitor = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: "AI 热点", exact: true }),
  });
  await monitor.getByRole("button", { name: "立即采集 AI 热点" }).click();

  const run = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: "搜索：AI" }),
  });
  await expect(run).toContainText("手动任务");
  await expect(run).toContainText("排队中");
  expect(submissions).toHaveLength(1);
  expect(Object.keys(submissions[0] ?? {}).sort()).toEqual([
    "expected_version",
    "idempotency_key",
  ]);
  expect(submissions[0]?.expected_version).toBe(3);
  expect(submissions[0]?.idempotency_key).toMatch(
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
  );
});

function collectionRun(id: string, jobId: string) {
  return {
    id,
    job_id: jobId,
    parent_run_id: null,
    monitor_version_id: "00000000-0000-0000-0000-000000000404",
    source: "bilibili",
    operation: "search_posts",
    request_value: "AI",
    retention_days: 7,
    trigger: "manual",
    ingestion_mode: "live",
    schedule_slot: null,
    budget_day: "2026-09-16",
    reserved_requests: 1,
    state: "queued",
    outcome: null,
    fencing_token: 0,
    pages_count: 0,
    items_count: 0,
    bytes_count: 0,
    stop_reason: null,
    window_since: "2026-09-16T00:00:00Z",
    window_until: "2026-09-16T01:00:00Z",
    created_at: "2026-09-16T01:00:00Z",
    completed_at: null,
  };
}
