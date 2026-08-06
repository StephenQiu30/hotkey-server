const DEFAULT_REDIRECT = "/dashboard";
const REDIRECT_ORIGIN = "https://hotkey.local";

/** Validate an allowlisted, same-origin dashboard return target. */
export function safeRedirect(value: string | null): string {
  if (!value || !value.startsWith("/") || value.startsWith("//") || value.includes("\\")) {
    return DEFAULT_REDIRECT;
  }

  try {
    const target = new URL(value, REDIRECT_ORIGIN);
    const isDashboardPath =
      target.pathname === DEFAULT_REDIRECT || target.pathname.startsWith(`${DEFAULT_REDIRECT}/`);
    if (target.origin !== REDIRECT_ORIGIN || !isDashboardPath) {
      return DEFAULT_REDIRECT;
    }
    return `${target.pathname}${target.search}${target.hash}`;
  } catch {
    return DEFAULT_REDIRECT;
  }
}

/** Build the login URL for a protected page without creating an open redirect. */
export function createLoginRedirect(pathname: string, search = ""): string {
  const target = safeRedirect(`${pathname}${search}`);
  return `/login?redirect=${encodeURIComponent(target)}`;
}
