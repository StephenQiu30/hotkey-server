import { readFileSync, writeFileSync } from "node:fs";
import process from "node:process";

const artifactDirectory = process.env.HOTKEY_BROWSER_ARTIFACT_DIR || "/tmp";
const a11yFiles = ["reports", "knowledge", "search-desktop", "search-mobile", "notifications-empty", "knowledge-permission"].map(
  (name) => `${artifactDirectory}/hotkey-a11y-${name}.json`,
);
const audits = a11yFiles.map(readJSON);
const errors = readJSON(`${artifactDirectory}/hotkey-browser-errors.json`);
const network = readJSON(`${artifactDirectory}/hotkey-browser-network.json`);
const viewport = readJSON(`${artifactDirectory}/hotkey-browser-viewport.json`);

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
if (!network?.success || failedRequests.length !== 0) {
  throw new Error(`real browser emitted ${failedRequests.length} failed HTTP request(s)`);
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

const summary = {
  version: "hotkey-browser-business-flow-v1",
  run_id: process.env.HOTKEY_BROWSER_RUN_ID,
  a11y_pages: a11yFiles.length,
  a11y_violations: 0,
  page_errors: 0,
  failed_requests: [],
  observed_requests: requiredRequests.length,
  mobile_viewport: { width: 390, height: 844, horizontal_overflow: false },
};
writeFileSync(`${artifactDirectory}/hotkey-browser-acceptance.json`, `${JSON.stringify(summary, null, 2)}\n`, { flag: "wx" });

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}
