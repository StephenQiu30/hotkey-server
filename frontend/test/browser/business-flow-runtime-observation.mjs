import { execFile as execFileCallback } from "node:child_process";
import { promisify } from "node:util";

const execFile = promisify(execFileCallback);
const isolatedPrefixPattern = /^hotkey-(?:ci|test|acceptance)(?:-[A-Za-z0-9][A-Za-z0-9_.-]{0,63})+$/;
const services = ["agent", "api", "minio", "postgres", "redis", "web"];
const projectionKinds = {
  notification: ["deliver_email", "deliver_alert_email", "project_user_notification"],
  report: ["build_report"],
  vault: ["project_knowledge", "reconcile_knowledge"],
  search: ["generate_source_document"],
};

export function createBusinessFlowRuntimeObserver(config, dependencies = {}) {
  const normalized = validateObserverConfig(config);
  const execute = dependencies.execFile ?? execFile;
  const fetchImplementation = dependencies.fetch ?? globalThis.fetch;
  if (typeof execute !== "function" || typeof fetchImplementation !== "function") {
    throw new Error("M4 runtime observation dependencies are unavailable");
  }
  const containerNames = services.map((service) => `${normalized.containerPrefix}-${service}`);
  return async () => {
    const [stats, metricsResponse] = await Promise.all([
      execute("docker", ["stats", "--no-stream", "--format", "{{json .}}", ...containerNames], { maxBuffer: 1024 * 1024 }),
      fetchImplementation(`${normalized.apiOrigin}/metrics`, { headers: { Accept: "text/plain" }, signal: AbortSignal.timeout(5_000) }),
    ]);
    if (metricsResponse == null || metricsResponse.ok !== true) throw new Error("M4 runtime metrics request failed");
    const metrics = await metricsResponse.text();
    return {
      resources: parseDockerResourceSamples(stats?.stdout, normalized.containerPrefix),
      projection_backlog: parseProjectionBacklogMetrics(metrics),
    };
  };
}

export function parseDockerResourceSamples(output, containerPrefix) {
  if (!isolatedPrefixPattern.test(containerPrefix ?? "") || typeof output !== "string") {
    throw new Error("M4 Docker resource observation is invalid");
  }
  const expected = new Map(services.map((service) => [`${containerPrefix}-${service}`, service]));
  const resources = {};
  for (const line of output.split("\n").filter((candidate) => candidate.trim() !== "")) {
    let raw;
    try {
      raw = JSON.parse(line);
    } catch {
      throw new Error("M4 Docker resource observation is not JSON");
    }
    const service = expected.get(raw?.Name);
    if (service == null || resources[service] != null) throw new Error("M4 Docker resource observation contains an unexpected container");
    const memory = String(raw.MemUsage ?? "").split(/\s*\/\s*/);
    if (memory.length !== 2) throw new Error("M4 Docker memory observation is invalid");
    const sample = {
      cpu_percent: parsePercent(raw.CPUPerc),
      memory_used_bytes: parseBytes(memory[0]),
      memory_limit_bytes: parseBytes(memory[1]),
      memory_percent: parsePercent(raw.MemPerc),
      pids: parseUnsignedInteger(raw.PIDs),
    };
    if (sample.memory_limit_bytes <= 0 || sample.memory_used_bytes > sample.memory_limit_bytes) {
      throw new Error("M4 Docker memory observation exceeds its fixed limit");
    }
    resources[service] = sample;
  }
  if (Object.keys(resources).sort().join(",") !== [...services].sort().join(",")) {
    throw new Error("M4 Docker resource observation is incomplete");
  }
  return Object.fromEntries(Object.entries(resources).sort(([left], [right]) => left.localeCompare(right)));
}

