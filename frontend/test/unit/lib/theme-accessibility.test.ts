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
  it("keeps muted text at WCAG AA contrast on shared light surfaces", () => {
    const muted = css.match(/--muted-foreground:\s*(#[0-9a-f]{6})/i)?.[1];
    expect(muted).toBeTruthy();
    expect(contrast(muted!, "#f5f8fd")).toBeGreaterThanOrEqual(4.5);
    expect(contrast(muted!, "#f8fafc")).toBeGreaterThanOrEqual(4.5);
  });
});
