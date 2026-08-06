import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const generatedDirectory = path.resolve(
  repositoryRoot,
  "src/services/hotkey/hotkey-server",
);
const schemaSource =
  process.env.HOTKEY_OPENAPI_SCHEMA ??
  path.resolve(repositoryRoot, "../docs/openapi/swagger.json");

function snapshotDirectory(directory) {
  if (!existsSync(directory)) return new Map();

  const snapshot = new Map();
  const pending = [directory];
  while (pending.length > 0) {
    const current = pending.pop();
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const absolutePath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        pending.push(absolutePath);
        continue;
      }
      const relativePath = path.relative(directory, absolutePath);
      const digest = createHash("sha256").update(readFileSync(absolutePath)).digest("hex");
      snapshot.set(relativePath, digest);
    }
  }
  return snapshot;
}

function changedFiles(before, after) {
  const files = new Set([...before.keys(), ...after.keys()]);
  return [...files].filter((file) => before.get(file) !== after.get(file)).sort();
}

async function assertOpenAPIContract() {
  const schema = /^https?:\/\//.test(schemaSource)
    ? await fetch(schemaSource, { signal: AbortSignal.timeout(10_000) }).then(
        async (response) => {
          if (!response.ok) {
            throw new Error(`OpenAPI contract unavailable: HTTP ${response.status}`);
          }
          return response.json();
        },
      )
    : JSON.parse(readFileSync(schemaSource, "utf8"));
  const supportedVersion =
    (typeof schema.openapi === "string" && schema.openapi.startsWith("3.")) ||
    schema.swagger === "2.0";
  if (!supportedVersion || !schema.paths || Object.keys(schema.paths).length === 0) {
    throw new Error("OpenAPI contract is invalid or contains no paths");
  }
}

try {
  await assertOpenAPIContract();
  const before = snapshotDirectory(generatedDirectory);
  const npmCLI = process.env.npm_execpath;

  console.log("Running npm run openapi:generate with the official @umijs/openapi CLI...");
  execFileSync(npmCLI ? process.execPath : "npm", npmCLI ? [npmCLI, "run", "openapi:generate"] : ["run", "openapi:generate"], {
    cwd: repositoryRoot,
    env: { ...process.env, HOTKEY_OPENAPI_SCHEMA: schemaSource },
    stdio: "inherit",
  });

  const after = snapshotDirectory(generatedDirectory);
  const changes = changedFiles(before, after);
  if (changes.length > 0) {
    console.error("Generated OpenAPI client was out of date:");
    for (const file of changes) console.error(`- ${file}`);
    process.exitCode = 1;
  } else {
    console.log("OpenAPI contract and generated client are in sync.");
  }
} catch (error) {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
}
