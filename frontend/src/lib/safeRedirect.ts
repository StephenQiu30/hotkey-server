/**
 * Validate a redirect target.
 *
 * Only same-origin relative paths are accepted. Absolute URLs and
 * protocol-relative URLs ("//evil.com") are rejected.
 */
export function safeRedirect(value: string | null): string {
  return value && value.startsWith("/") && !value.startsWith("//") ? value : "/dashboard";
}
