import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;
if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("knowledge search exposes explicit semantic mode and unavailable state", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const knowledge = page.getByRole("region", { name: "知识库" });
  await knowledge.getByLabel("检索方式").selectOption("semantic");
  await knowledge.getByLabel("搜索知识条目").fill("release outage");
  await knowledge.getByRole("button", { name: "语义检索" }).click();
  await expect(knowledge.getByRole("alert")).toHaveText(
    "尚未配置自建语义模型服务。",
  );

  const questions = knowledge.getByRole("form", { name: "受控问答" });
  await questions.getByLabel("问题类型").selectOption("evidence");
  await questions
    .getByRole("textbox", { name: "问题", exact: true })
    .fill("完全不存在的证据");
  await questions.getByRole("button", { name: "提交问题" }).click();
  await expect(questions.getByText("依据不足")).toBeVisible();
  await expect(
    questions.getByText("当前有效证据中没有找到可支持该问题的内容。"),
  ).toBeVisible();

  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
