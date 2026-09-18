import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { validateBoundaries } from "./check-boundaries.mjs";

async function withFixture(files, assertion) {
  const root = await mkdtemp(path.join(os.tmpdir(), "hotkey-boundaries-"));

  try {
    await Promise.all(
      Object.entries(files).map(async ([relativePath, contents]) => {
        const target = path.join(root, relativePath);
        await mkdir(path.dirname(target), { recursive: true });
        await writeFile(target, contents, "utf8");
      }),
    );
    await assertion(await validateBoundaries(root));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

test("allows app routes to compose a feature", async () => {
  await withFixture(
    {
      "app/page.tsx": 'import { Feature } from "@/features/monitor/feature";',
      "features/monitor/feature.tsx": "export const Feature = null;",
    },
    (violations) => assert.deepEqual(violations, []),
  );
});

test("rejects a feature importing an app route", async () => {
  await withFixture(
    {
      "app/page.tsx": "export default function Page() {}",
      "features/monitor/feature.tsx": 'import Page from "@/app/page";',
    },
    (violations) => {
      assert.equal(violations.length, 1);
      assert.match(violations[0], /features cannot import app routes/);
    },
  );
});

test("rejects direct imports between features", async () => {
  await withFixture(
    {
      "features/events/view.tsx": 'import "@/features/monitor/model";',
      "features/monitor/model.ts": "export const monitor = true;",
    },
    (violations) => {
      assert.equal(violations.length, 1);
      assert.match(violations[0], /cannot import another feature/);
    },
  );
});
