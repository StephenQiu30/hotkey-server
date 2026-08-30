import { describe, expect, it } from "vitest";
import {
  createBusinessFlowRuntimeObserver,
  parseDockerResourceSamples,
  parseProjectionBacklogMetrics,
} from "./business-flow-runtime-observation.mjs";

describe("M4 business-flow runtime observation", () => {
  it("collects bounded service resources and four projection backlogs without retaining container names or raw metrics", async () => {
    const observer = createBusinessFlowRuntimeObserver({
      apiOrigin: "http://127.0.0.1:8866",
      containerPrefix: "hotkey-ci-fixture",
    }, {
      execFile: async () => ({ stdout: dockerStatsFixture() }),
      fetch: async () => new Response(metricsFixture(), { status: 200 }),
    });

    const sample = await observer();
    expect(sample).toEqual({
      resources: {
        agent: { cpu_percent: 1.5, memory_used_bytes: 67108864, memory_limit_bytes: 17179869184, memory_percent: 0.39, pids: 4 },
        api: { cpu_percent: 12.25, memory_used_bytes: 134217728, memory_limit_bytes: 17179869184, memory_percent: 0.78, pids: 18 },
        minio: { cpu_percent: 0.3, memory_used_bytes: 100663296, memory_limit_bytes: 17179869184, memory_percent: 0.59, pids: 11 },
        postgres: { cpu_percent: 4.75, memory_used_bytes: 268435456, memory_limit_bytes: 17179869184, memory_percent: 1.56, pids: 14 },
        redis: { cpu_percent: 0.2, memory_used_bytes: 16777216, memory_limit_bytes: 17179869184, memory_percent: 0.1, pids: 6 },
        web: { cpu_percent: 2.5, memory_used_bytes: 83886080, memory_limit_bytes: 17179869184, memory_percent: 0.49, pids: 12 },
      },
      projection_backlog: {
        notification: { available: 3, running: 1, lag_seconds: 4.5 },
        report: { available: 2, running: 0, lag_seconds: 2.25 },
        vault: { available: 1, running: 1, lag_seconds: 1.75 },
        search: { available: 4, running: 2, lag_seconds: 6.5 },
      },
    });
    expect(JSON.stringify(sample)).not.toContain("hotkey-ci-fixture-api");
    expect(JSON.stringify(sample)).not.toContain("# HELP");
  });

  it("parses IEC and decimal Docker units and aggregates only approved projection job kinds", () => {
    const resources = parseDockerResourceSamples(dockerStatsFixture(), "hotkey-ci-fixture");
    expect(resources.api.max).toBeUndefined();
    expect(resources.api.memory_used_bytes).toBe(128 * 1024 * 1024);

    const rounded = parseDockerResourceSamples(
      dockerStatsFixture().replace("128MiB / 16GiB", "127.5MiB / 15.999GiB"),
      "hotkey-ci-fixture",
    );
    expect(rounded.api.memory_used_bytes).toBe(Math.round(127.5 * 1024 ** 2));
    expect(rounded.api.memory_limit_bytes).toBe(Math.round(15.999 * 1024 ** 3));

    const backlog = parseProjectionBacklogMetrics(metricsFixture());
    expect(backlog.notification).toEqual({ available: 3, running: 1, lag_seconds: 4.5 });
    expect(backlog.search).toEqual({ available: 4, running: 2, lag_seconds: 6.5 });
  });

  it("fails closed for formal containers, incomplete stats, failed metric collection or malformed numbers", async () => {
    expect(() => createBusinessFlowRuntimeObserver({ apiOrigin: "http://127.0.0.1:8866", containerPrefix: "hotkey" })).toThrow();
    expect(() => createBusinessFlowRuntimeObserver({ apiOrigin: "https://example.com", containerPrefix: "hotkey-ci-fixture" })).toThrow();
    expect(() => parseDockerResourceSamples(dockerStatsFixture().split("\n").slice(0, 5).join("\n"), "hotkey-ci-fixture")).toThrow();
    expect(() => parseProjectionBacklogMetrics(metricsFixture().replace("hotkey_runtime_metrics_collection_success 1", "hotkey_runtime_metrics_collection_success 0"))).toThrow();
    expect(() => parseProjectionBacklogMetrics(metricsFixture().replace(" 6.5", " NaN"))).toThrow();
  });
});

function dockerStatsFixture() {
  return [
    stat("agent", "1.50%", "64MiB / 16GiB", "0.39%", "4"),
    stat("api", "12.25%", "128MiB / 16GiB", "0.78%", "18"),
    stat("minio", "0.30%", "96MiB / 16GiB", "0.59%", "11"),
    stat("postgres", "4.75%", "256MiB / 16GiB", "1.56%", "14"),
    stat("redis", "0.20%", "16MiB / 16GiB", "0.10%", "6"),
    stat("web", "2.50%", "80MiB / 16GiB", "0.49%", "12"),
  ].join("\n");
}

function stat(service, cpu, memory, memoryPercent, pids) {
  return JSON.stringify({ Name: `hotkey-ci-fixture-${service}`, CPUPerc: cpu, MemUsage: memory, MemPerc: memoryPercent, PIDs: pids });
}

function metricsFixture() {
  return `# HELP hotkey_job_runs_total retained jobs
# TYPE hotkey_job_runs_total counter
hotkey_job_runs_total{kind="deliver_email",state="available"} 2
hotkey_job_runs_total{kind="project_user_notification",state="available"} 1
hotkey_job_runs_total{kind="deliver_alert_email",state="running"} 1
hotkey_job_runs_total{kind="build_report",state="available"} 2
hotkey_job_runs_total{kind="project_knowledge",state="available"} 1
hotkey_job_runs_total{kind="reconcile_knowledge",state="running"} 1
hotkey_job_runs_total{kind="generate_source_document",state="available"} 4
hotkey_job_runs_total{kind="generate_source_document",state="running"} 2
hotkey_job_runs_total{kind="normalize_content",state="available"} 99
hotkey_job_queue_lag_seconds{kind="deliver_email"} 4.5
hotkey_job_queue_lag_seconds{kind="project_user_notification"} 2
hotkey_job_queue_lag_seconds{kind="build_report"} 2.25
hotkey_job_queue_lag_seconds{kind="project_knowledge"} 1.75
hotkey_job_queue_lag_seconds{kind="generate_source_document"} 6.5
hotkey_job_queue_lag_seconds{kind="normalize_content"} 500
hotkey_runtime_metrics_collection_success 1
`;
}
