import { writeFileSync } from "node:fs";

const version = "hotkey-m4-business-flow-capacity-v1";
const gitRevisionPattern = /^[0-9a-f]{40}$/;
const runIDPattern = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$/;

export function validateBusinessFlowCapacityConfig(config) {
  if (config == null || typeof config !== "object") throw new Error("M4 capacity configuration is required");
  let origin;
  try {
    origin = new URL(config.apiOrigin);
  } catch {
    throw new Error("M4 capacity API origin is invalid");
  }
  if (origin.protocol !== "http:" || !["127.0.0.1", "localhost", "[::1]"].includes(origin.hostname) || origin.pathname !== "/") {
    throw new Error("M4 capacity API must be an isolated loopback HTTP endpoint");
  }
  if (typeof config.email !== "string" || !config.email.includes("@") || typeof config.password !== "string" || config.password.length < 16) {
    throw new Error("M4 capacity fixture credentials are invalid");
  }
  if (!Number.isSafeInteger(config.monitorID) || config.monitorID <= 0 || !runIDPattern.test(config.runID ?? "")) {
    throw new Error("M4 capacity fixture identity is invalid");
  }
  if (typeof config.environment !== "string" || config.environment.trim() !== config.environment || config.environment.length < 3 ||
      config.environment.length > 128 || config.environment.toLowerCase() === "production") {
    throw new Error("M4 capacity environment must identify an isolated non-production fixture");
  }
  if (typeof config.hardware !== "string" || config.hardware.trim() !== config.hardware || config.hardware.length < 3 || config.hardware.length > 256) {
    throw new Error("M4 capacity hardware description is invalid");
  }
  if (!gitRevisionPattern.test(config.gitRevision ?? "") || config.confirmIsolated !== true || config.productionEgressDisabled !== true) {
    throw new Error("M4 capacity requires a fixed revision and isolated egress-disabled confirmation");
  }
  if (!Number.isSafeInteger(config.warmups) || config.warmups < 0 || config.warmups > 20 ||
      !Number.isSafeInteger(config.samples) || config.samples < 1 || config.samples > 100) {
    throw new Error("M4 capacity warmups and samples must be finite and bounded");
  }
  const intervalMillis = config.intervalMillis ?? 0;
  if (!Number.isSafeInteger(intervalMillis) || intervalMillis < 0 || intervalMillis > 60_000) {
    throw new Error("M4 capacity inter-flow interval must be finite and bounded");
  }
  return config;
}

