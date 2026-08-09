import { describe, it, expect, vi } from "vitest";
import type { AxiosAdapter } from "axios";

vi.mock("@/services/hotkey/hotkey-server/identity", () => ({
  postAuthRefresh: vi.fn(),
}));

// -- authSession.ts unit tests ---------------------------------------

describe("authSession", () => {
  it("stores and retrieves the access token in memory only", async () => {
    const { setAccessToken, getAccessToken, clearAccessToken } = await import("@/lib/authSession");
    clearAccessToken();
    setAccessToken("tok1", 3600);
    expect(getAccessToken()).toBe("tok1");
    expect(localStorage.getItem("hk_access_token")).toBeNull();
    clearAccessToken();
    expect(getAccessToken()).toBe("");
  });

  it("detects expired token", async () => {
    const { setAccessToken, isAccessTokenExpired, clearAccessToken } = await import("@/lib/authSession");
    clearAccessToken();
    expect(isAccessTokenExpired()).toBe(true);
    setAccessToken("tok", 3600);
    expect(isAccessTokenExpired()).toBe(false);
  });

  it("supports single-flight refresh promise", async () => {
    const { refreshAccessToken, resetRefreshPromise } = await import("@/lib/authSession");
    resetRefreshPromise();
    let calls = 0;
    const p1 = refreshAccessToken(async () => { calls++; return "tok-a"; });
    const p2 = refreshAccessToken(async () => { calls++; return "tok-b"; });
    const [r1, r2] = await Promise.all([p1, p2]);
    expect(r1).toBe("tok-a");
    expect(r2).toBe("tok-a"); // same promise shared
    expect(calls).toBe(1);
  });

  it("serializes refreshes with the browser-wide authentication lock", async () => {
    const request = vi.fn(async (_name: string, callback: () => Promise<string>) =>
      callback(),
    );
    Object.defineProperty(navigator, "locks", {
      configurable: true,
      value: { request },
    });

    const { refreshAccessToken, resetRefreshPromise } = await import("@/lib/authSession");
    resetRefreshPromise();

    await expect(refreshAccessToken(async () => "locked-token")).resolves.toBe(
      "locked-token",
    );
    expect(request).toHaveBeenCalledWith(
      "hotkey-auth-refresh",
      expect.any(Function),
    );

    Object.defineProperty(navigator, "locks", {
      configurable: true,
      value: undefined,
    });
  });
});

describe("safe authentication redirects", () => {
  it("keeps an allowed dashboard return target", async () => {
    const { safeRedirect, createLoginRedirect } = await import("@/lib/safeRedirect");

    expect(safeRedirect("/dashboard/events?status=active#latest")).toBe(
      "/dashboard/events?status=active#latest",
    );
    expect(createLoginRedirect("/dashboard/events", "?status=active")).toBe(
      "/login?redirect=%2Fdashboard%2Fevents%3Fstatus%3Dactive",
    );
    expect(safeRedirect("/dashboard-evil")).toBe("/dashboard");
  });

  it.each([
    null,
    "https://evil.example/dashboard",
    "//evil.example/dashboard",
    "/\\evil.example/dashboard",
    "/login?redirect=/dashboard",
    "/settings",
  ])("rejects unsafe or unsupported return target %s", async (target) => {
    const { safeRedirect } = await import("@/lib/safeRedirect");
    expect(safeRedirect(target)).toBe("/dashboard");
  });

  it("does not mistake every absolute path for the public home page", async () => {
    const { shouldSkipAuthRedirect } = await import("@/lib/request");

    expect(shouldSkipAuthRedirect("/")).toBe(true);
    expect(shouldSkipAuthRedirect("/login")).toBe(true);
    expect(shouldSkipAuthRedirect("/dashboard/events")).toBe(false);
  });
});

// -- HotKeyAPIError ----------------------------------------------------

