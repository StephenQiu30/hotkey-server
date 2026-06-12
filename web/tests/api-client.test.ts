import { describe, it, expect } from "vitest";
import { buildURL } from "@/lib/api";

describe("buildURL", () => {
  it("builds monitor detail url", () => {
    expect(buildURL("/api/v1/monitors/1")).toBe(
      "http://localhost:8080/api/v1/monitors/1"
    );
  });

  it("builds notifications url", () => {
    expect(buildURL("/api/v1/notifications")).toBe(
      "http://localhost:8080/api/v1/notifications"
    );
  });
});