export async function runBusinessFlowCapacity(config, dependencies = {}) {
  validateBusinessFlowCapacityConfig(config);
  const fetchImplementation = dependencies.fetch ?? globalThis.fetch;
  const nowMicros = dependencies.nowMicros ?? (() => Number(process.hrtime.bigint() / 1000n));
  const sleep = dependencies.sleep ?? ((milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds)));
  if (typeof fetchImplementation !== "function" || typeof nowMicros !== "function" || typeof sleep !== "function") {
    throw new Error("M4 capacity runtime is unavailable");
  }
  const apiOrigin = config.apiOrigin.replace(/\/$/, "");
  const requestIDs = new Set();
  let requestSequence = 0;
  const durations = Object.fromEntries(stageNames().map((stage) => [stage, []]));

  const request = async (path, { method = "GET", token, body, stage, iteration }) => {
    const operation = ++requestSequence;
    const requestID = `${config.runID}:${stage}:${iteration}:${operation}`;
    const response = await fetchImplementation(`${apiOrigin}${path}`, {
      method,
      headers: {
        Accept: "application/json",
        "X-Request-ID": requestID,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(body ? { "Content-Type": "application/json" } : {}),
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
      signal: AbortSignal.timeout(15_000),
    });
    const reflectedRequestID = response.headers.get("x-request-id");
    if (reflectedRequestID !== requestID || requestIDs.has(reflectedRequestID)) {
      throw new Error(`M4 capacity request correlation failed for ${method} ${safePath(path)}`);
    }
    requestIDs.add(reflectedRequestID);
    const envelope = await response.json().catch(() => null);
    if (!response.ok || envelope == null || typeof envelope !== "object" || !("data" in envelope)) {
      throw new Error(`M4 capacity request failed: ${method} ${safePath(path)} (${response.status})`);
    }
    return envelope.data;
  };

  const login = await request("/api/v1/auth/login", {
    method: "POST",
    body: { email: config.email, password: config.password },
    stage: "authentication",
    iteration: 0,
  });
  const token = login?.access_token;
  if (typeof token !== "string" || token.length < 32) throw new Error("M4 capacity login did not return an access token");

  const total = config.warmups + config.samples;
  for (let iteration = 1; iteration <= total; iteration++) {
    if (iteration > 1 && (config.intervalMillis ?? 0) > 0) await sleep(config.intervalMillis);
    const measured = iteration > config.warmups;
    const measure = async (stage, operation) => {
      if (!measured) return operation();
      const started = nowMicros();
      const result = await operation();
      const elapsed = nowMicros() - started;
      if (!Number.isSafeInteger(elapsed) || elapsed < 0) throw new Error(`M4 capacity ${stage} clock is invalid`);
      durations[stage].push(elapsed);
      return result;
    };

    const report = await measure("report_build", () => request("/api/v1/reports", {
      method: "POST", token, stage: "report_build", iteration,
      body: { type: "daily", timezone: "Asia/Shanghai", monitor_id: config.monitorID },
    }));
    if (!positiveID(report?.id) || !positiveID(report?.version) || report.status !== "draft") {
      throw new Error("M4 capacity report draft is invalid");
    }

    const pending = await measure("notification_visibility", async () => {
      const submitted = await request(`/api/v1/reports/${report.id}/submit`, {
        method: "POST", token, stage: "notification_visibility", iteration,
        body: { expected_resource_version: report.version },
      });
      const notifications = await request("/api/v1/notifications?after_id=0&limit=100", {
        token, stage: "notification_visibility", iteration,
      });
      const notice = notifications?.items?.find((item) => item.resource_type === "report" && item.resource_id === report.id &&
        item.event_type === "report.approval_requested");
      if (notice?.deep_link !== `/dashboard/reports?report=${report.id}`) {
        throw new Error("M4 capacity notification did not converge to the submitted report");
      }
      return submitted;
    });
    if (pending?.status !== "pending_approval" || pending.version !== report.version + 1) {
      throw new Error("M4 capacity report submission is invalid");
    }

    const published = await measure("report_publication", () => request(`/api/v1/reports/${report.id}/approve`, {
      method: "POST", token, stage: "report_publication", iteration,
      body: { expected_resource_version: pending.version },
    }));
    if (published?.status !== "published" || published.frozen !== true) {
      throw new Error("M4 capacity report publication is invalid");
    }

    const document = await measure("vault_publication", async () => {
      const documents = await request("/api/v1/knowledge/documents?limit=200", {
        token, stage: "vault_publication", iteration,
      });
      const planned = documents?.items?.find((item) => item.reportID === report.id);
      if (!positiveID(planned?.id)) throw new Error("M4 capacity report knowledge document is missing");
      const proposals = await request("/api/v1/knowledge/proposals?status=pending&limit=200", {
        token, stage: "vault_publication", iteration,
      });
      const proposal = proposals?.items?.find((item) => item.documentID === planned.id);
      if (!positiveID(proposal?.id) || !positiveID(proposal?.version)) {
        throw new Error("M4 capacity report knowledge proposal is missing");
      }
      const approved = await request(`/api/v1/knowledge/proposals/${proposal.id}/approve?version=${proposal.version}`, {
        method: "POST", token, stage: "vault_publication", iteration,
      });
      if (approved?.status !== "approved" || !positiveID(approved?.version)) {
        throw new Error("M4 capacity knowledge proposal approval is invalid");
      }
      return request(`/api/v1/knowledge/proposals/${proposal.id}/apply?version=${approved.version}`, {
        method: "POST", token, stage: "vault_publication", iteration,
      });
    });
    if (!positiveID(document?.id) || document.reportID !== report.id || document.status !== "active" || document.revisionNo < 1) {
      throw new Error("M4 capacity Vault publication did not converge");
    }

    await measure("search_visibility", async () => {
      const params = new URLSearchParams({ q: "日报", types: "knowledge", limit: "100" });
      const search = await request(`/api/v1/search?${params}`, { token, stage: "search_visibility", iteration });
      if (!search?.items?.some((item) => item.type === "knowledge" && item.id === document.id && item.status === "active")) {
        throw new Error("M4 capacity knowledge document is not visible through PostgreSQL search");
      }
    });
  }

  return {
    version,
    status: "measured",
    approval: "required",
    git_revision: config.gitRevision,
    environment: config.environment,
    hardware: config.hardware,
    run_id: config.runID,
    percentile_algorithm: "nearest-rank-ceiling",
    workload: {
      warmups: config.warmups,
      samples: config.samples,
      concurrency: 1,
      cache_state: "warm",
      inter_flow_interval_millis: config.intervalMillis ?? 0,
      flow: "daily_report_to_notification_to_vault_to_postgresql_search",
    },
    stages: Object.fromEntries(stageNames().map((stage) => [stage, summarizeDurations(durations[stage])])),
    correlation: { strategy: "x-request-id", observed: requestIDs.size, unique: requestIDs.size },
    privacy: {
      retained_fields: ["safe_run_id", "git_revision", "bounded_workload", "duration_micros", "request_correlation_counts"],
      excluded_fields: ["access_token", "authorization", "email", "password", "report_content", "query_text", "vault_path", "host_path"],
      sentinel_leaks: 0,
    },
    exclusions: ["external_provider_delivery", "cold_cache", "production_hardware", "production_traffic"],
    errors: 0,
  };
}

export function writeBusinessFlowCapacityReport(output, report, sentinels = []) {
  if (typeof output !== "string" || output.trim() !== output || output.length === 0) {
    throw new Error("M4 capacity output path is required");
  }
  const payload = `${JSON.stringify(report, null, 2)}\n`;
  for (const sentinel of sentinels) {
    if (typeof sentinel === "string" && sentinel.length !== 0 && payload.includes(sentinel)) {
      throw new Error("M4 capacity report contains a protected sentinel");
    }
  }
  writeFileSync(output, payload, { flag: "wx", mode: 0o600 });
}

function summarizeDurations(values) {
  if (!Array.isArray(values) || values.length === 0 || values.some((value) => !Number.isSafeInteger(value) || value < 0)) {
    throw new Error("M4 capacity stage has no valid samples");
  }
  const sorted = [...values].sort((left, right) => left - right);
  return {
    samples: sorted.length,
    duration_micros: sorted,
    p50_micros: nearestRank(sorted, 0.5),
    p95_micros: nearestRank(sorted, 0.95),
    p99_micros: nearestRank(sorted, 0.99),
    errors: 0,
  };
}

function nearestRank(sorted, percentile) {
  return sorted[Math.max(0, Math.ceil(sorted.length * percentile) - 1)];
}

function stageNames() {
  return ["report_build", "notification_visibility", "report_publication", "vault_publication", "search_visibility"];
}

function positiveID(value) {
  return Number.isSafeInteger(value) && value > 0;
}

function safePath(value) {
  try {
    return new URL(value, "http://127.0.0.1").pathname;
  } catch {
    return "invalid-path";
  }
}
