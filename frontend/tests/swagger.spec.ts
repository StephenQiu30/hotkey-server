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
  const capability = document.components.schemas.SourceOperationCapability;
  expect(capability.required).toContain("pipeline");
  expect(capability.required).toContain("eligible_for_collection");
  expect(capability.required).toContain("requires_operations");
  const runInput = document.components.schemas.CollectionRunRequest;
  const runView = document.components.schemas.CollectionRunView;
  expect(runInput.required).toContain("request_value");
  expect(runInput.properties.query_variant).toBe(undefined);
  expect(runView.properties.operation.enum).toEqual([
    "search_posts",
    "fetch_post",
  ]);
  expect(runView.required).toContain("parent_run_id");
  expect(document.components.schemas.SourceView.properties.pipeline).toBe(
    undefined,
  );
});
