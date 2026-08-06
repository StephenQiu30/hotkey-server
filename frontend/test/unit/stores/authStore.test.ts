import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/services/hotkey/hotkey-server/identity", () => ({
  postAuthLogin: vi.fn(),
  postAuthLogout: vi.fn(),
  getAuthMe: vi.fn(),
  postAuthRefresh: vi.fn(),
}));

vi.mock("@/lib/authSession", () => ({
  setAccessToken: vi.fn(),
  clearAccessToken: vi.fn(),
  getAccessToken: vi.fn(() => ""),
  isAccessTokenExpired: vi.fn(() => true),
}));

import { useAuthStore } from "@/stores/authStore";
import * as authService from "@/services/hotkey/hotkey-server/identity";
import * as authSession from "@/lib/authSession";
import { AuthStatus } from "@/lib/domainEnums";

beforeEach(() => {
  vi.mocked(authService.postAuthLogin).mockReset();
  vi.mocked(authService.postAuthLogout).mockReset();
  vi.mocked(authService.getAuthMe).mockReset();
  vi.mocked(authService.postAuthRefresh).mockReset();
  vi.mocked(authSession.setAccessToken).mockReset();
  vi.mocked(authSession.clearAccessToken).mockReset();
  vi.mocked(authSession.getAccessToken).mockReset().mockReturnValue("");
  localStorage.clear();
  useAuthStore.setState({ status: AuthStatus.Initializing, user: null, error: null });
});

