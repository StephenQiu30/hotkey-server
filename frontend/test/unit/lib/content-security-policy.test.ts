import { describe, expect, it } from "vitest";
import nextConfig from "../../../next.config";

describe("Content Security Policy", () => {
  it("serves every page with a report-safe browser fallback", async () => {
    expect(nextConfig.headers).toBeTypeOf("function");

    const rules = await nextConfig.headers!();
    const globalRule = rules.find((rule) => rule.source === "/:path*");
    const policy = globalRule?.headers.find(
      (header) => header.key.toLowerCase() === "content-security-policy",
    )?.value;

    expect(policy).toBeDefined();
    expect(policy).toContain("default-src 'self'");
    expect(policy).toContain("script-src 'self' 'unsafe-inline'");
    expect(policy).toContain("script-src-attr 'none'");
    expect(policy).toContain("object-src 'none'");
    expect(policy).toContain("base-uri 'self'");
    expect(policy).toContain("frame-ancestors 'none'");
    expect(policy).not.toContain("'unsafe-eval'");
    expect(policy).not.toMatch(/[\r\n]/);
  });
});
