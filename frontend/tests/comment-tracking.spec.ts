import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;

if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("one inbox match starts persisted comment tracking through the generated API", async ({
  page,
}) => {
  const contentId = "00000000-0000-0000-0000-000000000901";
  const monitorId = "00000000-0000-0000-0000-000000000902";
  const monitorVersionId = "00000000-0000-0000-0000-000000000903";
  const matchId = "00000000-0000-0000-0000-000000000904";
  const runId = "00000000-0000-0000-0000-000000000905";
  const jobId = "00000000-0000-0000-0000-000000000906";
  const now = new Date().toISOString();
  let reviewState: "new" | "following" = "new";
  const trackingRequests: string[] = [];

  await page.route(/\/api\/monitors(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({
      json: {
        items: [
          {
            id: monitorId,
            title: "评论研究",
            state: "active",
            current_version: 1,
            query_spec: {
              include_any: ["AI"],
              include_all: [],
              exclude: [],
              aliases: [],
            },
            source_ids: ["bilibili"],
            schedule: { interval_minutes: 60, retention_days: 7 },
            budget: { daily_requests: 120, content_purchase_cost: 0 },
            created_at: now,
            updated_at: now,
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route(/\/api\/contents(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({
      json: {
        items: [
          {
            id: contentId,
            source: "bilibili",
            provider_namespace: "video",
            external_id: "aid:200",
            kind: "post",
            root_external_id: "aid:200",
            parent_external_id: null,
            relation_status: "root",
            text: "用户选择追踪的第二条内容",
            version: 1,
            canonical_url: "https://www.bilibili.com/video/av200",
            published_at: now,
            first_seen_at: now,
            last_seen_at: now,
            reply_count: 3,
            matches: [
              {
                id: matchId,
                monitor_id: monitorId,
                monitor_version_id: monitorVersionId,
                monitor_version: 1,
                monitor_title: "评论研究",
                match_reason: ["AI"],
                relevance_status: "accepted",
                review_state: reviewState,
                first_seen_at: now,
                last_seen_at: now,
              },
            ],
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route(
    new RegExp(`/api/monitor-matches/${matchId}/comment-tracking$`),
    async (route) => {
      trackingRequests.push(new URL(route.request().url()).pathname);
      reviewState = "following";
      await route.fulfill({
        json: {
          match_id: matchId,
          content_id: contentId,
          root_content_id: contentId,
          review_state: "following",
          replayed: false,
          run: {
            id: runId,
            job_id: jobId,
            parent_run_id: "00000000-0000-0000-0000-000000000907",
            monitor_version_id: monitorVersionId,
            source: "bilibili",
            operation: "fetch_post",
            request_value: "aid:200",
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
            window_since: "2026-09-15T00:00:00Z",
            window_until: "2026-09-16T00:00:00Z",
            created_at: now,
            completed_at: null,
          },
        },
      });
    },
  );

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const inbox = page.getByRole("region", { name: /监控收件箱/ });
  expect(trackingRequests).toEqual([]);
  await inbox.getByRole("button", { name: "追踪评论 评论研究" }).click();
  await expect(inbox.getByText(/评论追踪中/)).toBeVisible();
  expect(trackingRequests).toEqual([
    `/api/monitor-matches/${matchId}/comment-tracking`,
  ]);
});
