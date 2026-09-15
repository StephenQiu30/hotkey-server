import { execFileSync } from "node:child_process";
import {
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";

function files(root, directory = root) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? files(root, path) : [relative(root, path)];
  });
}

const temporary = mkdtempSync(join(tmpdir(), "hotkey-openapi-"));
try {
  execFileSync("node", ["node_modules/@umijs/openapi/dist/cli.js"], {
    env: { ...process.env, HOTKEY_OPENAPI_OUTPUT: temporary },
    stdio: "inherit",
  });
  const expected = join(temporary, "api");
  const actual = "src/api";
  if (!statSync(expected).isDirectory())
    throw new Error("OpenAPI generation failed");
  const expectedFiles = files(expected).sort();
  const actualFiles = files(actual).sort();
  if (JSON.stringify(expectedFiles) !== JSON.stringify(actualFiles)) {
    throw new Error("Generated API file list is stale: npm run generate");
  }
  for (const path of expectedFiles) {
    if (
      readFileSync(join(expected, path), "utf8") !==
      readFileSync(join(actual, path), "utf8")
    ) {
      throw new Error(`Generated API is stale: ${path}`);
    }
  }
} finally {
  rmSync(temporary, { recursive: true, force: true });
}
