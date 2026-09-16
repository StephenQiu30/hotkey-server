import { test, expect } from "@playwright/test";
const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;
if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );
test("owner login, monitor edit, real diagnostic and revocation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();
  await expect(
    page.getByRole("heading", { name: "我的热点观察" }),
  ).toBeVisible();
  const sources = page.getByRole("region", { name: "来源能力" });
  await expect(sources.getByRole("heading", { name: "B站" })).toBeVisible();
  await expect(
    sources.getByText("技术可读 · 权限待核对 · 未连接").first(),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: /监控收件箱/ })).toBeVisible();
  await expect(
    page.getByRole("heading", { level: 2, name: /采集运行/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "还没有采集运行" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "还没有监控内容" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "还没有事件档案" }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: /知识库/ })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "还没有知识条目" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "还没有事件提醒" }),
  ).toBeVisible();
  const eventTitle = `事件验证-${Date.now()}`;
  await page.getByLabel("事件名称").fill(eventTitle);
  await page.getByLabel("简介").fill("人工整理的合成事件");
  await page.getByRole("button", { name: "创建事件" }).click();
  await expect(
    page.getByRole("heading", { name: eventTitle, exact: true }),
  ).toBeVisible();
  await expect(page.getByText("修订 v1 · 0 条内容")).toBeVisible();
  const eventCard = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: eventTitle, exact: true }),
  });
  await eventCard.getByRole("button", { name: "查看近 7 天趋势" }).click();
  await expect(
    eventCard.getByText("该事件还没有带原始证据的平台内容。"),
  ).toBeVisible();
  await expect(
    eventCard.getByRole("heading", { name: "评论观点样本" }),
  ).toBeVisible();
  await expect(eventCard.getByText("尚未冻结评论样本。")).toBeVisible();
  await eventCard.getByRole("button", { name: "冻结新样本" }).click();
  await expect(
    eventCard.getByText("当前事件在所选范围没有可分析评论。"),
  ).toBeVisible();
  const sourceEventTitle = `待合并-${Date.now()}`;
  await page.getByLabel("事件名称").fill(sourceEventTitle);
  await page.getByRole("button", { name: "创建事件" }).click();
  const sourceEvent = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: sourceEventTitle, exact: true }),
  });
  await sourceEvent
    .getByLabel(`选择 ${sourceEventTitle} 的合并目标`)
    .selectOption({ label: eventTitle });
  await sourceEvent.getByRole("button", { name: "合并到所选事件" }).click();
  await expect(sourceEvent.getByText(/已归档 · 修订 v2/)).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "事件提醒 2 未读" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "标为已读" }).first().click();
  await expect(
    page.getByRole("heading", { name: "事件提醒 1 未读" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "新建监控", exact: true }).click();
  await expect(page.getByLabel("每日请求上限")).toHaveValue("120");
  const title = `学习验证-${Date.now()}`;
  await page.getByLabel("监控名称").fill(title);
  await page
    .getByLabel("任一关键词（每行一个，最多 20 个）")
    .fill("人工智能\nAI");
  await page.getByLabel("必须同时包含").fill("监管");
  await page.getByLabel("排除词").fill("广告");
  await page.getByLabel("别名").fill("生成式AI");
  await expect(page.getByLabel("证据保留（天）")).toHaveValue("7");
  await page.getByLabel("微博", { exact: true }).check();
  await page.getByLabel("B站", { exact: true }).check();
  await page.getByRole("button", { name: "预览查询" }).click();
  await expect(page.getByRole("heading", { name: /查询预览/ })).toBeVisible();
  await expect(page.getByText("平台查询 · 入库前过滤").first()).toBeVisible();
  await page.getByRole("button", { name: "保存草稿" }).click();
  await expect(
    page.getByRole("heading", { name: title, exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: `编辑 ${title}` }).click();
  await page.getByLabel("监控名称").fill(title + "-更新");
  await page.getByRole("button", { name: "保存草稿" }).click();
  const card = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: title + "-更新", exact: true }),
  });
  await expect(card.getByText("草稿 · v2")).toBeVisible();
  await expect(card.getByText("来源未准入，暂不能启用。")).toBeVisible();
  await expect(
    page.getByRole("button", { name: `启用 ${title}-更新` }),
  ).toBeDisabled();
  let currentJob: Record<string, unknown> | null = null;
  let jobReads = 0;
  let runReads = 0;
  await page.route("**/api/jobs", async (route) => {
    if (route.request().method() === "POST") {
      const response = await route.fetch();
      currentJob = await response.json();
      await route.fulfill({ response });
      return;
    }
    jobReads += 1;
    const job = currentJob;
    if (job === null) {
      await route.fulfill({ json: { items: [], next_cursor: null } });
      return;
    }
    const active = jobReads === 1;
    await route.fulfill({
      json: {
        items: [
          {
            ...job,
            status: active ? "queued" : "succeeded",
            attempts: active ? 0 : 1,
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route("**/api/collection-runs?**", async (route) => {
    runReads += 1;
    if (currentJob === null) {
      await route.fulfill({ json: { items: [], next_cursor: null } });
      return;
    }
    const activeState = runReads === 1 ? "queued" : "running";
    const active = runReads <= 2;
    await route.fulfill({
      json: {
        items: [
          {
            id: "00000000-0000-0000-0000-000000000201",
            job_id: currentJob.id,
            parent_run_id: null,
            monitor_version_id: "00000000-0000-0000-0000-000000000202",
            source: "bilibili",
            operation: "search_posts",
            request_value: "轮询验证",
            retention_days: 7,
            trigger: "manual",
            ingestion_mode: "live",
            schedule_slot: null,
            budget_day: "2026-09-16",
            reserved_requests: 1,
            state: active ? activeState : "completed",
            outcome: active ? null : "ok",
            fencing_token: 1,
            pages_count: active ? 0 : 1,
            items_count: active ? 0 : 1,
            bytes_count: active ? 0 : 128,
            stop_reason: null,
            window_since: "2026-09-16T00:00:00Z",
            window_until: "2026-09-16T01:00:00Z",
            created_at: "2026-09-16T01:00:00Z",
            completed_at: active ? null : "2026-09-16T01:00:01Z",
          },
        ],
        next_cursor: null,
      },
    });
  });
  const jobResponse = page.waitForResponse(
    (r) => r.url().endsWith("/api/jobs") && r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "运行诊断" }).click();
  const job = await (await jobResponse).json();
  const jobRow = page.getByRole("row").filter({ hasText: job.id.slice(0, 8) });
  await expect(jobRow).toContainText("排队中");
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "hidden",
    });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  const hiddenReads = { jobs: jobReads, runs: runReads };
  await page.waitForTimeout(4500);
  expect({ jobs: jobReads, runs: runReads }).toEqual(hiddenReads);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      value: "visible",
    });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await expect(jobRow).toContainText("已完成", { timeout: 20000 });
  const collectionRun = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: "搜索：轮询验证" }),
  });
  await expect(collectionRun).toContainText("已完成", { timeout: 20000 });
  await page.waitForTimeout(4500);
  const settledReads = { jobs: jobReads, runs: runReads };
  await page.waitForTimeout(4500);
  expect({ jobs: jobReads, runs: runReads }).toEqual(settledReads);
  await page.reload();
  await expect(
    page.getByRole("heading", { name: title + "-更新", exact: true }),
  ).toBeVisible();
  const eventDossiers = page.getByRole("region", { name: /事件档案/ });
  await expect(
    eventDossiers.getByRole("heading", { name: eventTitle, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "事件提醒 1 未读" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/workspace-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({
    path: "test-results/workspace-desktop.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("button", { name: "登录工作台" })).toBeVisible();
  expect((await page.request.get("/api/monitors")).status()).toBe(401);
  expect(errors).toEqual([]);
});
