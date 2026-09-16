import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;

if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("active collection run can be cancelled through the generated job API", async ({
  page,
}) => {
  const jobId = "00000000-0000-0000-0000-000000000301";
  let cancelled = false;
  const cancelRequests: Array<{ method: string; url: string }> = [];

  await page.route(/\/api\/collection-runs(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: {
        items: [
          {
            id: "00000000-0000-0000-0000-000000000302",
            job_id: jobId,
            parent_run_id: null,
            monitor_version_id: "00000000-0000-0000-0000-000000000303",
            source: "bilibili",
            operation: "list_comments",
            request_value: "取消入口验证",
            retention_days: 7,
            trigger: "manual",
            ingestion_mode: "live",
            schedule_slot: null,
            budget_day: "2026-09-16",
            reserved_requests: 1,
            state: cancelled ? "cancelled" : "queued",
            outcome: cancelled ? "partial" : null,
            fencing_token: 1,
            pages_count: 0,
            items_count: 0,
            bytes_count: 0,
            stop_reason: cancelled ? "user_cancelled" : null,
            window_since: "2026-09-16T00:00:00Z",
            window_until: "2026-09-16T01:00:00Z",
            created_at: "2026-09-16T01:00:00Z",
            completed_at: cancelled ? "2026-09-16T01:00:01Z" : null,
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route(`**/api/jobs/${jobId}/cancel`, async (route) => {
    cancelRequests.push({
      method: route.request().method(),
      url: route.request().url(),
    });
    cancelled = true;
    await route.fulfill({
      json: {
        id: jobId,
        kind: "collect_page",
        status: "cancelled",
        epoch: 1,
        attempts: 0,
        deadline: "2026-09-16T01:30:00Z",
      },
    });
  });

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const run = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: "根评论：取消入口验证" }),
  });
  await expect(run).toContainText("排队中");
  const cancel = run.getByRole("button", {
    name: "取消采集 取消入口验证",
  });
  await expect(cancel).toBeVisible();
  await cancel.click();

  await expect(run).toContainText("已取消");
  await expect(cancel).toHaveCount(0);
  expect(cancelRequests).toHaveLength(1);
  expect(cancelRequests[0]?.method).toBe("POST");
  expect(new URL(cancelRequests[0]?.url ?? "").pathname).toBe(
    `/api/jobs/${jobId}/cancel`,
  );
});
