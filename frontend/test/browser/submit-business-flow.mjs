import process from "node:process";

const apiOrigin = required("HOTKEY_BROWSER_API_ORIGIN").replace(/\/$/, "");
const email = required("HOTKEY_BROWSER_EMAIL");
const password = required("HOTKEY_BROWSER_PASSWORD");
const reportID = Number(required("HOTKEY_BROWSER_REPORT_ID"));

if (!Number.isSafeInteger(reportID) || reportID <= 0) {
  throw new Error("browser acceptance report identity is invalid");
}

const login = await request("/api/v1/auth/login", {
  method: "POST",
  body: { email, password },
});
const accessToken = login?.access_token;
if (typeof accessToken !== "string" || accessToken.length < 32) {
  throw new Error("browser acceptance login did not return an access token");
}

const report = await request(`/api/v1/reports/${reportID}`, { method: "GET", token: accessToken });
if (report?.status !== "draft" || !Number.isSafeInteger(report?.version)) {
  throw new Error("browser acceptance sentinel report is not a draft");
}
const pending = await request(`/api/v1/reports/${reportID}/submit`, {
  method: "POST",
  token: accessToken,
  body: { expected_resource_version: report.version },
});
if (pending?.status !== "pending_approval" || pending?.version !== report.version + 1) {
  throw new Error("browser acceptance report submission did not freeze the expected version");
}

const notifications = await request("/api/v1/notifications?after_id=0&limit=100", {
  method: "GET",
  token: accessToken,
});
const approvalNotice = (notifications?.items ?? []).find(
  (item) => item.resource_type === "report" && item.resource_id === reportID && item.event_type === "report.approval_requested",
);
if (approvalNotice?.deep_link !== `/dashboard/reports?report=${reportID}`) {
  throw new Error("browser acceptance approval notification is missing its safe report deep link");
}

process.stdout.write(`${JSON.stringify({ reportID, reportVersion: pending.version })}\n`);

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
