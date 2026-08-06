/**
 * In-memory access-token storage with single-flight refresh.
 *
 * Page reloads intentionally discard the token. The Auth store restores a valid
 * session through the server-managed HttpOnly refresh cookie instead of exposing
 * long-lived credentials to JavaScript-readable browser storage.
 */

let accessToken = "";
let expiresAt = 0;
let refreshPromise: Promise<string> | null = null;

/** Store a new access token in memory. */
export function setAccessToken(token: string, expiresIn: number): void {
  accessToken = token;
  expiresAt = Date.now() + expiresIn * 1000;
}

/** Clear the in-memory access token. */
export function clearAccessToken(): void {
  accessToken = "";
  expiresAt = 0;
}

/** Read the current in-memory access token. */
export function getAccessToken(): string {
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
