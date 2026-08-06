/**
 * Access token storage with localStorage persistence and single-flight refresh.
 *
 * Token is stored both in memory (fast access) and localStorage (survives page reload).
 * On page reload, getAccessToken() falls back to localStorage transparently.
 */

const STORAGE_KEY = "hk_access_token";

let accessToken = "";
let expiresAt = 0;
let refreshPromise: Promise<string> | null = null;

/**
 * Store a new access token in memory and localStorage.
 */
export function setAccessToken(token: string, expiresIn: number): void {
  accessToken = token;
  expiresAt = Date.now() + expiresIn * 1000;
  try {
    localStorage.setItem(STORAGE_KEY, token);
  } catch {
    // localStorage unavailable (SSR / private mode) — memory-only fallback
  }
}

/**
 * Clear the access token from memory and localStorage.
 */
export function clearAccessToken(): void {
  accessToken = "";
  expiresAt = 0;
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch { /* ignore */ }
}

/**
 * Read the current access token.
 * Falls back to localStorage when the in-memory value is empty (page reload).
 */
export function getAccessToken(): string {
  if (!accessToken) {
    try {
      accessToken = localStorage.getItem(STORAGE_KEY) ?? "";
    } catch { /* ignore */ }
  }
  return accessToken;
}

/**
 * Returns true when no valid access token is held.
 */
export function isAccessTokenExpired(): boolean {
  if (!accessToken) return true;
  // Buffer by 10 s to avoid edge-of-expiry races
  return Date.now() + 10_000 >= expiresAt;
}

/**
 * Single-flight refresh: callers share one in-flight Promise.
 */
export function refreshAccessToken(
  perform: () => Promise<string>,
): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = perform().finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise;
}

/**
 * Reset the single-flight Promise (e.g. after logout).
 */
export function resetRefreshPromise(): void {
  refreshPromise = null;
}
