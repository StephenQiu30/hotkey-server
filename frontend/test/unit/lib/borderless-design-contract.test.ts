import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const sourceRoot = join(process.cwd(), "src");

const borderlessSurfaces = [
  "app/page.tsx",
  "app/dashboard/page.tsx",
  "app/dashboard/contents/page.tsx",
  "components/PublicHeader.tsx",
  "components/auth/AuthShell.tsx",
  "components/dashboard/HotspotCard.tsx",
  "components/dashboard/TopNav.tsx",
  "components/home/HomeHero.tsx",
  "components/home/SignalOrbitScene.tsx",
  "layouts/BasicLayout.tsx",
];

describe("borderless visual contract", () => {
  it.each(borderlessSurfaces)("keeps %s free from decorative lines and ring shadows", (file) => {
    const source = readFileSync(join(sourceRoot, file), "utf8");

    expect(source).not.toMatch(/shadow-border/);
    expect(source).not.toMatch(/\bborder-(?:b|t|l|r|x|y)\b/);
    expect(source).not.toMatch(/\bdivide-[xy]\b/);
    expect(source).not.toMatch(/box-shadow:[^\]]*0_0_0_1px/);
  });

  it("uses soft surfaces instead of simulated outlines in shared primitives", () => {
    const files = [
      "components/ui/alert.tsx",
      "components/ui/badge.tsx",
      "components/ui/button.tsx",
      "components/ui/skeleton.tsx",
      "components/ui/surface.tsx",
    ];

    for (const file of files) {
      const source = readFileSync(join(sourceRoot, file), "utf8");
      expect(source, file).not.toContain("shadow-border");
      expect(source, file).not.toMatch(/\bring-1\b/);
    }
  });
});
