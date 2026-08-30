import process from "node:process";
import { spawn } from "node:child_process";
import {
  runAvailabilityRehearsal,
  writeAvailabilityReport,
} from "./availability-rehearsal.mjs";

const composeProject = required("HOTKEY_AVAILABILITY_COMPOSE_PROJECT");
const composeFile = required("HOTKEY_AVAILABILITY_COMPOSE_FILE");
const containerPrefix = required("HOTKEY_CONTAINER_PREFIX");
const apiContainer = `${composeProject}-availability-api`;
const workerContainer = `${composeProject}-availability-worker`;
const config = {
  apiOrigin: required("HOTKEY_BROWSER_API_ORIGIN"),
  composeFile,
  composeProject,
  apiContainer,
  workerContainer,
  environment: required("HOTKEY_AVAILABILITY_ENVIRONMENT"),
  hardware: required("HOTKEY_AVAILABILITY_HARDWARE"),
  gitRevision: required("HOTKEY_AVAILABILITY_GIT_REVISION"),
  runID: required("HOTKEY_AVAILABILITY_RUN_ID"),
  probesPerMinute: 6,
  productionEgressDisabled: boolean("HOTKEY_AVAILABILITY_PRODUCTION_EGRESS_DISABLED"),
};

validateContainerPrefix(containerPrefix);
let report;
let rehearsalError;
try {
  await splitRuntimeRoles();
  report = await runAvailabilityRehearsal(config, {
    transition: transitionScenario,
    probe: probeReadiness,
  });
} catch (error) {
  rehearsalError = error;
} finally {
  try {
    await restoreFreshStack();
  } catch (cleanupError) {
    if (rehearsalError == null) rehearsalError = cleanupError;
    else rehearsalError = new AggregateError([rehearsalError, cleanupError], "availability rehearsal and cleanup failed");
  }
}
if (rehearsalError != null) throw rehearsalError;

writeAvailabilityReport(required("HOTKEY_AVAILABILITY_OUTPUT"), report, protectedSentinels());
process.stdout.write(
  `Availability measured: run_id=${report.run_id} available=${report.measurement.available_minutes} unavailable=${report.measurement.unavailable_minutes}\n`,
);

async function splitRuntimeRoles() {
  await compose("stop", "hotkey-web", "hotkey-server");
  await removeContainer(apiContainer);
  await removeContainer(workerContainer);
  await compose(
    "run", "--detach", "--name", apiContainer, "--no-deps", "--publish", "127.0.0.1:8866:8080",
    "--env", "HOTKEY_ROLE=api", "hotkey-server", "serve", "--role", "api",
  );
  await compose(
    "run", "--detach", "--name", workerContainer, "--no-deps",
    "--env", "HOTKEY_ROLE=worker", "hotkey-server", "serve", "--role", "worker",
  );
  await waitForContainerRunning(workerContainer);
  await waitForAvailability(true);
}

async function transitionScenario(scenario) {
  switch (scenario) {
    case "baseline":
      await startDependencies();
      await waitForAvailability(true);
      return;
    case "postgres_unavailable":
      await compose("stop", "postgres");
      await waitForAvailability(false);
      return;
    case "postgres_recovered":
      await compose("start", "postgres");
      await waitForDependencyHealth("postgres");
      await waitForAvailability(true);
      return;
    case "redis_unavailable":
      await compose("stop", "redis");
      await waitForAvailability(true);
      return;
    case "minio_unavailable":
      await compose("start", "redis");
      await waitForDependencyHealth("redis");
      await compose("stop", "minio");
      await waitForAvailability(true);
      return;
    case "worker_unavailable":
      await compose("start", "minio");
      await waitForDependencyHealth("minio");
      await runDocker(["stop", workerContainer]);
      await waitForAvailability(true);
      return;
    case "full_recovery":
      await startDependencies();
      await runDocker(["start", workerContainer]);
      await waitForContainerRunning(workerContainer);
      await waitForAvailability(true);
      return;
    default:
      throw new Error(`unsupported availability scenario: ${scenario}`);
  }
}

async function startDependencies() {
  await compose("start", "postgres", "redis", "minio");
  for (const service of ["postgres", "redis", "minio"]) await waitForDependencyHealth(service);
}

async function restoreFreshStack() {
  await removeContainer(apiContainer);
  await removeContainer(workerContainer);
  await compose("start", "postgres", "redis", "minio", { allowFailure: true });
  await compose("up", "--detach", "--wait", "--wait-timeout", "180", "hotkey-server", "hotkey-web");
}

