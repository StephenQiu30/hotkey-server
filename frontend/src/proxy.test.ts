import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";

import { proxy } from "./proxy";

describe("protected workbench navigation", () => {
  it("redirects a missing session cookie to login", () => {
    const response = proxy(new NextRequest("https://hotkey.test/events"));

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("https://hotkey.test/login");
    expect(response.headers.get("content-security-policy")).toContain(
      "default-src 'self'",
    );
  });

  it("lets a cookie-bearing request reach the page for API verification", () => {
    const request = new NextRequest("https://hotkey.test/events", {
      headers: { cookie: "hotkey_session=unverified" },
    });

    const response = proxy(request);

    expect(response.status).toBe(200);
    expect(response.headers.get("x-middleware-next")).toBe("1");
  });
});
