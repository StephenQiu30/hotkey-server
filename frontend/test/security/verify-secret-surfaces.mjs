import process from "node:process";
import { scanSecretSurfaces, writeSecretScanReport } from "./secret-surface-scan.mjs";

const artifactDirectory = required("HOTKEY_BROWSER_ARTIFACT_DIR");
const result = scanSecretSurfaces({
  canaries: [
    ["jwt", "HOTKEY_JWT_SECRET"],
    ["verification_hmac", "HOTKEY_VERIFICATION_HMAC_SECRET"],
    ["database_dsn", "POSTGRES_PASSWORD"],
    ["minio", "MINIO_ROOT_PASSWORD"],
    ["source_master_key", "HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY"],
    ["agent_auth", "HOTKEY_AGENT_AUTH_TOKEN"],
    ["model_api_key", "HOTKEY_AGENT_MODEL_API_KEY"],
    ["smtp", "HOTKEY_SMTP_PASSWORD"],
    ["source_token", "HOTKEY_TEST_SOURCE_TOKEN"],
    ["cookie", "HOTKEY_TEST_COOKIE_SECRET"],
    ["user_password", "HOTKEY_BROWSER_PASSWORD"],
  ].map(([id, name]) => ({ id, value: required(name) })),
  surfaces: [
    { id: "container_logs", path: `${artifactDirectory}/hotkey-container-logs.txt` },
    { id: "http_error", path: `${artifactDirectory}/hotkey-secret-http-error.json` },
    { id: "metrics", path: `${artifactDirectory}/hotkey-secret-metrics.txt` },
    { id: "source_response", path: `${artifactDirectory}/hotkey-secret-source-response.json` },
    { id: "database_operational", path: `${artifactDirectory}/hotkey-secret-database-surfaces.txt` },
    { id: "frontend_bundle", path: `${artifactDirectory}/hotkey-frontend-runtime` },
    { id: "vault_delivery", path: `${artifactDirectory}/hotkey-vault-delivery.md` },
    { id: "acceptance_attachments", path: `${artifactDirectory}/hotkey-acceptance-attachments` },
    { id: "openapi", path: "docs/openapi/swagger.json" },
    { id: "documentation", path: "docs" },
  ],
});

const reportPath = `${artifactDirectory}/hotkey-secret-surface-scan.json`;
writeSecretScanReport(reportPath, result, required("HOTKEY_BROWSER_RUN_ID"));
if (result.leaks.length !== 0) {
  throw new Error(`synthetic secret scan found ${result.leaks.length} leak(s); inspect the sanitized report`);
}

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}
