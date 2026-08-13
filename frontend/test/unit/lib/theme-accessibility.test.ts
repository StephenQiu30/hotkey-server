import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(join(process.cwd(), "src/app/globals.css"), "utf8");

function channel(value: number) {
  const normalized = value / 255;
  return normalized <= 0.04045
    ? normalized / 12.92
    : ((normalized + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string) {
  const value = hex.replace("#", "");
  return 0.2126 * channel(Number.parseInt(value.slice(0, 2), 16))
    + 0.7152 * channel(Number.parseInt(value.slice(2, 4), 16))
    + 0.0722 * channel(Number.parseInt(value.slice(4, 6), 16));
}

function contrast(foreground: string, background: string) {
  const [lighter, darker] = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
  return (lighter + 0.05) / (darker + 0.05);
}

describe("dashboard theme accessibility", () => {
  it("reserves a stable document scrollbar gutter across dashboard routes", () => {
    expect(css).toMatch(/html\s*{[\s\S]*?scrollbar-gutter:\s*stable;/);
  });

  it("binds Tailwind typography tokens to the fonts loaded by next/font", () => {
    expect(css).toContain("var(--font-geist-sans)");
    expect(css).toContain("var(--font-geist-mono)");
  });

  it.each([
    ["light", css.match(/:root\s*{([\s\S]*?)}/)?.[1]],
    ["dark", css.match(/\.dark\s*{([\s\S]*?)}/)?.[1]],
  ])("keeps muted text at WCAG AA contrast in the %s theme", (_, theme) => {
    expect(theme).toBeTruthy();
    const muted = theme!.match(/--muted-foreground:\s*(#[0-9a-f]{6})/i)?.[1];
    const background = theme!.match(/--background:\s*(#[0-9a-f]{6})/i)?.[1];
    const secondary = theme!.match(/--secondary:\s*(#[0-9a-f]{6})/i)?.[1];
    expect(muted).toBeTruthy();
    expect(background).toBeTruthy();
    expect(secondary).toBeTruthy();
    expect(contrast(muted!, background!)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(muted!, secondary!)).toBeGreaterThanOrEqual(4.5);
  });

  it("provides a global reduced-motion fallback", () => {
    expect(css).toContain("@media (prefers-reduced-motion: reduce)");
    expect(css).toContain("transition-duration: 0.01ms !important");
  });

  it("keeps explicit card separators and exposes an accessible skip link", () => {
    expect(css).not.toContain(
      '[data-slot="card"] > [data-slot="card-header"]'
    );
    expect(css).toContain(".skip-link");
    expect(css).toContain(".skip-link:focus-visible");
  });
});