async function waitForAvailability(expected) {
  const deadline = Date.now() + 120_000;
  let last = "no response";
  while (Date.now() < deadline) {
    try {
      const observed = await probeReadiness();
      last = `${observed.http_status}/${observed.business_code}`;
      if (expected && observed.http_status === 200 && observed.business_code === 0) return;
      if (!expected && observed.http_status === 503 && observed.business_code === 90001) return;
    } catch (error) {
      last = error instanceof Error ? error.message : "probe failed";
    }
    await delay(1_000);
  }
  throw new Error(`availability readiness did not become ${expected ? "available" : "database-unavailable"}: ${last}`);
}

async function waitForDependencyHealth(service) {
  const container = `${containerPrefix}-${service}`;
  const deadline = Date.now() + 120_000;
  let last = "unknown";
  while (Date.now() < deadline) {
    const result = await runDocker(["inspect", container], { allowFailure: true, capture: true });
    if (result.code === 0) {
      try {
        const inspected = JSON.parse(result.stdout)?.[0]?.State;
        last = inspected?.Health?.Status ?? inspected?.Status ?? "unknown";
        if (inspected?.Running === true && (inspected?.Health == null || inspected.Health.Status === "healthy")) return;
      } catch {
        last = "invalid inspect output";
      }
    }
    await delay(1_000);
  }
  throw new Error(`availability dependency ${service} did not become healthy: ${last}`);
}

async function waitForContainerRunning(container) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const result = await runDocker(["inspect", container], { allowFailure: true, capture: true });
    if (result.code === 0) {
      try {
        if (JSON.parse(result.stdout)?.[0]?.State?.Running === true) return;
      } catch {
        // Retry a bounded exact-container inspection.
      }
    }
    await delay(500);
  }
  throw new Error(`availability container ${container} did not start`);
}

async function probeReadiness() {
  const started = performance.now();
  const response = await fetch(`${config.apiOrigin.replace(/\/$/, "")}/readyz`, {
    headers: { Accept: "application/json" },
    signal: AbortSignal.timeout(5_000),
  });
  const envelope = await response.json().catch(() => null);
  return {
    http_status: response.status,
    business_code: Number.isSafeInteger(envelope?.code) ? envelope.code : -1,
    duration_millis: Number((performance.now() - started).toFixed(3)),
  };
}

async function compose(...input) {
  let options = {};
  if (input.at(-1) != null && typeof input.at(-1) === "object") options = input.pop();
  return runDocker(["compose", "--project-name", composeProject, "--file", composeFile, ...input], options);
}

async function removeContainer(container) {
  await runDocker(["rm", "--force", container], { allowFailure: true });
}

async function runDocker(args, { allowFailure = false, capture = false } = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn("docker", args, { env: process.env, stdio: capture ? ["ignore", "pipe", "pipe"] : "inherit" });
    let stdout = "";
    let stderr = "";
    if (capture) {
      child.stdout.setEncoding("utf8");
      child.stderr.setEncoding("utf8");
      child.stdout.on("data", (chunk) => { stdout += chunk; });
      child.stderr.on("data", (chunk) => { stderr += chunk; });
    }
    child.once("error", reject);
    child.once("close", (code) => {
      if (code === 0 || allowFailure) resolve({ code, stdout, stderr });
      else reject(new Error(`bounded Docker operation failed with exit code ${code}: ${safeDockerOperation(args)}`));
    });
  });
}

function safeDockerOperation(args) {
  return args.filter((value) => !value.includes("=")).join(" ");
}

function protectedSentinels() {
  return [
    optional("HOTKEY_BROWSER_EMAIL"),
    optional("HOTKEY_BROWSER_PASSWORD"),
    optional("HOTKEY_M4_CAPACITY_BODY_SENTINEL"),
    optional("HOTKEY_M4_CAPACITY_EMAIL_SENTINEL"),
    optional("HOTKEY_M4_CAPACITY_HOST_PATH_SENTINEL"),
    optional("HOTKEY_CREDENTIAL_BODY_SENTINEL"),
  ];
}

function validateContainerPrefix(value) {
  if (!/^[a-z0-9][a-z0-9_-]{1,62}$/.test(value) || value === "hotkey") {
    throw new Error("availability dependency container prefix must be isolated from the formal stack");
  }
}

function required(name) {
  const value = optional(name);
  if (value === "") throw new Error(`${name} is required`);
  return value;
}

function optional(name) {
  return process.env[name]?.trim() ?? "";
}

function boolean(name) {
  return optional(name).toLowerCase() === "true";
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
