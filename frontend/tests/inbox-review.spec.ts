import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;

if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("inbox review updates one topic match and keeps generated filters", async ({
  page,
}) => {
  const contentId = "00000000-0000-0000-0000-000000000601";
  const firstMonitorId = "00000000-0000-0000-0000-000000000602";
  const secondMonitorId = "00000000-0000-0000-0000-000000000603";
  const firstMatchId = "00000000-0000-0000-0000-000000000604";
  const secondMatchId = "00000000-0000-0000-0000-000000000605";
  const states = new Map<string, "new" | "ignored" | "following">([
    [firstMatchId, "new"],
    [secondMatchId, "new"],
  ]);
  const writes: Array<{ path: string; body: unknown }> = [];
  const queries: URLSearchParams[] = [];
  const monitors = [
    {
      id: firstMonitorId,
      title: "主题甲",
      state: "draft",
      current_version: 1,
      query_spec: {
        include_any: ["甲"],
        include_all: [],
        exclude: [],
        aliases: [],
      },
      source_ids: ["bilibili"],
      schedule: { interval_minutes: 60, retention_days: 7 },
      budget: { daily_requests: 192, content_purchase_cost: 0 },
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    },
    {
      id: secondMonitorId,
      title: "主题乙",
      state: "draft",
      current_version: 1,
      query_spec: {
        include_any: ["乙"],
        include_all: [],
        exclude: [],
        aliases: [],
      },
      source_ids: ["bilibili"],
      schedule: { interval_minutes: 60, retention_days: 7 },
      budget: { daily_requests: 192, content_purchase_cost: 0 },
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    },
  ];

  await page.route(/\/api\/monitors(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({ json: { items: monitors, next_cursor: null } });
  });
  await page.route(/\/api\/contents(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    const url = new URL(route.request().url());
    queries.push(new URLSearchParams(url.search));
    const requestedMonitor = url.searchParams.get("monitor_id");
    const requestedState = url.searchParams.get("review_state");
    const requestedSource = url.searchParams.get("source");
    const matches = [
      {
        id: firstMatchId,
        monitor_id: firstMonitorId,
        monitor_version_id: "00000000-0000-0000-0000-000000000606",
        monitor_version: 1,
        monitor_title: "主题甲",
        match_reason: ["甲"],
        relevance_status: "pending",
        review_state: states.get(firstMatchId),
        first_seen_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
      },
      {
        id: secondMatchId,
        monitor_id: secondMonitorId,
        monitor_version_id: "00000000-0000-0000-0000-000000000607",
        monitor_version: 1,
        monitor_title: "主题乙",
        match_reason: ["乙"],
        relevance_status: "accepted",
        review_state: states.get(secondMatchId),
        first_seen_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
      },
    ].filter(
      (match) =>
        (!requestedMonitor || match.monitor_id === requestedMonitor) &&
        (requestedState
          ? match.review_state === requestedState
          : match.review_state !== "ignored"),
    );
    await route.fulfill({
      json: {
        items:
          matches.length && (!requestedSource || requestedSource === "bilibili")
            ? [
                {
                  id: contentId,
                  source: "bilibili",
                  provider_namespace: "video",
                  external_id: "BV1Synthetic",
                  kind: "post",
                  root_external_id: "BV1Synthetic",
                  parent_external_id: null,
                  relation_status: "root",
                  text: "合成多主题内容",
                  version: 1,
                  canonical_url: "https://www.bilibili.com/video/BV1Synthetic",
                  published_at: new Date().toISOString(),
                  first_seen_at: new Date().toISOString(),
                  last_seen_at: new Date().toISOString(),
                  reply_count: 3,
                  matches,
                },
              ]
            : [],
        next_cursor: null,
      },
    });
  });
  await page.route(/\/api\/monitor-matches\/[^/?]+$/, async (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-1)!;
    const body = route.request().postDataJSON() as {
      review_state: "new" | "ignored";
    };
    writes.push({ path: new URL(route.request().url()).pathname, body });
    states.set(id, body.review_state);
    await route.fulfill({
      json: {
        id,
        monitor_id: id === firstMatchId ? firstMonitorId : secondMonitorId,
        monitor_version_id: "00000000-0000-0000-0000-000000000606",
        monitor_version: 1,
        monitor_title: id === firstMatchId ? "主题甲" : "主题乙",
        match_reason: [id === firstMatchId ? "甲" : "乙"],
        relevance_status: "pending",
        review_state: body.review_state,
        first_seen_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
      },
    });
  });

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const inbox = page.getByRole("region", { name: /监控收件箱/ });
  await expect(inbox.getByText("主题甲 · v1")).toBeVisible();
  await expect(inbox.getByText("主题乙 · v1")).toBeVisible();
  await inbox.getByRole("button", { name: "忽略 主题甲" }).click();
  await expect(inbox.getByText("主题甲 · v1")).toBeHidden();
  await expect(inbox.getByText("主题乙 · v1")).toBeVisible();

  await inbox.getByLabel("监控主题").selectOption(firstMonitorId);
  await expect(inbox.getByText("没有符合筛选条件的内容")).toBeVisible();
  await inbox.getByLabel("审核状态").selectOption("ignored");
  await expect(inbox.getByText("主题甲 · v1")).toBeVisible();
  await inbox.getByRole("button", { name: "恢复 主题甲" }).click();
  await expect(inbox.getByText("没有符合筛选条件的内容")).toBeVisible();

  await inbox.getByLabel("审核状态").selectOption("");
  await expect(inbox.getByText("主题甲 · v1")).toBeVisible();
  await inbox.getByLabel("发现时间").selectOption("24h");
  await expect.poll(() => queries.at(-1)?.get("discovered_since")).toBeTruthy();
  await inbox.getByLabel("来源").selectOption("x");
  await expect(inbox.getByText("没有符合筛选条件的内容")).toBeVisible();

  expect(writes).toEqual([
    {
      path: `/api/monitor-matches/${firstMatchId}`,
      body: { review_state: "ignored" },
    },
    {
      path: `/api/monitor-matches/${firstMatchId}`,
      body: { review_state: "new" },
    },
  ]);
});
