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

const findSourceFiles = (directory: string): string[] => {
  if (!fs.existsSync(directory)) return [];
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const resolved = path.join(directory, entry.name);
    if (entry.isDirectory()) return findSourceFiles(resolved);
    return /\.(ts|tsx)$/.test(entry.name) ? [resolved] : [];
  });
};

describe("repository test layout", () => {
  it("persists the HotKey borderless intelligence design contract at the project root", () => {
    const designContract = path.join(repositoryRoot, "..", "design.md");
    expect(fs.existsSync(designContract)).toBe(true);
    const design = fs.readFileSync(designContract, "utf8");
    expect(design).toContain("# HotKey 设计系统：中性情报界面");
    expect(design).toContain("### 无边框结构（硬性规则）");
    expect(design).toContain("不得使用可见描边、内描边、ring-shadow 或装饰分隔线");
  });

  it("keeps all test files under the repository test directory", () => {
    expect(findTestFiles(path.join(repositoryRoot, "src"))).toEqual([]);
    expect(fs.existsSync(path.join(repositoryRoot, "test", "setup.ts"))).toBe(true);
  });

  it("removes legacy redirect routes and references from the product workspace", () => {
    expect(fs.existsSync(path.join(repositoryRoot, "src/app/dashboard/topics/page.tsx"))).toBe(false);
    expect(fs.existsSync(path.join(repositoryRoot, "src/app/dashboard/favorites/page.tsx"))).toBe(false);
    const source = findSourceFiles(path.join(repositoryRoot, "src"))
      .map((file) => fs.readFileSync(file, "utf8"))
      .join("\n");
    expect(source).not.toMatch(/\/dashboard\/(topics|favorites)/);
  });

  it("provides route-level loading and recoverable error fallbacks", () => {
    expect(fs.existsSync(path.join(repositoryRoot, "src/app/dashboard/loading.tsx"))).toBe(true);
    expect(fs.existsSync(path.join(repositoryRoot, "src/app/dashboard/error.tsx"))).toBe(true);
  });

  it("gives every shadcn table a named keyboard-scroll region", () => {
    const missingLabels = findSourceFiles(path.join(repositoryRoot, "src"))
      .flatMap((file) => {
        const tags = [...fs.readFileSync(file, "utf8").matchAll(/<Table\b[^>]*>/gs)];
        return tags
          .filter(([tag]) => !tag.includes("scrollAreaLabel="))
          .map(() => path.relative(repositoryRoot, file));
      });
    expect(missingLabels).toEqual([]);
  });

  it("keeps dashboard page shells and perimeter surfaces on shared primitives", () => {
    const dashboardSource = [
      path.join(repositoryRoot, "src/app/dashboard"),
      path.join(repositoryRoot, "src/components/dashboard"),
    ]
      .flatMap(findSourceFiles)
      .map((file) => fs.readFileSync(file, "utf8"))
      .join("\n");

    expect(dashboardSource).not.toMatch(/className="[^"]*\bapp-page\b/);
    expect(dashboardSource).not.toContain("border border-border");
  });
});