export function parseProjectionBacklogMetrics(metrics) {
  if (typeof metrics !== "string" || metrics.length === 0 || metrics.length > 4 * 1024 * 1024) {
    throw new Error("M4 projection backlog metrics are invalid");
  }
  let collectionSucceeded = false;
  const counts = new Map();
  const lags = new Map();
  for (const line of metrics.split("\n")) {
    if (line === "hotkey_runtime_metrics_collection_success 1") collectionSucceeded = true;
    let match = line.match(/^hotkey_job_runs_total\{([^}]*)\}\s+([^\s]+)$/);
    if (match) {
      const labels = parseLabels(match[1]);
      const count = Number(match[2]);
      if (!Number.isSafeInteger(count) || count < 0) throw new Error("M4 projection backlog count is invalid");
      if (typeof labels.kind === "string" && (labels.state === "available" || labels.state === "running")) {
        counts.set(`${labels.kind}:${labels.state}`, count);
      }
      continue;
    }
    match = line.match(/^hotkey_job_queue_lag_seconds\{([^}]*)\}\s+([^\s]+)$/);
    if (match) {
      const labels = parseLabels(match[1]);
      const lag = Number(match[2]);
      if (!Number.isFinite(lag) || lag < 0) throw new Error("M4 projection backlog lag is invalid");
      if (typeof labels.kind === "string") lags.set(labels.kind, lag);
    }
  }
  if (!collectionSucceeded) throw new Error("M4 runtime metric collection did not succeed");
  const result = {};
  for (const [stage, kinds] of Object.entries(projectionKinds)) {
    result[stage] = {
      available: sum(kinds.map((kind) => counts.get(`${kind}:available`) ?? 0)),
      running: sum(kinds.map((kind) => counts.get(`${kind}:running`) ?? 0)),
      lag_seconds: Math.max(0, ...kinds.map((kind) => lags.get(kind) ?? 0)),
    };
  }
  return result;
}

function validateObserverConfig(config) {
  if (config == null || typeof config !== "object" || !isolatedPrefixPattern.test(config.containerPrefix ?? "")) {
    throw new Error("M4 runtime observer requires an isolated container prefix");
  }
  let origin;
  try {
    origin = new URL(config.apiOrigin);
  } catch {
    throw new Error("M4 runtime observer API origin is invalid");
  }
  if (origin.protocol !== "http:" || !["127.0.0.1", "localhost", "[::1]"].includes(origin.hostname) || origin.pathname !== "/") {
    throw new Error("M4 runtime observer API must be isolated loopback HTTP");
  }
  return { apiOrigin: origin.origin, containerPrefix: config.containerPrefix };
}

function parseLabels(value) {
  const labels = {};
  const pattern = /([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"\\])*)"/g;
  let match;
  while ((match = pattern.exec(value)) != null) labels[match[1]] = match[2].replace(/\\([\\"])/g, "$1");
  return labels;
}

function parsePercent(value) {
  const raw = typeof value === "string" && value.endsWith("%") ? value.slice(0, -1) : "";
  const parsed = Number(raw);
  if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100_000) throw new Error("M4 Docker percentage is invalid");
  return parsed;
}

function parseBytes(value) {
  const match = String(value ?? "").trim().match(/^(\d+(?:\.\d+)?)\s*([kmgtpe]?i?b)$/i);
  if (!match) throw new Error("M4 Docker byte quantity is invalid");
  const units = { b: 1, kb: 1e3, mb: 1e6, gb: 1e9, tb: 1e12, pb: 1e15, eb: 1e18, kib: 1024, mib: 1024 ** 2, gib: 1024 ** 3, tib: 1024 ** 4, pib: 1024 ** 5, eib: 1024 ** 6 };
  const bytes = Math.round(Number(match[1]) * units[match[2].toLowerCase()]);
  if (!Number.isSafeInteger(bytes) || bytes < 0) throw new Error("M4 Docker byte quantity is unsafe");
  return bytes;
}

function parseUnsignedInteger(value) {
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < 0) throw new Error("M4 Docker process count is invalid");
  return parsed;
}

function sum(values) {
  const total = values.reduce((current, value) => current + value, 0);
  if (!Number.isSafeInteger(total)) throw new Error("M4 projection backlog total is unsafe");
  return total;
}
