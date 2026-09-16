import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;

if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("content discussion loads generated detail pages only after user action", async ({
  page,
}) => {
  const contentId = "00000000-0000-0000-0000-000000000701";
  const rootId = "00000000-0000-0000-0000-000000000702";
  const commentId = "00000000-0000-0000-0000-000000000703";
  const replyId = "00000000-0000-0000-0000-000000000704";
  const now = new Date().toISOString();
  const requests: string[] = [];
  const item = (
    id: string,
    externalId: string,
    kind: "post" | "comment" | "reply",
    text: string,
    parentExternalId: string | null,
  ) => ({
    id,
    source: "bilibili",
    provider_namespace: kind === "post" ? "video" : "comment",
    external_id: externalId,
    kind,
    root_external_id: "BV1Discussion",
    parent_external_id: parentExternalId,
    relation_status: kind === "post" ? "root" : "resolved",
    text,
    version: 1,
    canonical_url: null,
    published_at: now,
    first_seen_at: now,
    last_seen_at: now,
    reply_count: kind === "post" ? 2 : 0,
  });
  const selected = item(
    contentId,
    "reply:selected",
    "reply",
    "选中的回复",
    "comment:1",
  );
  const root = item(rootId, "BV1Discussion", "post", "根帖正文", null);
  const parent = item(commentId, "comment:1", "comment", "根评论正文", null);
  const reply = item(replyId, "reply:2", "reply", "回复正文", "comment:1");

  await page.route(/\/api\/contents(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({
      json: {
        items: [
          {
            ...selected,
            matches: [],
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route(
    new RegExp(`/api/contents/${contentId}(?:\\?.*)?$`),
    async (route) => {
      const url = new URL(route.request().url());
      requests.push(`${url.pathname}?${url.searchParams.toString()}`);
      const cursor = url.searchParams.get("cursor");
      await route.fulfill({
        json: {
          selected,
          root,
          parent,
          discussion: cursor ? [reply] : [parent],
          next_cursor: cursor ? null : "next-page",
        },
      });
    },
  );

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const inbox = page.getByRole("region", { name: /监控收件箱/ });
  await expect(inbox.getByText("选中的回复")).toBeVisible();
  expect(requests).toEqual([]);

  await inbox.getByRole("button", { name: "查看评论上下文" }).click();
  const discussion = inbox.getByRole("region", {
    name: "reply:selected 的评论上下文",
  });
  await expect(discussion.getByText("根帖正文")).toBeVisible();
  await expect(discussion.getByText("父级评论")).toBeVisible();
  await expect(
    discussion.getByText("根评论正文", { exact: true }),
  ).toBeVisible();
  await discussion.getByRole("button", { name: "更多评论" }).click();
  await expect(discussion.getByText("回复正文")).toBeVisible();

  expect(requests).toEqual([
    `/api/contents/${contentId}?limit=20`,
    `/api/contents/${contentId}?limit=20&cursor=next-page`,
  ]);
});
