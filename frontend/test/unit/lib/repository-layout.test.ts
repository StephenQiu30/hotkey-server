import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const repositoryRoot = process.cwd();

const findTestFiles = (directory: string): string[] => {
  if (!fs.existsSync(directory)) return [];

  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const resolved = path.join(directory, entry.name);
    if (entry.isDirectory()) return findTestFiles(resolved);
    return /\.(test|spec)\.(ts|tsx)$/.test(entry.name) ? [resolved] : [];
  });
};

describe("repository test layout", () => {
  it("keeps all test files under the repository test directory", () => {
    expect(findTestFiles(path.join(repositoryRoot, "src"))).toEqual([]);
    expect(fs.existsSync(path.join(repositoryRoot, "test", "setup.ts"))).toBe(true);
  });
});
