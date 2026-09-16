import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;

if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("event trend rule uses generated APIs and refreshes the notification inbox", async ({
  page,
}) => {
  const eventId = "00000000-0000-0000-0000-000000000501";
  const ruleId = "00000000-0000-0000-0000-000000000502";
  const occurrenceId = "00000000-0000-0000-0000-000000000503";
  const notificationId = "00000000-0000-0000-0000-000000000504";
  let rule: Record<string, unknown> | null = null;
  let alerted = false;
  const writes: Array<{ method: string; path: string; body: unknown }> = [];

  await page.route(/\/api\/events(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({
      json: {
        items: [
          {
            id: eventId,
            title: "合成趋势事件",
            summary: "浏览器契约验证",
            status: "active",
            current_revision: 1,
            created_at: "2026-09-15T00:00:00Z",
            updated_at: "2026-09-15T00:00:00Z",
            members: [],
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route(`**/api/events/${eventId}/trends?**`, async (route) => {
    await route.fulfill({
      json: {
        event_id: eventId,
        metric_version: "event-trend-v1",
        timezone: "UTC",
        since: "2026-09-09T00:00:00Z",
        until: "2026-09-16T00:00:00Z",
        bucket_hours: 24,
        sources: [
          {
            source: "bilibili",
            buckets: [
              {
                starts_at: "2026-09-15T00:00:00Z",
                ends_at: "2026-09-16T00:00:00Z",
                new_posts: 2,
                new_discussions: 0,
                observed_reply_delta: 0,
                coverage_status: "comparable",
                interruption_reasons: [],
                live_run_count: 1,
                backfill_run_count: 0,
                excluded_backfill_items: 0,
                excluded_backfill_observations: 0,
                policy_versions: ["policy-v1"],
              },
            ],
          },
        ],
      },
    });
  });
  await page.route(
    `**/api/events/${eventId}/trend-alert-rules`,
    async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ json: rule === null ? [] : [rule] });
        return;
      }
      writes.push({
        method: route.request().method(),
        path: new URL(route.request().url()).pathname,
        body: route.request().postDataJSON(),
      });
      rule = {
        id: ruleId,
        event_id: eventId,
        source: "bilibili",
        metric: "new_posts",
        bucket_hours: 24,
        threshold_count: 1,
        version: 1,
        enabled: true,
        created_at: "2026-09-16T00:05:00Z",
        updated_at: "2026-09-16T00:05:00Z",
      };
      await route.fulfill({ status: 201, json: rule });
    },
  );
  await page.route(
    `**/api/events/${eventId}/trend-alert-rules/${ruleId}`,
    async (route) => {
      writes.push({
        method: route.request().method(),
        path: new URL(route.request().url()).pathname,
        body: route.request().postDataJSON(),
      });
      rule = { ...rule, version: 2, enabled: false };
      await route.fulfill({ json: rule });
    },
  );
  await page.route(
    `**/api/events/${eventId}/trend-alerts/evaluate`,
    async (route) => {
      writes.push({
        method: route.request().method(),
        path: new URL(route.request().url()).pathname,
        body: route.request().postData(),
      });
      alerted = true;
      await route.fulfill({
        json: { evaluated_rules: 1, created_notifications: 1 },
      });
    },
  );
  await page.route(/\/api\/notifications(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: {
        items: alerted
          ? [
              {
                id: notificationId,
                event_id: eventId,
                change_id: null,
                trend_occurrence_id: occurrenceId,
                rule_version: 1,
                kind: "trend_threshold_reached",
                message: "bilibili 24小时桶新增根帖达到 2（阈值 1）",
                created_at: "2026-09-16T00:05:00Z",
                read_at: null,
              },
            ]
          : [],
        next_cursor: null,
        unread_count: alerted ? 1 : 0,
      },
    });
  });

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const eventsSection = page.getByRole("region", { name: /事件档案/ });
  const card = eventsSection.getByRole("article").filter({
    has: page.getByRole("heading", { name: "合成趋势事件", exact: true }),
  });
  await card.getByRole("button", { name: "查看近 7 天趋势" }).click();
  await expect(card.getByRole("cell", { name: "可比较" })).toBeVisible();
  await card.getByRole("button", { name: "创建提醒" }).click();
  await expect(card.getByText("趋势提醒规则已创建")).toBeVisible();
  await expect(card.getByText(/B站 · 24 小时 · 新增根帖 ≥ 1/)).toBeVisible();
  await card.getByRole("button", { name: "检查提醒" }).click();
  await expect(card.getByText("新增 1 条趋势提醒")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "事件提醒 1 未读" }),
  ).toBeVisible();
  await expect(page.getByText("趋势阈值")).toBeVisible();
  await card.getByRole("button", { name: "停用" }).click();
  await expect(card.getByRole("button", { name: "启用" })).toBeVisible();

  expect(writes).toEqual([
    {
      method: "POST",
      path: `/api/events/${eventId}/trend-alert-rules`,
      body: {
        source: "bilibili",
        metric: "new_posts",
        bucket_hours: 24,
        threshold_count: 1,
      },
    },
    {
      method: "POST",
      path: `/api/events/${eventId}/trend-alerts/evaluate`,
      body: null,
    },
    {
      method: "PATCH",
      path: `/api/events/${eventId}/trend-alert-rules/${ruleId}`,
      body: { expected_version: 1, threshold_count: 1, enabled: false },
    },
  ]);
});
