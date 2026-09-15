import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

function verify(source) {
  return () =>
    execFileSync(process.execPath, ["scripts/check-boundaries.mjs"], {
      env: { ...process.env, HOTKEY_FRONTEND_SOURCE: source },
      stdio: "pipe",
    });
}

const temporary = mkdtempSync(join(tmpdir(), "hotkey-boundaries-"));
try {
  const valid = join(temporary, "valid");
  mkdirSync(join(valid, "api"), { recursive: true });
  writeFileSync(join(valid, "main.tsx"), "import './api/jobs';\n");
  writeFileSync(join(valid, "request.ts"), "export const request = 1;\n");
  writeFileSync(
    join(valid, "api/jobs.ts"),
    "import { request } from '../request'; export { request };\n",
  );
  assert.doesNotThrow(verify(valid));

  const invalidImport = join(temporary, "invalid-import");
  mkdirSync(join(invalidImport, "features"), { recursive: true });
  writeFileSync(
    join(invalidImport, "features/value.ts"),
    "export const value = 1;\n",
  );
  writeFileSync(
    join(invalidImport, "request.ts"),
    "import { value } from './features/value'; export { value };\n",
  );
  assert.throws(verify(invalidImport), /request cannot import features/);

  const unknownLayer = join(temporary, "unknown-layer");
  mkdirSync(join(unknownLayer, "misc"), { recursive: true });
  writeFileSync(
    join(unknownLayer, "misc/value.ts"),
    "export const value = 1;\n",
  );
  assert.throws(verify(unknownLayer), /Unknown frontend layer: misc/);
} finally {
  rmSync(temporary, { recursive: true, force: true });
}
