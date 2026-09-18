import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { findDesignViolations } from "./check-design-system.mjs";

async function withFixture(source, assertion) {
  const root = await mkdtemp(path.join(os.tmpdir(), "hotkey-design-"));

  try {
    await mkdir(path.join(root, "src"), { recursive: true });
    await writeFile(path.join(root, "src/component.tsx"), source, "utf8");
    await writeFile(path.join(root, "README.md"), "# Fixture\n", "utf8");
    await writeFile(path.join(root, "DESIGN.md"), "# Fixture\n", "utf8");
    await assertion(await findDesignViolations(root));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

test("allows named scales and standard responsive variants", async () => {
  await withFixture(
    'export const className = "h-16 max-w-7xl sm:grid-cols-2 xl:gap-12 2xl:gap-16";',
    (violations) => assert.deepEqual(violations, []),
  );
});

test("rejects raw pixel values", async () => {
  await withFixture(
    ['export const style = "width: 24', 'px";'].join(""),
    (violations) => {
      assert.equal(violations.length, 1);
      assert.match(violations[0], /raw pixel value/);
    },
  );
});

test("rejects arbitrary layout classes", async () => {
  await withFixture('export const className = "h-[42rem]";', (violations) => {
    assert.equal(violations.length, 1);
    assert.match(violations[0], /arbitrary layout class/);
  });
});
