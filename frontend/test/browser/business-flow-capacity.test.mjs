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
      observeRuntime: runtimeObserverFixture(),
    });

    expect(report).toMatchObject({
      version: "hotkey-m4-business-flow-capacity-v1",
      status: "measured",
      approval: "required",
      run_id: "m4-capacity-fixture",
      workload: { warmups: 1, samples: 3, concurrency: 1, cache_state: "warm", inter_flow_interval_millis: 10_000 },
      correlation: { strategy: "x-request-id", observed: 37, unique: 37 },
      runtime_observation: {
        strategy: "after_each_measured_stage",
        samples: 15,
        resources: {
          api: { samples: 15, memory_limit_bytes: 17179869184, max_cpu_percent: 13.5, max_memory_used_bytes: 134217728, max_memory_percent: 0.8, max_pids: 18 },
        },
        projection_backlog: {
          notification: { samples: 15, max_available: 2, max_running: 1, max_lag_seconds: 3.5, final_available: 0, final_running: 0, final_lag_seconds: 0 },
          report: { samples: 15, max_available: 0, max_running: 0, max_lag_seconds: 0, final_available: 0, final_running: 0, final_lag_seconds: 0 },
          vault: { samples: 15, max_available: 1, max_running: 1, max_lag_seconds: 1.25, final_available: 0, final_running: 0, final_lag_seconds: 0 },
          search: { samples: 15, max_available: 1, max_running: 0, max_lag_seconds: 0.5, final_available: 0, final_running: 0, final_lag_seconds: 0 },
        },
      },
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
    expect(report.runtime_observation.raw_samples).toHaveLength(15);
    expect(report.runtime_observation.raw_samples.at(-1)).toMatchObject({
      sequence: 15,
      stage: "search_visibility",
      projection_backlog: { notification: { available: 0, running: 0, lag_seconds: 0 } },
    });
  });

  it("writes one exclusive sanitized artifact without persisting credentials or fixture sentinels", async () => {
    const fixture = newCapacityHTTPFixture();
    const config = validConfig();
    config.warmups = 0;
    config.samples = 1;
    const report = await runBusinessFlowCapacity(config, {
      fetch: fixture.fetch,
      nowMicros: fixture.nowMicros,
      observeRuntime: runtimeObserverFixture(),
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
      (config) => { config.filesystem = ""; },
    ]) {
      const config = validConfig();
      mutate(config);
      expect(() => validateBusinessFlowCapacityConfig(config)).toThrow();
    }
  });

  it("fails closed when required resource or projection-backlog observations are unavailable", async () => {
    const fixture = newCapacityHTTPFixture();
    const config = validConfig();
    config.warmups = 0;
    config.samples = 1;

    await expect(runBusinessFlowCapacity(config, {
      fetch: fixture.fetch,
      nowMicros: fixture.nowMicros,
    })).rejects.toThrow(/runtime observer/i);

    await expect(runBusinessFlowCapacity(config, {
      fetch: newCapacityHTTPFixture().fetch,
      nowMicros: newCapacityHTTPFixture().nowMicros,
      observeRuntime: async () => ({ resources: {}, projection_backlog: {} }),
    })).rejects.toThrow(/runtime observation/i);
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
    filesystem: "ext4 workspace; overlay2 Docker storage",
    gitRevision: "0123456789abcdef0123456789abcdef01234567",
    confirmIsolated: true,
    productionEgressDisabled: true,
    warmups: 1,
    samples: 3,
    intervalMillis: 0,
  };
}

function runtimeObserverFixture() {
  let sequence = 0;
  return async () => {
    sequence++;
    const final = sequence === 15;
    return {
      resources: {
        api: {
          cpu_percent: 12.5 + sequence / 15,
          memory_used_bytes: sequence === 15 ? 134217728 : 125829120 + sequence * 500000,
          memory_limit_bytes: 17179869184,
          memory_percent: 0.7 + sequence / 150,
          pids: 17 + (sequence === 8 ? 1 : 0),
        },
      },
      projection_backlog: {
        notification: { available: final ? 0 : sequence === 3 ? 2 : 1, running: final ? 0 : sequence === 4 ? 1 : 0, lag_seconds: final ? 0 : sequence === 3 ? 3.5 : 0.25 },
        report: { available: 0, running: 0, lag_seconds: 0 },
        vault: { available: final ? 0 : sequence === 8 ? 1 : 0, running: final ? 0 : sequence === 9 ? 1 : 0, lag_seconds: final ? 0 : sequence === 8 ? 1.25 : 0 },
        search: { available: final ? 0 : sequence === 12 ? 1 : 0, running: 0, lag_seconds: final ? 0 : sequence === 12 ? 0.5 : 0 },
      },
    };
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
