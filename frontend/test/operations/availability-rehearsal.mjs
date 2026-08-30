import { basename, isAbsolute } from "node:path";
import { writeFileSync } from "node:fs";

const version = "hotkey-single-host-availability-rehearsal-v1";
const gitRevisionPattern = /^[0-9a-f]{40}$/;
const safeNamePattern = /^[a-z0-9][a-z0-9_-]{1,62}$/;
const runIDPattern = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$/;
const defaultMinuteDurationMillis = 60_000;
const candidateTargetPercent = 99.5;
const calendarWindowDays = 30;

const scenarios = Object.freeze([
  Object.freeze({ name: "baseline", dependency: "none", expectedAPIAvailable: true }),
  Object.freeze({ name: "postgres_unavailable", dependency: "postgresql", expectedAPIAvailable: false }),
  Object.freeze({ name: "postgres_recovered", dependency: "postgresql", expectedAPIAvailable: true }),
  Object.freeze({ name: "redis_unavailable", dependency: "redis", expectedAPIAvailable: true }),
  Object.freeze({ name: "minio_unavailable", dependency: "minio", expectedAPIAvailable: true }),
  Object.freeze({ name: "worker_unavailable", dependency: "worker", expectedAPIAvailable: true }),
  Object.freeze({ name: "full_recovery", dependency: "all", expectedAPIAvailable: true }),
]);

export function validateAvailabilityConfig(config) {
  if (config == null || typeof config !== "object") throw new Error("availability configuration is required");

  let origin;
  try {
    origin = new URL(config.apiOrigin);
  } catch {
    throw new Error("availability API origin is invalid");
  }
  if (origin.protocol !== "http:" || !["127.0.0.1", "localhost", "[::1]"].includes(origin.hostname) ||
      origin.pathname !== "/" || origin.username !== "" || origin.password !== "" || origin.search !== "" || origin.hash !== "") {
    throw new Error("availability API must be an isolated loopback HTTP endpoint");
  }
  if (typeof config.composeFile !== "string" || !isAbsolute(config.composeFile) || basename(config.composeFile) !== "docker-compose.yml") {
    throw new Error("availability compose file must be an absolute docker-compose.yml path");
  }
  if (!safeNamePattern.test(config.composeProject ?? "")) {
    throw new Error("availability compose project is invalid");
  }
  if (config.composeProject === "hotkey") {
    throw new Error("availability rehearsal cannot target the formal Compose project");
  }
  if (config.apiContainer !== `${config.composeProject}-availability-api` ||
      config.workerContainer !== `${config.composeProject}-availability-worker` ||
      !safeNamePattern.test(config.apiContainer) || !safeNamePattern.test(config.workerContainer)) {
    throw new Error("availability rehearsal requires exact isolated container names");
  }
  if (typeof config.environment !== "string" || config.environment.trim() !== config.environment ||
      config.environment.length < 3 || config.environment.length > 128 ||
      config.environment.toLowerCase().includes("production") || !config.environment.toLowerCase().includes("isolated")) {
    throw new Error("availability environment must identify an isolated non-production fixture");
  }
  if (typeof config.hardware !== "string" || config.hardware.trim() !== config.hardware ||
      config.hardware.length < 3 || config.hardware.length > 256) {
    throw new Error("availability hardware description is invalid");
  }
  if (!gitRevisionPattern.test(config.gitRevision ?? "")) {
    throw new Error("availability git revision must be a complete lowercase commit SHA");
  }
  if (!runIDPattern.test(config.runID ?? "")) {
    throw new Error("availability run identity is invalid");
  }
  if (!Number.isSafeInteger(config.probesPerMinute) || config.probesPerMinute < 2 || config.probesPerMinute > 12) {
    throw new Error("availability probes per minute must be bounded between 2 and 12");
  }
  if (config.productionEgressDisabled !== true) {
    throw new Error("availability rehearsal requires production egress to be disabled");
  }
  return config;
}