describe("HotKeyAPIError", () => {
  it("serializes OpenAPI csv arrays without Axios bracket suffixes", async () => {
    const { serializeQueryParams } = await import("@/lib/request");

    expect(
      serializeQueryParams({
        lifecycle: ["active", "cooling"],
        trend: ["rising"],
        q: "Alpha 发布",
        cursor: undefined,
      }),
    ).toBe(
      "lifecycle=active%2Ccooling&trend=rising&q=Alpha+%E5%8F%91%E5%B8%83",
    );
  });

  it("carries HTTP status and Chinese message", async () => {
    const { HotKeyAPIError } = await import("@/lib/request");
    const err = new HotKeyAPIError(401, "邮箱或密码错误", null, 20002);
    expect(err.status).toBe(401);
    expect(err.code).toBe(20002);
    expect(err.message).toBe("邮箱或密码错误");
    expect(err.name).toBe("HotKeyAPIError");
  });

  it("maps stable backend error codes to user-facing Chinese messages", async () => {
    const { getUserFacingAPIErrorMessage } = await import("@/lib/apiErrorMessages");

    expect(getUserFacingAPIErrorMessage(20002, "invalid credentials")).toBe("邮箱或密码错误");
    expect(getUserFacingAPIErrorMessage(30001, "monitor version conflict")).toBe(
      "监控已被更新，请刷新后重试",
    );
    expect(getUserFacingAPIErrorMessage(12345, "自定义错误")).toBe("自定义错误");
  });

  it("sends generated request options through Axios and returns response data", async () => {
    const { request } = await import("@/lib/request");
    const adapter: AxiosAdapter = async (config) => ({
      config,
      data: { code: 0, data: { ok: true }, message: "ok" },
      headers: {},
      status: 200,
      statusText: "OK",
    });

    await expect(
      request<{ code: number; data: { ok: boolean }; message: string }>("/api/v1/test", {
        adapter,
        method: "POST",
        data: { source: "generated-client" },
      }),
    ).resolves.toEqual({ code: 0, data: { ok: true }, message: "ok" });
  });

  it("refreshes an expiring access token before a protected request", async () => {
    const authService = await import("@/services/hotkey/hotkey-server/identity");
    const { setAccessToken, clearAccessToken } = await import("@/lib/authSession");
    const { request } = await import("@/lib/request");
    vi.mocked(authService.postAuthRefresh).mockResolvedValueOnce({
      data: { access_token: "fresh-token" },
    } as any);
    clearAccessToken();
    setAccessToken("expiring-token", 0);

    let authorization = "";
    const adapter: AxiosAdapter = async (config) => {
      authorization = String(config.headers?.Authorization ?? "");
      return {
        config,
        data: { code: 0, data: { ok: true }, message: "ok" },
        headers: {},
        status: 200,
        statusText: "OK",
      };
    };

    await request("/api/v1/auth/me", { adapter, method: "GET" });

    expect(authService.postAuthRefresh).toHaveBeenCalledOnce();
    expect(authorization).toBe("Bearer fresh-token");
    clearAccessToken();
  });
});

describe("registration contract", () => {
  it("submits the verification ticket instead of the verified email", async () => {
    const { createRegisterRequest } = await import("@/lib/registerRequest");
    expect(createRegisterRequest("ticket-123", "Passw0rd!", "Alice")).toEqual({
      verification_ticket: "ticket-123",
      password: "Passw0rd!",
      display_name: "Alice",
    });
  });
});

describe("source health diagnostics", () => {
  it("turns stable diagnostic codes into actionable Chinese messages", async () => {
    const { getSourceHealthMessage } = await import("@/lib/sourceHealthMessages");

    expect(getSourceHealthMessage("destination_not_permitted")).toContain("Fake-IP");
    expect(getSourceHealthMessage("request_failed")).toBe("无法连接来源，请检查网络后重试");
    expect(getSourceHealthMessage("future_code")).toBe("来源暂不可用");
  });
});
