import process from "node:process";
import {
  runBusinessFlowCapacity,
  writeBusinessFlowCapacityReport,
} from "./business-flow-capacity.mjs";
import { createBusinessFlowRuntimeObserver } from "./business-flow-runtime-observation.mjs";

const config = {
  apiOrigin: required("HOTKEY_BROWSER_API_ORIGIN"),
  email: required("HOTKEY_BROWSER_EMAIL"),
  password: required("HOTKEY_BROWSER_PASSWORD"),
  monitorID: integer("HOTKEY_BROWSER_MONITOR_ID"),
  runID: required("HOTKEY_M4_CAPACITY_RUN_ID"),
  environment: required("HOTKEY_M4_CAPACITY_ENVIRONMENT"),
  hardware: required("HOTKEY_M4_CAPACITY_HARDWARE"),
  filesystem: required("HOTKEY_M4_CAPACITY_FILESYSTEM"),
  gitRevision: required("HOTKEY_M4_CAPACITY_GIT_REVISION"),
  confirmIsolated: boolean("HOTKEY_M4_CAPACITY_CONFIRM_ISOLATED"),
  productionEgressDisabled: boolean("HOTKEY_M4_CAPACITY_PRODUCTION_EGRESS_DISABLED"),
  warmups: integer("HOTKEY_M4_CAPACITY_WARMUPS", 2),
  samples: integer("HOTKEY_M4_CAPACITY_SAMPLES", 12),
  intervalMillis: integer("HOTKEY_M4_CAPACITY_INTER_FLOW_INTERVAL_MILLIS", 31_000),
};
const report = await runBusinessFlowCapacity(config, {
  observeRuntime: createBusinessFlowRuntimeObserver({
    apiOrigin: config.apiOrigin,
    containerPrefix: required("HOTKEY_CONTAINER_PREFIX"),
  }),
});
writeBusinessFlowCapacityReport(required("HOTKEY_M4_CAPACITY_OUTPUT"), report, [
  config.email,
  config.password,
  optional("HOTKEY_M4_CAPACITY_BODY_SENTINEL"),
  optional("HOTKEY_M4_CAPACITY_EMAIL_SENTINEL"),
  optional("HOTKEY_M4_CAPACITY_HOST_PATH_SENTINEL"),
  optional("HOTKEY_M4_CAPACITY_CREDENTIAL_SENTINEL"),
]);
process.stdout.write(`M4 capacity measured: run_id=${report.run_id} samples=${report.workload.samples} errors=${report.errors}\n`);

function required(name) {
  const value = optional(name);
  if (value === "") throw new Error(`${name} is required`);
  return value;
}

function optional(name) {
  return process.env[name]?.trim() ?? "";
}

function integer(name, fallback) {
  const raw = optional(name);
  if (raw === "" && fallback !== undefined) return fallback;
  const value = Number(raw);
  if (!Number.isSafeInteger(value)) throw new Error(`${name} must be an integer`);
  return value;
}

function boolean(name) {
  return optional(name).toLowerCase() === "true";
}
