import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";

const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/;
const ALLOWED_EXTERNAL_PROTOCOLS = new Set(["http:", "https:"]);

export function toSafeExternalURL(value: string | null | undefined) {
  const candidate = value?.trim();
  if (!candidate || CONTROL_CHARACTERS.test(candidate)) return undefined;

  try {
    const parsed = new URL(candidate);
    return ALLOWED_EXTERNAL_PROTOCOLS.has(parsed.protocol)
      ? parsed.toString()
      : undefined;
  } catch {
    return undefined;
  }
}

type SafeExternalLinkProps = Omit<
  AnchorHTMLAttributes<HTMLAnchorElement>,
  "href" | "rel" | "target"
> & {
  fallback?: ReactNode;
  href?: string | null;
};

export const SafeExternalLink = forwardRef<
  HTMLAnchorElement,
  SafeExternalLinkProps
>(function SafeExternalLink(
  { children, className, fallback, href, title, ...props },
  ref,
) {
  const safeURL = toSafeExternalURL(href);
  if (!safeURL) {
    return (
      <span className={className} title={title}>
        {fallback ?? children}
      </span>
    );
  }

  return (
    <a
      {...props}
      className={className}
      href={safeURL}
      ref={ref}
      rel="noopener noreferrer"
      target="_blank"
      title={title}
    >
      {children}
    </a>
  );
});
