import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";

const DEFAULT_API_ORIGIN = "http://127.0.0.1:8867";

function getApiOrigin(): URL {
  const origin = new URL(process.env.HOTKEY_API_ORIGIN ?? DEFAULT_API_ORIGIN);

  if (origin.protocol !== "http:" && origin.protocol !== "https:") {
    throw new Error("HOTKEY_API_ORIGIN must use http or https");
  }

  return origin;
}

function rewriteApiRequest(request: NextRequest): NextResponse {
  const destination = new URL(request.nextUrl.pathname, getApiOrigin());
  destination.search = request.nextUrl.search;

  return NextResponse.rewrite(destination);
}

function createContentSecurityPolicy(nonce: string): string {
  const isDevelopment = process.env.NODE_ENV === "development";

  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${isDevelopment ? " 'unsafe-eval'" : ""}`,
    `style-src 'self'${isDevelopment ? " 'unsafe-inline'" : ` 'nonce-${nonce}'`}`,
    "img-src 'self' blob: data:",
    "font-src 'self' data:",
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    ...(isDevelopment ? [] : ["upgrade-insecure-requests"]),
  ].join("; ");
}

export function proxy(request: NextRequest): NextResponse {
  if (
    request.nextUrl.pathname === "/api" ||
    request.nextUrl.pathname.startsWith("/api/")
  ) {
    return rewriteApiRequest(request);
  }

  const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
  const contentSecurityPolicy = createContentSecurityPolicy(nonce);
  const requestHeaders = new Headers(request.headers);
  requestHeaders.set("x-nonce", nonce);
  requestHeaders.set("Content-Security-Policy", contentSecurityPolicy);

  const response = NextResponse.next({
    request: {
      headers: requestHeaders,
    },
  });
  response.headers.set("Content-Security-Policy", contentSecurityPolicy);
  response.headers.set("Referrer-Policy", "strict-origin-when-cross-origin");
  response.headers.set("X-Content-Type-Options", "nosniff");

  return response;
}

export const config = {
  matcher: [
    "/api/:path*",
    {
      source:
        "/((?!_next/static|_next/image|favicon.ico|icon.svg|apple-icon.png|manifest.webmanifest|robots.txt|sitemap.xml).*)",
      missing: [
        { type: "header", key: "next-router-prefetch" },
        { type: "header", key: "purpose", value: "prefetch" },
      ],
    },
  ],
};
