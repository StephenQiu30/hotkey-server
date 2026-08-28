import process from "node:process";

const apiOrigin = required("HOTKEY_BROWSER_API_ORIGIN").replace(/\/$/, "");
const email = required("HOTKEY_BROWSER_EMAIL");
const password = required("HOTKEY_BROWSER_PASSWORD");
const monitorID = Number(required("HOTKEY_BROWSER_MONITOR_ID"));
const runID = required("HOTKEY_BROWSER_RUN_ID");

if (!Number.isSafeInteger(monitorID) || monitorID <= 0 || !/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(runID)) {
  throw new Error("browser acceptance fixture identity is invalid");
}

const login = await request("/api/v1/auth/login", {
  method: "POST",
  body: { email, password },
});
const accessToken = login?.access_token;
if (typeof accessToken !== "string" || accessToken.length < 32) {
  throw new Error("browser acceptance login did not return an access token");
}

const report = await request("/api/v1/reports", {
  method: "POST",
  token: accessToken,
  body: { type: "daily", timezone: "Asia/Shanghai", monitor_id: monitorID },
});
if (!Number.isSafeInteger(report?.id) || !Number.isSafeInteger(report?.version) || report.status !== "draft") {
  throw new Error("browser acceptance report draft is invalid");
}
process.stdout.write(`${JSON.stringify({ runID, monitorID, reportID: report.id, reportVersion: report.version })}\n`);

async function request(path, { method, token, body }) {
  const response = await fetch(`${apiOrigin}${path}`, {
    method,
    headers: {
      Accept: "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
    signal: AbortSignal.timeout(15_000),
  });
  const envelope = await response.json().catch(() => null);
  if (!response.ok || envelope == null || typeof envelope !== "object") {
    throw new Error(`browser acceptance request failed: ${method} ${path} (${response.status})`);
  }
  return envelope.data;
}

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}
