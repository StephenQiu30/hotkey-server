import { mkdtempSync, readFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  runBusinessFlowCapacity,
  validateBusinessFlowCapacityConfig,
  writeBusinessFlowCapacityReport,
} from "./business-flow-capacity.mjs";

describe("M4 business-flow capacity evidence", () => {
  it("measures report, notification, Vault and PostgreSQL search through the real HTTP contract", async () => {
    const fixture = newCapacityHTTPFixture();
    const config = validConfig();
    config.intervalMillis = 10_000;
    const sleeps = [];
    const report = await runBusinessFlowCapacity(config, {
      fetch: fixture.fetch,
      nowMicros: fixture.nowMicros,
      sleep: async (milliseconds) => { sleeps.push(milliseconds); },
    });

    expect(report).toMatchObject({
      version: "hotkey-m4-business-flow-capacity-v1",
      status: "measured",
      approval: "required",
      run_id: "m4-capacity-fixture",
      workload: { warmups: 1, samples: 3, concurrency: 1, cache_state: "warm", inter_flow_interval_millis: 10_000 },
      correlation: { strategy: "x-request-id", observed: 37, unique: 37 },
      errors: 0,
    });
    expect(Object.keys(report.stages)).toEqual([
      "report_build",
      "notification_visibility",
      "report_publication",
      "vault_publication",
      "search_visibility",
    ]);
    for (const stage of Object.values(report.stages)) {
      expect(stage.samples).toBe(3);
      expect(stage.duration_micros).toEqual([1000, 1000, 1000]);
      expect(stage.p50_micros).toBe(1000);
      expect(stage.p95_micros).toBe(1000);
      expect(stage.p99_micros).toBe(1000);
      expect(stage.errors).toBe(0);
    }
    expect(fixture.completedFlows()).toBe(4);
    expect(sleeps).toEqual([10_000, 10_000, 10_000]);
  });

  it("writes one exclusive sanitized artifact without persisting credentials or fixture sentinels", async () => {
    const fixture = newCapacityHTTPFixture();
    const config = validConfig();
    config.warmups = 0;
    config.samples = 1;
    const report = await runBusinessFlowCapacity(config, {
      fetch: fixture.fetch,
      nowMicros: fixture.nowMicros,
    });
    const root = mkdtempSync(join(tmpdir(), "hotkey-m4-capacity-"));
    const output = join(root, "capacity.json");

    writeBusinessFlowCapacityReport(output, report, [
      config.email,
      config.password,
      "HOTKEY_BODY_SENTINEL_fixture",
      "/Users/private/vault/report.md",
    ]);

    const saved = readFileSync(output, "utf8");
    expect(JSON.parse(saved)).toMatchObject({ version: "hotkey-m4-business-flow-capacity-v1", errors: 0 });
    expect(saved).not.toContain(config.email);
    expect(saved).not.toContain(config.password);
    expect(saved).not.toContain("HOTKEY_BODY_SENTINEL_fixture");
    expect(saved).not.toContain("/Users/private/vault/report.md");
    expect(statSync(output).mode & 0o777).toBe(0o600);
    expect(() => writeBusinessFlowCapacityReport(output, report, [])).toThrow();
  });

  it("fails closed without an isolated environment, disabled production egress, fixed revision or bounded samples", () => {
    for (const mutate of [
      (config) => { config.confirmIsolated = false; },
      (config) => { config.productionEgressDisabled = false; },
      (config) => { config.environment = "production"; },
      (config) => { config.gitRevision = "main"; },
      (config) => { config.samples = 0; },
      (config) => { config.samples = 101; },
      (config) => { config.intervalMillis = 60_001; },
      (config) => { config.runID = "unsafe run id"; },
    ]) {
      const config = validConfig();
      mutate(config);
      expect(() => validateBusinessFlowCapacityConfig(config)).toThrow();
    }
  });
});

function validConfig() {
  return {
    apiOrigin: "http://127.0.0.1:8866",
    email: "capacity-editor@example.test",
    password: "fixture-password-0123456789abcdef",
    monitorID: 910001,
    runID: "m4-capacity-fixture",
    environment: "isolated-fixture",
    hardware: "4 cpu 8 GiB local SSD",
    gitRevision: "0123456789abcdef0123456789abcdef01234567",
    confirmIsolated: true,
    productionEgressDisabled: true,
    warmups: 1,
    samples: 3,
    intervalMillis: 0,
  };
}

function newCapacityHTTPFixture() {
  let micros = 0;
  let reportID = 100;
  let currentReportID = 0;
  let currentDocumentID = 0;
  let currentProposalID = 0;
  let completed = 0;

  const nowMicros = () => {
    micros += 1000;
    return micros;
  };
  const fetch = async (input, init = {}) => {
    const url = new URL(input);
    const method = init.method ?? "GET";
    const requestID = init.headers?.["X-Request-ID"];
    let data;
    if (url.pathname === "/api/v1/auth/login" && method === "POST") {
      data = { access_token: "fixture-access-token-0123456789abcdef" };
    } else if (url.pathname === "/api/v1/reports" && method === "POST") {
      currentReportID = ++reportID;
      data = { id: currentReportID, version: 1, status: "draft", title: `日报 #${currentReportID}` };
    } else if (url.pathname === `/api/v1/reports/${currentReportID}/submit` && method === "POST") {
      data = { id: currentReportID, version: 2, status: "pending_approval" };
    } else if (url.pathname === "/api/v1/notifications" && method === "GET") {
      data = { items: [{ resource_type: "report", resource_id: currentReportID, event_type: "report.approval_requested", deep_link: `/dashboard/reports?report=${currentReportID}` }] };
    } else if (url.pathname === `/api/v1/reports/${currentReportID}/approve` && method === "POST") {
      currentDocumentID = currentReportID + 1000;
      currentProposalID = currentReportID + 2000;
      data = { id: currentReportID, version: 3, status: "published", frozen: true };
    } else if (url.pathname === "/api/v1/knowledge/documents" && method === "GET") {
      data = { items: [{ id: currentDocumentID, reportID: currentReportID, status: "planned" }] };
    } else if (url.pathname === "/api/v1/knowledge/proposals" && method === "GET") {
      data = { items: [{ id: currentProposalID, version: 1, documentID: currentDocumentID, status: "pending" }] };
    } else if (url.pathname === `/api/v1/knowledge/proposals/${currentProposalID}/approve` && method === "POST") {
      data = { id: currentProposalID, version: 2, documentID: currentDocumentID, status: "approved" };
    } else if (url.pathname === `/api/v1/knowledge/proposals/${currentProposalID}/apply` && method === "POST") {
      data = { id: currentDocumentID, reportID: currentReportID, status: "active", revisionNo: 1 };
    } else if (url.pathname === "/api/v1/search" && method === "GET") {
      completed++;
      data = { items: [{ type: "knowledge", id: currentDocumentID, status: "active" }] };
    } else {
      return response(404, null, requestID);
    }
    return response(200, data, requestID);
  };
  return { fetch, nowMicros, completedFlows: () => completed };
}

function response(status, data, requestID) {
  return new Response(JSON.stringify({ code: status === 200 ? 0 : 404, data }), {
    status,
    headers: { "content-type": "application/json", "x-request-id": requestID ?? "generated-request-id" },
  });
}
