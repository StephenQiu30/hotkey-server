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
  await page.getByRole("button", { name: "新建监控", exact: true }).click();
  await expect(page.getByLabel("每日请求上限")).toHaveValue("96");
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
  const jobResponse = page.waitForResponse(
    (r) => r.url().endsWith("/api/v1/jobs") && r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "运行诊断" }).click();
  const job = await (await jobResponse).json();
  await expect
    .poll(
      async () => {
        await page.getByRole("button", { name: "刷新", exact: true }).click();
        return page
          .getByRole("row")
          .filter({ hasText: job.id.slice(0, 8) })
          .textContent();
      },
      { timeout: 20000 },
    )
    .toContain("已完成");
  await page.reload();
  await expect(
    page.getByRole("heading", { name: title + "-更新", exact: true }),
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
  expect((await page.request.get("/api/v1/monitors")).status()).toBe(401);
  expect(errors).toEqual([]);
});