export async function runAvailabilityRehearsal(config, dependencies = {}) {
  validateAvailabilityConfig(config);
  const transition = dependencies.transition;
  const probe = dependencies.probe;
  const sleep = dependencies.sleep ?? ((milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds)));
  const now = dependencies.now ?? (() => new Date());
  const minuteDurationMillis = dependencies.minuteDurationMillis ?? defaultMinuteDurationMillis;
  if (typeof transition !== "function" || typeof probe !== "function" || typeof sleep !== "function" || typeof now !== "function") {
    throw new Error("availability rehearsal runtime is unavailable");
  }
  if (!Number.isSafeInteger(minuteDurationMillis) || minuteDurationMillis < config.probesPerMinute ||
      minuteDurationMillis % config.probesPerMinute !== 0) {
    throw new Error("availability observation minute must divide evenly across probes");
  }

  const samples = [];
  const probeIntervalMillis = minuteDurationMillis / config.probesPerMinute;
  for (const scenario of scenarios) {
    await transition(scenario.name);
    const startedAt = validTimestamp(now(), `${scenario.name} start`);
    const probes = [];
    for (let index = 0; index < config.probesPerMinute; index += 1) {
      await sleep(probeIntervalMillis);
      const observed = await probe({ scenario: scenario.name, sequence: index + 1 });
      validateProbe(observed, scenario.name);
      probes.push({
        sequence: index + 1,
        observed_at: validTimestamp(now(), `${scenario.name} probe`),
        http_status: observed.http_status,
        business_code: observed.business_code,
        duration_millis: observed.duration_millis,
        available: observed.http_status === 200 && observed.business_code === 0,
      });
    }
    const observedAPIAvailable = probes.every((entry) => entry.available);
    if (observedAPIAvailable !== scenario.expectedAPIAvailable) {
      throw new Error(`${scenario.name} availability mismatch: expected ${scenario.expectedAPIAvailable}, observed ${observedAPIAvailable}`);
    }
    samples.push({
      scenario: scenario.name,
      dependency: scenario.dependency,
      observation_minutes: 1,
      started_at: startedAt,
      ended_at: validTimestamp(now(), `${scenario.name} end`),
      expected_api_available: scenario.expectedAPIAvailable,
      observed_api_available: observedAPIAvailable,
      attributed: true,
      probes,
    });
  }

  const availableMinutes = samples.filter((sample) => sample.observed_api_available).length;
  const unavailableMinutes = samples.length - availableMinutes;
  const availabilityPercent = roundSix((availableMinutes / samples.length) * 100);
  const budgetMinutes = Math.floor(calendarWindowDays * 24 * 60 * (100 - candidateTargetPercent) / 100);
  return {
    version,
    status: "measured",
    approval: "required",
    release_ready: false,
    git_revision: config.gitRevision,
    environment: config.environment,
    hardware: config.hardware,
    run_id: config.runID,
    production_egress_disabled: true,
    scope: "candidate_methodology_not_production_sla",
    compose: {
      project: config.composeProject,
      file: basename(config.composeFile),
      topology: "single_host_split_api_worker_no_ha",
    },
    measurement: {
      probe: "/readyz",
      aggregation: "conservative_all_probes_per_observation_minute",
      maintenance_window: "none",
      observation_minute_millis: minuteDurationMillis,
      probes_per_minute: config.probesPerMinute,
      excluded_minutes: 0,
      available_minutes: availableMinutes,
      unavailable_minutes: unavailableMinutes,
      availability_percent: availabilityPercent,
    },
    error_budget: {
      candidate_target_percent: candidateTargetPercent,
      calendar_window_days: calendarWindowDays,
      budget_minutes: budgetMinutes,
      injected_unavailable_minutes: unavailableMinutes,
      remaining_budget_minutes: Math.max(0, budgetMinutes - unavailableMinutes),
      target_met_in_injected_window: availabilityPercent >= candidateTargetPercent,
    },
    exclusions: ["production_traffic", "external_provider_delivery", "high_availability_failover"],
    samples,
    differences: [],
  };
}

export function writeAvailabilityReport(output, report, sentinels = []) {
  if (typeof output !== "string" || output.trim() !== output || output.length === 0) {
    throw new Error("availability output path is required");
  }
  const payload = `${JSON.stringify(report, null, 2)}\n`;
  for (const sentinel of sentinels) {
    if (typeof sentinel === "string" && sentinel.length !== 0 && payload.includes(sentinel)) {
      throw new Error("availability report contains a protected sentinel");
    }
  }
  writeFileSync(output, payload, { flag: "wx", mode: 0o600 });
}

function validateProbe(probe, scenario) {
  if (probe == null || typeof probe !== "object" || !Number.isSafeInteger(probe.http_status) ||
      probe.http_status < 100 || probe.http_status > 599 || !Number.isSafeInteger(probe.business_code) ||
      !Number.isFinite(probe.duration_millis) || probe.duration_millis < 0) {
    throw new Error(`${scenario} returned an invalid availability probe`);
  }
}

function validTimestamp(value, label) {
  if (!(value instanceof Date) || Number.isNaN(value.getTime())) throw new Error(`${label} clock is invalid`);
  return value.toISOString();
}

function roundSix(value) {
  return Number(value.toFixed(6));
}
