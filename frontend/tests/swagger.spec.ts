import { expect, test } from "@playwright/test";

const apiURL = process.env.HOTKEY_API_URL ?? "http://localhost:8867";

test("Swagger UI renders the FastAPI generated contract", async ({ page }) => {
  const response = await page.goto(`${apiURL}/docs`);
  expect(response?.ok()).toBe(true);
  await expect(page.locator(".swagger-ui")).toBeVisible();
  await expect(page.locator(".info .title")).toContainText("HotKey API");

  const contract = await page.request.get(`${apiURL}/openapi.json`);
  expect(contract.ok()).toBe(true);
  const document = await contract.json();
  expect(document.info.title).toBe("HotKey API");
  expect(document.paths["/api/v1/monitors"].get.operationId).toBe(
    "listMonitors",
  );
  expect(
    document.paths["/api/v1/monitors/{identity}/runs"].post.operationId,
  ).toBe("createCollectionRun");
  expect(
    document.paths["/api/v1/collection-runs/{identity}"].get.operationId,
  ).toBe("getCollectionRun");
  expect(document.paths["/api/v1/collection-runs"].get.operationId).toBe(
    "listCollectionRuns",
  );
});
