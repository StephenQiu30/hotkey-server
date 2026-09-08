import { execFileSync } from "node:child_process";
import { readFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
const dir = mkdtempSync(join(tmpdir(), "hotkey-contract-"));
try {
  const path = join(dir, "generated.ts");
  execFileSync("node", [
    "node_modules/openapi-typescript/bin/cli.js",
    "../docs/openapi/openapi.json",
    "-o",
    path,
  ]);
  if (
    readFileSync(path, "utf8") !== readFileSync("src/api.generated.ts", "utf8")
  )
    throw new Error("OpenAPI client is stale: npm run generate");
} finally {
  rmSync(dir, { recursive: true, force: true });
}
