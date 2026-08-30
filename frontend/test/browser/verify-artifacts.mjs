import { readFileSync, writeFileSync } from "node:fs";
import process from "node:process";

const artifactDirectory = process.env.HOTKEY_BROWSER_ARTIFACT_DIR || "/tmp";
const a11yFiles = ["reports", "report-content-security", "knowledge", "search-desktop", "search-mobile", "notifications-empty", "knowledge-permission", "role-editor", "role-analyst", "role-viewer", "role-admin"].map(
  (name) => `${artifactDirectory}/hotkey-a11y-${name}.json`,
);
const audits = a11yFiles.map(readJSON);
const errors = readJSON(`${artifactDirectory}/hotkey-browser-errors.json`);
const network = readJSON(`${artifactDirectory}/hotkey-browser-network.json`);
const viewport = readJSON(`${artifactDirectory}/hotkey-browser-viewport.json`);
const contentSecurity = readJSON(`${artifactDirectory}/hotkey-browser-content-security.json`);
const roleFiles = {
  viewer: `${artifactDirectory}/hotkey-role-viewer.json`,
  analyst: `${artifactDirectory}/hotkey-role-analyst.json`,
  editor: `${artifactDirectory}/hotkey-role-editor.json`,
  admin: `${artifactDirectory}/hotkey-role-admin.json`,
};
const roleArtifacts = Object.fromEntries(
  Object.entries(roleFiles).map(([role, path]) => [role, readJSON(path)]),
);
const keyboard = readJSON(`${artifactDirectory}/hotkey-keyboard-focus.json`);

for (const [index, audit] of audits.entries()) {
  const violations = audit?.data?.violations ?? [];
  if (!audit?.success || violations.length !== 0) {
    throw new Error(`accessibility audit ${a11yFiles[index]} has ${violations.length} violation(s)`);
  }
}

const pageErrors = errors?.data?.errors ?? [];
if (!errors?.success || pageErrors.length !== 0) {
  throw new Error(`real browser emitted ${pageErrors.length} page error(s)`);
}

const requests = network?.data?.requests ?? [];
const failedRequests = requests.filter(
  (request) => typeof request.status === "number" && request.status >= 400,
);
const failedRequestSummary = failedRequests.map((request) => ({
  method: request.method,
  status: request.status,
  pathname: safePathname(request.url),
}));
writeFileSync(
  `${artifactDirectory}/hotkey-browser-network-summary.json`,
  `${JSON.stringify({ version: "hotkey-browser-network-summary-v1", run_id: process.env.HOTKEY_BROWSER_RUN_ID, failed_requests: failedRequestSummary }, null, 2)}\n`,
  { flag: "wx" },
);
if (!network?.success || failedRequests.length !== 0) {
  throw new Error(`real browser emitted ${failedRequests.length} failed HTTP request(s): ${JSON.stringify(failedRequestSummary)}`);
}

const requiredRequests = [
  ["POST", /\/api\/v1\/auth\/login$/],
  ["GET", /\/api\/v1\/notifications(?:\?|$)/],
  ["POST", /\/api\/v1\/reports\/\d+\/approve$/],
  ["POST", /\/api\/v1\/knowledge\/proposals\/\d+\/approve(?:\?|$)/],
  ["POST", /\/api\/v1\/knowledge\/proposals\/\d+\/apply(?:\?|$)/],
  ["GET", /\/api\/v1\/search(?:\?|$)/],
];
for (const [method, pattern] of requiredRequests) {
  if (!requests.some((request) => request.method === method && request.status >= 200 && request.status < 300 && pattern.test(request.url))) {
    throw new Error(`real browser did not observe required request ${method} ${pattern}`);
  }
}

if (!viewport?.success || viewport?.data?.result !== true) {
  throw new Error("mobile viewport has horizontal overflow or an invalid main landmark");
}

if (!contentSecurity?.success || contentSecurity?.data?.result !== true) {
  throw new Error("malicious report content created an executable DOM surface");
}

for (const [role, artifact] of Object.entries(roleArtifacts)) {
  const result = artifact?.data?.result;
  const expectedBoundary = role === "admin" ? result?.admin_surface : result?.denied_surface;
  if (!artifact?.success || result?.role !== role || result?.allowed_surface !== true || expectedBoundary !== true || result?.navigation_policy !== true) {
    throw new Error(`browser role matrix failed for ${role}`);
  }
}

const keyboardResult = keyboard?.data?.result;
if (!keyboard?.success || keyboardResult?.sequence_length !== 8 || keyboardResult?.left_body !== true || keyboardResult?.distinct_targets < 6 || keyboardResult?.visible_focus !== true) {
  throw new Error("browser keyboard focus matrix did not preserve ordered visible focus");
}

const summary = {
  version: "hotkey-browser-business-flow-v1",
  run_id: process.env.HOTKEY_BROWSER_RUN_ID,
  a11y_pages: a11yFiles.length,
  a11y_violations: 0,
  page_errors: 0,
  failed_requests: [],
  observed_requests: requiredRequests.length,
  mobile_viewport: { width: 390, height: 844, horizontal_overflow: false },
  malicious_report_content_inert: true,
  four_role_browser_matrix: true,
  keyboard_focus: {
    sequence_length: keyboardResult.sequence_length,
    distinct_targets: keyboardResult.distinct_targets,
    visible: true,
  },
};
writeFileSync(`${artifactDirectory}/hotkey-browser-acceptance.json`, `${JSON.stringify(summary, null, 2)}\n`, { flag: "wx" });

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

function safePathname(value) {
  try {
    return new URL(value).pathname;
  } catch {
    return "invalid-url";
  }
}
