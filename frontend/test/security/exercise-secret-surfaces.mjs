import { writeFileSync } from "node:fs";
import process from "node:process";

const apiOrigin = required("HOTKEY_BROWSER_API_ORIGIN").replace(/\/$/, "");
const email = required("HOTKEY_SECRET_ADMIN_EMAIL");
const password = required("HOTKEY_BROWSER_PASSWORD");
const sourceToken = required("HOTKEY_TEST_SOURCE_TOKEN");
const cookieSecret = required("HOTKEY_TEST_COOKIE_SECRET");
const artifactDirectory = required("HOTKEY_BROWSER_ARTIFACT_DIR");

const login = await request("/api/v1/auth/login", {
  method: "POST",
  body: { email, password },
});
const accessToken = login?.access_token;
if (typeof accessToken !== "string" || accessToken.length < 32) {
  throw new Error("secret surface login did not return an access token");
}

const source = await request("/api/v1/source-connections", {
  method: "POST",
  token: accessToken,
  body: {
    preset_id: "x",
    name: "Synthetic secret acceptance source",
    credential: sourceToken,
    config: {},
  },
});
if (!Number.isSafeInteger(source?.id) || source?.credential_configured !== true) {
  throw new Error("secret surface source credential was not accepted");
}
writeExclusive("hotkey-secret-source-response.json", source);

const errorResponse = await fetch(`${apiOrigin}/api/v1/source-connections/not-a-number`, {
  headers: {
    Accept: "application/json",
    Authorization: `Bearer ${cookieSecret}`,
    Cookie: `hotkey_refresh=${cookieSecret}`,
    "X-API-Key": sourceToken,
  },
  signal: AbortSignal.timeout(15_000),
});
const errorBody = await errorResponse.json().catch(() => null);
if (errorResponse.status !== 401 || !Number.isSafeInteger(errorBody?.code) || errorBody.code <= 0 || typeof errorBody?.message !== "string") {
  throw new Error(`secret surface error response is invalid (${errorResponse.status})`);
}
writeExclusive("hotkey-secret-http-error.json", {
  status: errorResponse.status,
  request_id: errorResponse.headers.get("x-request-id"),
  body: errorBody,
});

const metricsResponse = await fetch(`${apiOrigin}/metrics`, { signal: AbortSignal.timeout(15_000) });
if (!metricsResponse.ok) {
  throw new Error(`secret surface metrics request failed (${metricsResponse.status})`);
}
writeFileSync(`${artifactDirectory}/hotkey-secret-metrics.txt`, await metricsResponse.text(), { flag: "wx", mode: 0o600 });

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
    throw new Error(`secret surface request failed: ${method} ${path} (${response.status})`);
  }
  return envelope.data;
}

function writeExclusive(name, value) {
  writeFileSync(`${artifactDirectory}/${name}`, `${JSON.stringify(value, null, 2)}\n`, { flag: "wx", mode: 0o600 });
}

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}
