import { appendFileSync, readFileSync, writeFileSync } from "node:fs";
import { randomBytes } from "node:crypto";
import { pathToFileURL } from "node:url";
import process from "node:process";

export function generateSyntheticCanaries(random = randomBytes) {
  const hex = () => random(24).toString("hex");
  return {
    HOTKEY_JWT_SECRET: `hkci_jwt_${hex()}`,
    HOTKEY_VERIFICATION_HMAC_SECRET: `hkci_hmac_${hex()}`,
    POSTGRES_PASSWORD: `hkci_database_${hex()}`,
    MINIO_ROOT_PASSWORD: `hkci_minio_${hex()}`,
    HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY: random(32).toString("base64"),
    HOTKEY_AGENT_AUTH_TOKEN: `hkci_agent_${hex()}`,
    HOTKEY_AGENT_MODEL_API_KEY: `hkci_model_${hex()}`,
    HOTKEY_SMTP_PASSWORD: `hkci_smtp_${hex()}`,
    HOTKEY_TEST_SOURCE_TOKEN: `hkci_source_${hex()}`,
    HOTKEY_TEST_COOKIE_SECRET: `hkci_cookie_${hex()}`,
  };
}

export function persistSyntheticCanaries(canaries, githubEnvironmentPath, backendEnvironmentPath, mask = () => {}) {
  if (!githubEnvironmentPath || !backendEnvironmentPath) {
    throw new Error("synthetic secret output paths are required");
  }
  const entries = Object.entries(canaries);
  if (entries.length !== 10 || new Set(entries.map(([, value]) => value)).size !== entries.length) {
    throw new Error("synthetic secret canary set is incomplete or reused");
  }
  for (const [, value] of entries) {
    if (typeof value !== "string" || value.length < 24 || /[\r\n]/.test(value)) {
      throw new Error("synthetic secret canary is invalid");
    }
    mask(value);
  }
  appendFileSync(
    githubEnvironmentPath,
    entries.map(([name, value]) => `${name}=${value}\n`).join(""),
    { mode: 0o600 },
  );

  let environment = readFileSync(backendEnvironmentPath, "utf8");
  for (const name of [
    "HOTKEY_JWT_SECRET",
    "HOTKEY_VERIFICATION_HMAC_SECRET",
    "HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY",
    "HOTKEY_SMTP_PASSWORD",
  ]) {
    environment = setEnvironmentValue(environment, name, canaries[name]);
  }
  writeFileSync(backendEnvironmentPath, environment, { mode: 0o600 });
}

function setEnvironmentValue(contents, name, value) {
  const active = new RegExp(`^${name}=.*$`, "m");
  if (active.test(contents)) {
    return contents.replace(active, `${name}=${value}`);
  }
  return `${contents.replace(/\n?$/, "\n")}${name}=${value}\n`;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const githubEnvironmentPath = process.argv[2];
  const backendEnvironmentPath = process.argv[3];
  persistSyntheticCanaries(
    generateSyntheticCanaries(),
    githubEnvironmentPath,
    backendEnvironmentPath,
    (value) => process.stdout.write(`::add-mask::${value}\n`),
  );
}