describe("auth store state machine", () => {
  it("starts in initializing state with no user", () => {
    const state = useAuthStore.getState();
    expect(state.status).toBe("initializing");
    expect(state.user).toBeNull();
  });

  it("initialize transitions to authenticated when me succeeds", async () => {
    vi.mocked(authSession.getAccessToken).mockReturnValueOnce("valid-token");
    vi.mocked(authSession.isAccessTokenExpired).mockReturnValueOnce(false);

    const userData: HotKeyAPI.UserResponse = {
      id: 1,
      email: "a@b.com",
      display_name: "Alice",
    };
    vi.mocked(authService.getAuthMe).mockResolvedValueOnce({
      data: userData,
    } as any);

    await useAuthStore.getState().initialize();

    expect(useAuthStore.getState().status).toBe("authenticated");
    expect(useAuthStore.getState().user).toEqual(userData);
  });

  it("attempts cookie refresh when no in-memory access token exists", async () => {
    vi.mocked(authSession.getAccessToken).mockReturnValueOnce("");
    vi.mocked(authService.postAuthRefresh).mockRejectedValueOnce(new Error("no session"));
    useAuthStore.setState({ status: AuthStatus.Initializing, user: null, error: null });

    await useAuthStore.getState().initialize();

    expect(authService.postAuthRefresh).toHaveBeenCalledOnce();
    expect(authService.getAuthMe).not.toHaveBeenCalled();
    expect(useAuthStore.getState().status).toBe(AuthStatus.Unauthenticated);
  });

  it("restores a session from its HttpOnly refresh cookie", async () => {
    const user = {
      id: 1,
      email: "a@b.com",
      display_name: "Alice",
    } as HotKeyAPI.UserResponse;
    vi.mocked(authSession.getAccessToken).mockReturnValueOnce("");
    vi.mocked(authService.postAuthRefresh).mockResolvedValueOnce({
      data: { access_token: "refreshed-token", user },
    } as any);
    vi.mocked(authService.getAuthMe).mockResolvedValueOnce({ data: user } as any);

    await useAuthStore.getState().initialize();

    expect(authService.postAuthRefresh).toHaveBeenCalledOnce();
    expect(authSession.setAccessToken).toHaveBeenCalledWith("refreshed-token", 900);
    expect(authService.getAuthMe).toHaveBeenCalledOnce();
    expect(useAuthStore.getState().status).toBe(AuthStatus.Authenticated);
  });

  it("does not persist identity state in localStorage", async () => {
    const user = { id: 1, email: "a@b.com", display_name: "Alice" } as HotKeyAPI.UserResponse;
    vi.mocked(authService.postAuthLogin).mockResolvedValueOnce({
      data: { access_token: "tok1", user },
    } as any);
    vi.mocked(authService.getAuthMe).mockResolvedValueOnce({ data: user } as any);

    await useAuthStore.getState().login({ email: "a@b.com", password: "pass123" });

    expect(localStorage.getItem("hk-auth-storage")).toBeNull();
  });

  it("initialize transitions to unauthenticated when me fails", async () => {
    vi.mocked(authSession.getAccessToken).mockReturnValueOnce("valid-token");
    vi.mocked(authSession.isAccessTokenExpired).mockReturnValueOnce(false);

    vi.mocked(authService.getAuthMe).mockRejectedValueOnce(new Error("no session"));

    await useAuthStore.getState().initialize();

    expect(useAuthStore.getState().status).toBe("unauthenticated");
    expect(useAuthStore.getState().user).toBeNull();
  });

  it("login stores token, fetches me, and sets authenticated", async () => {
    const userData: HotKeyAPI.UserResponse = {
      id: 1,
      email: "a@b.com",
      display_name: "Alice",
    };

    vi.mocked(authService.postAuthLogin).mockResolvedValueOnce({
      data: { access_token: "tok1", user: { id: 1, email: "a@b.com", display_name: "Alice" } },
    } as any);

    vi.mocked(authService.getAuthMe).mockResolvedValueOnce({
      data: userData,
    } as any);

    await useAuthStore.getState().login({
      email: "a@b.com",
      password: "pass123",
    });

    expect(authService.postAuthLogin).toHaveBeenCalledWith({
      email: "a@b.com",
      password: "pass123",
    });
    expect(authSession.setAccessToken).toHaveBeenCalledWith("tok1", 900);
    expect(authService.getAuthMe).toHaveBeenCalled();
    expect(useAuthStore.getState().status).toBe("authenticated");
    expect(useAuthStore.getState().user).toEqual(userData);
  });

  it("establishes the session returned by registration without another login", async () => {
    const userData = { id: 2, email: "new@example.com", display_name: "New" } as HotKeyAPI.UserResponse;
    vi.mocked(authService.getAuthMe).mockResolvedValueOnce({ data: userData } as any);

    await useAuthStore.getState().establishSession({
      access_token: "registered-token",
      user: { id: 2, email: "new@example.com", display_name: "New" },
    });

    expect(authService.postAuthLogin).not.toHaveBeenCalled();
    expect(authSession.setAccessToken).toHaveBeenCalledWith("registered-token", 900);
    expect(authService.getAuthMe).toHaveBeenCalledOnce();
    expect(useAuthStore.getState().user).toEqual(userData);
  });

  it("login sets error on invalid credentials", async () => {
    vi.mocked(authService.postAuthLogin).mockRejectedValueOnce({
      message: "邮箱或密码错误",
      code: 401,
    });

    await expect(
      useAuthStore.getState().login({ email: "a@b.com", password: "wrong" }),
    ).rejects.toThrow();

    expect(useAuthStore.getState().status).toBe("unauthenticated");
    expect(useAuthStore.getState().error).toBe("邮箱或密码错误");
  });

  it("logout calls API and clears state", async () => {
    useAuthStore.setState({ status: AuthStatus.Authenticated, user: { id: 1, email: "a@b.com" } as any });
    vi.mocked(authService.postAuthLogout).mockResolvedValueOnce({ code: 0, message: "success" } as any);

    await useAuthStore.getState().logout();

    expect(authService.postAuthLogout).toHaveBeenCalled();
    expect(authSession.clearAccessToken).toHaveBeenCalled();
    expect(useAuthStore.getState().status).toBe("unauthenticated");
    expect(useAuthStore.getState().user).toBeNull();
  });

  it("logout is idempotent when already unauthenticated", async () => {
    useAuthStore.setState({ status: AuthStatus.Unauthenticated, user: null });

    await useAuthStore.getState().logout();

    expect(authService.postAuthLogout).not.toHaveBeenCalled();
    expect(useAuthStore.getState().status).toBe("unauthenticated");
  });

  it("clearSession resets to unauthenticated", () => {
    useAuthStore.setState({ status: AuthStatus.Authenticated, user: { id: 1 } as any });

    useAuthStore.getState().clearSession();

    expect(authSession.clearAccessToken).toHaveBeenCalled();
    expect(useAuthStore.getState().status).toBe("unauthenticated");
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().error).toBeNull();
  });
});
