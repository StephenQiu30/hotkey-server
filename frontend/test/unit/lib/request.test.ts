import { describe, it, expect, beforeEach } from "vitest";
import type { AxiosAdapter } from "axios";

// -- authSession.ts unit tests ---------------------------------------

describe("authSession", () => {
  it("stores and retrieves access token", async () => {
    const { setAccessToken, getAccessToken, clearAccessToken } = await import("@/lib/authSession");
    clearAccessToken();
    setAccessToken("tok1", 3600);
    expect(getAccessToken()).toBe("tok1");
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
});

// -- HotKeyAPIError ----------------------------------------------------

describe("HotKeyAPIError", () => {
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
