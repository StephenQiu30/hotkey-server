import { mkdtempSync, readFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";
import {
  runAvailabilityRehearsal,
  validateAvailabilityConfig,
  writeAvailabilityReport,
} from "./availability-rehearsal.mjs";

describe("single-host availability rehearsal", () => {
  it("measures fixed minute samples and attributes each dependency fault", async () => {
    const transitions = [];
    let scenario = "baseline";
    let elapsedMillis = 0;
    const result = await runAvailabilityRehearsal(validConfig(), {
      minuteDurationMillis: 600,
      transition: vi.fn(async (next) => {
        scenario = next;
        transitions.push(next);
      }),
      probe: vi.fn(async () => ({
        http_status: scenario === "postgres_unavailable" ? 503 : 200,
        business_code: scenario === "postgres_unavailable" ? 90001 : 0,
        duration_millis: 2,
      })),
      sleep: vi.fn(async (duration) => {
        elapsedMillis += duration;
      }),
      now: () => new Date(Date.UTC(2026, 7, 30, 4, 0, 0) + elapsedMillis),
    });

    expect(transitions).toEqual([
      "baseline",
      "postgres_unavailable",
      "postgres_recovered",
      "redis_unavailable",
      "minio_unavailable",
      "worker_unavailable",
      "full_recovery",
    ]);
    expect(result).toMatchObject({
      version: "hotkey-single-host-availability-rehearsal-v1",
      status: "measured",
      approval: "required",
      release_ready: false,
      production_egress_disabled: true,
      scope: "candidate_methodology_not_production_sla",
      measurement: {
        probe: "/readyz",
        aggregation: "conservative_all_probes_per_observation_minute",
        maintenance_window: "none",
        excluded_minutes: 0,
        available_minutes: 6,
        unavailable_minutes: 1,
        availability_percent: 85.714286,
      },
      error_budget: {
        candidate_target_percent: 99.5,
        calendar_window_days: 30,
        budget_minutes: 216,
        injected_unavailable_minutes: 1,
        remaining_budget_minutes: 215,
        target_met_in_injected_window: false,
      },
    });
    expect(result.samples).toHaveLength(7);
    expect(result.samples.every((sample) => sample.probes.length === 6)).toBe(true);
    expect(result.samples.find((sample) => sample.scenario === "postgres_unavailable")).toMatchObject({
      dependency: "postgresql",
      expected_api_available: false,
      observed_api_available: false,
      attributed: true,
    });
    for (const name of ["redis_unavailable", "minio_unavailable", "worker_unavailable"]) {
      expect(result.samples.find((sample) => sample.scenario === name)).toMatchObject({
        expected_api_available: true,
        observed_api_available: true,
        attributed: true,
      });
    }
    expect(result.differences).toEqual([]);
  });

  it("fails when observed degradation does not match the fixed contract", async () => {
    let scenario = "baseline";
    await expect(runAvailabilityRehearsal(validConfig(), {
      minuteDurationMillis: 60,
      transition: async (next) => { scenario = next; },
      probe: async () => ({
        http_status: scenario === "redis_unavailable" ? 503 : scenario === "postgres_unavailable" ? 503 : 200,
        business_code: scenario === "redis_unavailable" || scenario === "postgres_unavailable" ? 90001 : 0,
        duration_millis: 1,
      }),
      sleep: async () => {},
      now: steppedClock(),
    })).rejects.toThrow("redis_unavailable availability mismatch");
  });

  it("rejects production, the formal project, and incomplete reproducibility inputs", () => {
    expect(() => validateAvailabilityConfig({ ...validConfig(), environment: "production" })).toThrow("isolated");
    expect(() => validateAvailabilityConfig({ ...validConfig(), composeProject: "hotkey" })).toThrow("formal");
    expect(() => validateAvailabilityConfig({ ...validConfig(), gitRevision: "short" })).toThrow("revision");
    expect(() => validateAvailabilityConfig({ ...validConfig(), productionEgressDisabled: false })).toThrow("egress");
  });

  it("writes private immutable evidence without protected values", () => {
    const root = mkdtempSync(join(tmpdir(), "hotkey-availability-"));
    const output = join(root, "availability.json");
    const result = {
      version: "hotkey-single-host-availability-rehearsal-v1",
      status: "measured",
      differences: [],
    };
    writeAvailabilityReport(output, result, ["secret-canary"]);
    expect(JSON.parse(readFileSync(output, "utf8"))).toEqual(result);
    expect(statSync(output).mode & 0o777).toBe(0o600);
    expect(() => writeAvailabilityReport(output, result, [])).toThrow();
    expect(() => writeAvailabilityReport(join(root, "leak.json"), { value: "secret-canary" }, ["secret-canary"])).toThrow("protected");
  });
});

function validConfig() {
  return {
    apiOrigin: "http://127.0.0.1:8866",
    composeFile: "/tmp/hotkey-isolated/docker-compose.yml",
    composeProject: "hotkey-ci",
    apiContainer: "hotkey-ci-availability-api",
    workerContainer: "hotkey-ci-availability-worker",
    environment: "github-actions-isolated-availability",
    hardware: "ubuntu-latest; 4 CPU; 16 GiB RAM; Docker loopback",
    gitRevision: "0123456789abcdef0123456789abcdef01234567",
    runID: "availability-123-1",
    probesPerMinute: 6,
    productionEgressDisabled: true,
  };
}

function steppedClock() {
  let current = Date.UTC(2026, 7, 30, 4, 0, 0);
  return () => {
    current += 10;
    return new Date(current);
  };
}
