import axios, { AxiosError, type AxiosRequestConfig } from "axios";
import {
  getAccessToken,
  clearAccessToken,
  refreshAccessToken,
  resetRefreshPromise,
} from "./authSession";
import { getUserFacingAPIErrorMessage } from "./apiErrorMessages";

/** Pages that do NOT require authentication — skip redirect-to-login on 401. */
const PUBLIC_PATHS = ["/", "/login", "/register", "/forgot-password", "/reset-password"];

function shouldSkipRedirect(pathname: string): boolean {
  return PUBLIC_PATHS.some((p) => pathname === p || pathname.startsWith(p));
}

/**
 * Internal marker to prevent infinite refresh loops.
 * Requests already retried carry _retry: true on their config.
 */
const RETRY_MARKER = "_retry" as const;

/** Endpoints that must NEVER trigger an automatic token refresh. */
const NO_REFRESH_PATHS = [
  "/api/v1/auth/login",
  "/api/v1/auth/logout",
  "/api/v1/auth/refresh",
  "/api/v1/auth/registrations",
  "/api/v1/auth/password-resets/confirm",
  "/api/v1/auth/email-verifications",
  "/api/v1/auth/email-verifications/confirm",
];

function isNoRefreshPath(url: string = ""): boolean {
  return NO_REFRESH_PATHS.some((p) => url.startsWith(p));
}

export class HotKeyAPIError extends Error {
  constructor(
    public status: number,
    message: string = "操作失败，请稍后重试",
    public data: unknown = null,
    public code: number = status,
  ) {
    super(message);
    this.name = "HotKeyAPIError";
  }
}

export type RequestOptions = AxiosRequestConfig;

const apiClient = axios.create({
  baseURL: "",
  timeout: 15000,
  withCredentials: true, // HttpOnly Refresh Cookie
});

// ── Request interceptor: inject Bearer token from memory ───────────

apiClient.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// ── Response interceptor: envelope parsing + single-flight refresh ─

apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError<{ code?: number; message?: string; data?: unknown }>) => {
    // ── Network-level errors (no HTTP response received) ──────────
    // e.g. ECONNREFUSED, ERR_NETWORK, ERR_NAME_NOT_RESOLVED
    if (!error.response) {
      const networkMsg =
        error.code === "ERR_NETWORK" || error.message?.includes("ECONNREFUSED")
          ? "无法连接到服务器，请确认 hotkey-server 已启动"
          : `网络错误：${error.message ?? "请检查网络连接"}`;
      throw new HotKeyAPIError(0, networkMsg);
    }

    const status = error.response?.status ?? 0;
    const body = error.response?.data;
    const requestUrl = error.config?.url ?? "";
    const isRetry = (error.config as any)?.[RETRY_MARKER] === true;

    // Parse envelope: { code, message, data }
    const errorMessage = getUserFacingAPIErrorMessage(body?.code, body?.message);
    const errorData = body?.data ?? null;

    // 401 handling: refresh + retry (once per request)
    if (status === 401 && !isRetry && !isNoRefreshPath(requestUrl)) {
      try {
        // Single-flight: all concurrent 401s share one refresh
        const newToken = await refreshAccessToken(async () => {
          const { postAuthRefresh } = await import(
            "@/services/hotkey/hotkey-server/identity"
          );
          const res = await postAuthRefresh();
          const data = res.data ?? {};
          const token = data.access_token ?? "";
          if (!token) throw new Error("refresh returned no token");
          return token;
        });

        // Update the stored token
        (await import("./authSession")).setAccessToken(newToken, 900);

        // Retry the original request with the new token
        const retryConfig: Record<string, any> = {
          ...error.config,
          headers: { ...(error.config?.headers ?? {}) as any, Authorization: `Bearer ${newToken}` },
          _retry: true,
        };
        const retryResponse = await apiClient(retryConfig);
        return retryResponse;
      } catch (refreshError) {
        // Refresh failed — clear session
        clearAccessToken();
        resetRefreshPromise();
        if (typeof window !== "undefined" && !shouldSkipRedirect(window.location.pathname)) {
          window.location.href = "/login";
        }
        throw new HotKeyAPIError(401, "登录已过期，请重新登录");
      }
    }

    // 401 on auth endpoints or retry — fail immediately
    if (status === 401) {
      clearAccessToken();
      if (typeof window !== "undefined" && !shouldSkipRedirect(window.location.pathname)) {
        window.location.href = "/login";
      }
    }

    throw new HotKeyAPIError(status, errorMessage, errorData, body?.code ?? status);
  },
);

/**
 * Unified API request helper.
 *
 * Wraps the Axios instance which handles:
 * - Bearer token injection from memory
 * - HttpOnly refresh cookie (withCredentials)
 * - Automatic 401 → refresh → retry (single-flight)
 * - Unified envelope error parsing
 */
export async function request<T>(url: string, options?: RequestOptions): Promise<T> {
  const response = await apiClient({ url, ...options });
  return response.data;
}
