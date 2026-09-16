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
  expect(document.paths["/api/monitors"].get.operationId).toBe("listMonitors");
  expect(document.paths["/api/monitors/{identity}/runs"].post.operationId).toBe(
    "createCollectionRun",
  );
  expect(
    document.paths["/api/collection-runs/{identity}"].get.operationId,
  ).toBe("getCollectionRun");
  expect(document.paths["/api/collection-runs"].get.operationId).toBe(
    "listCollectionRuns",
  );
  expect(document.paths["/api/events"].post.operationId).toBe("createEvent");
  expect(
    document.paths["/api/events/{identity}/members"].post.operationId,
  ).toBe("addEventMember");
  expect(document.paths["/api/events/{identity}/merge"].post.operationId).toBe(
    "mergeEvent",
  );
  expect(document.paths["/api/events/{identity}/split"].post.operationId).toBe(
    "splitEvent",
  );
  expect(document.paths["/api/events/{identity}/trends"].get.operationId).toBe(
    "getEventTrends",
  );
  expect(
    document.paths["/api/events/{identity}/analysis-runs"].post.operationId,
  ).toBe("createEventAnalysisRun");
  expect(
    document.paths["/api/analysis-runs/{identity}/samples/{sample_id}/label"]
      .put.operationId,
  ).toBe("labelAnalysisSample");
  expect(
    document.paths["/api/analysis-runs/{identity}/recompute"].post.operationId,
  ).toBe("recomputeAnalysisRun");
  expect(
    document.paths["/api/analysis-runs/{identity}/knowledge-entry"].post
      .operationId,
  ).toBe("publishAnalysisKnowledge");
  expect(document.paths["/api/knowledge"].get.operationId).toBe(
    "searchKnowledge",
  );
  expect(document.paths["/api/knowledge/query"].post.operationId).toBe(
    "queryKnowledge",
  );
  expect(
    document.paths["/api/contents/{identity}/withdraw"].post.operationId,
  ).toBe("withdrawContent");
  expect(document.paths["/api/contents/{identity}"].get.operationId).toBe(
    "getContentDetail",
  );
  expect(
    document.paths["/api/monitor-matches/{identity}"].patch.operationId,
  ).toBe("reviewMonitorMatch");
  expect(document.paths["/api/knowledge/{identity}"].get.operationId).toBe(
    "getKnowledgeEntry",
  );
  expect(
    document.paths["/api/knowledge/{identity}/semantic-index"].post.operationId,
  ).toBe("indexKnowledgeEntry");
  expect(document.paths["/api/notifications"].get.operationId).toBe(
    "listNotifications",
  );
  expect(
    document.paths["/api/notifications/{identity}/read"].post.operationId,
  ).toBe("markNotificationRead");
  const capability = document.components.schemas.SourceOperationCapability;
  expect(capability.required).toContain("pipeline");
  expect(capability.required).toContain("eligible_for_collection");
  expect(capability.required).toContain("requires_operations");
  const runInput = document.components.schemas.MonitorRunRequest;
  const runBatch = document.components.schemas.CollectionRunBatchView;
  const runView = document.components.schemas.CollectionRunView;
  expect(runInput.required.sort()).toEqual([
    "expected_version",
    "idempotency_key",
  ]);
  expect(Object.keys(runInput.properties).sort()).toEqual([
    "expected_version",
    "idempotency_key",
  ]);
  expect(runBatch.required.sort()).toEqual(["items", "replayed"]);
  expect(runView.properties.operation.enum).toEqual([
    "search_posts",
    "fetch_post",
    "list_comments",
    "list_replies",
  ]);
  expect(runView.required).toContain("parent_run_id");
  const analysisRun = document.components.schemas.AnalysisRunView;
  expect(analysisRun.required).toContain("manifest_sha256");
  expect(analysisRun.properties.token_budget.const).toBe(0);
  expect(document.components.schemas.KnowledgeAnswer.required).toContain(
    "method",
  );
  expect(
    document.components.schemas.ControlledCommentStatistics.required,
  ).toContain("event_revision");
  expect(document.components.schemas.SourceView.properties.pipeline).toBe(
    undefined,
  );
});
