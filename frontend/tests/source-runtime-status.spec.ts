import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;
if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("source capability shows degraded runtime and a recovery action", async ({
  page,
}) => {
  await page.route("**/api/sources", async (route) => {
    const response = await route.fetch();
    const sources = (await response.json()) as API.SourceView[];
    const bilibili = sources.find((source) => source.id === "bilibili");
    const comments = bilibili?.operations.find(
      (operation) => operation.operation === "list_comments",
    );
    if (!comments) throw new Error("Bilibili comments capability is missing");
    comments.runtime = {
      status: "degraded",
      last_success_at: "2026-09-16T10:00:00Z",
      last_failure_at: "2026-09-16T11:00:00Z",
      last_failure_code: "access_denied",
      recovery_action: "refresh_authorization",
    };
    await route.fulfill({ response, json: sources });
  });

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const sources = page.getByRole("region", { name: "来源能力" });
  const bilibili = sources.getByRole("article").filter({
    has: page.getByRole("heading", { name: "B站", exact: true }),
  });
  const comments = bilibili.getByRole("listitem").filter({
    hasText: "根评论",
  });
  await expect(comments).toContainText("运行异常 · 授权被拒绝");
  await expect(comments).toContainText("恢复：更新授权后执行有界重试");
  await expect(
    bilibili
      .getByRole("listitem")
      .filter({ hasText: "关键词发现" })
      .getByText("尚无运行记录"),
  ).toBeVisible();

  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
