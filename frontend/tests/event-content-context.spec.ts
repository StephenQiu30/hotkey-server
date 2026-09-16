import { expect, test } from "@playwright/test";

const username = process.env.HOTKEY_E2E_USERNAME;
const password = process.env.HOTKEY_E2E_PASSWORD;

if (!username || !password)
  throw new Error(
    "Set synthetic owner credentials for a disposable Compose stack",
  );

test("event member opens persisted discussion through the generated content API", async ({
  page,
}) => {
  const eventId = "00000000-0000-0000-0000-000000000801";
  const selectedId = "00000000-0000-0000-0000-000000000802";
  const rootId = "00000000-0000-0000-0000-000000000803";
  const parentId = "00000000-0000-0000-0000-000000000804";
  const now = new Date().toISOString();
  const detailRequests: string[] = [];
  const detailItem = (
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
    root_external_id: "BV1EventContext",
    parent_external_id: parentExternalId,
    relation_status: kind === "post" ? "root" : "resolved",
    text,
    version: 1,
    canonical_url: null,
    published_at: now,
    first_seen_at: now,
    last_seen_at: now,
    reply_count: 0,
  });
  const selected = detailItem(
    selectedId,
    "reply:event",
    "reply",
    "事件中的回复",
    "comment:event",
  );
  const root = detailItem(rootId, "BV1EventContext", "post", "事件根帖", null);
  const parent = detailItem(
    parentId,
    "comment:event",
    "comment",
    "事件父评论",
    null,
  );

  await page.route(/\/api\/events(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    await route.fulfill({
      json: {
        items: [
          {
            id: eventId,
            title: "合成事件",
            summary: "用于验证事件评论上下文",
            status: "active",
            current_revision: 2,
            created_at: now,
            updated_at: now,
            members: [
              {
                content_id: selectedId,
                source: "bilibili",
                kind: "reply",
                external_id: "reply:event",
                canonical_url: null,
                added_at: now,
              },
            ],
          },
        ],
        next_cursor: null,
      },
    });
  });
  await page.route(
    new RegExp(`/api/contents/${selectedId}(?:\\?.*)?$`),
    async (route) => {
      const url = new URL(route.request().url());
      detailRequests.push(`${url.pathname}?${url.searchParams.toString()}`);
      await route.fulfill({
        json: {
          selected,
          root,
          parent,
          discussion: [parent, selected],
          next_cursor: null,
        },
      });
    },
  );

  await page.goto("/");
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录工作台" }).click();

  const dossier = page
    .getByRole("article")
    .filter({ has: page.getByRole("heading", { name: "合成事件" }) });
  await expect(dossier).toBeVisible();
  expect(detailRequests).toEqual([]);

  await dossier.getByRole("button", { name: "查看评论上下文" }).click();
  const discussion = dossier.getByRole("region", {
    name: "reply:event 的评论上下文",
  });
  await expect(discussion.getByText("事件根帖")).toBeVisible();
  await expect(
    discussion.getByText("事件父评论", { exact: true }),
  ).toBeVisible();
  await expect(discussion.getByText("事件中的回复")).toBeVisible();
  await expect(dossier.getByRole("button", { name: "移出" })).toBeEnabled();
  expect(detailRequests).toEqual([`/api/contents/${selectedId}?limit=20`]);
});
