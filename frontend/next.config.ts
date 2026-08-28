import type { NextConfig } from "next";

const apiOrigin = process.env.HOTKEY_API_ORIGIN ?? "http://127.0.0.1:8866";

// App Router emits inline bootstrap scripts for hydration. Keep those scripts
// compatible while refusing inline event-handler attributes from untrusted
// report fields and closing the remaining executable embedding surfaces.
const contentSecurityPolicy = [
  "default-src 'self'",
  "script-src 'self' 'unsafe-inline'",
  "script-src-attr 'none'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self'",
  "connect-src 'self' ws: wss:",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-src 'none'",
  "frame-ancestors 'none'",
  "manifest-src 'self'",
  "worker-src 'self'",
].join("; ");

const nextConfig: NextConfig = {
  agentRules: false,
  allowedDevOrigins: ["127.0.0.1"],
  output: process.env.NEXT_OUTPUT === "standalone" ? "standalone" : undefined,
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          {
            key: "Content-Security-Policy",
            value: contentSecurityPolicy,
          },
        ],
      },
    ];
  },
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${apiOrigin}/api/:path*`,
      },
      {
        source: "/healthz",
        destination: `${apiOrigin}/healthz`,
      },
      {
        source: "/feeds/:path*",
        destination: `${apiOrigin}/feeds/:path*`,
      },
    ];
  },
  async redirects() {
    return [
      {
        source: "/dashboard/monitors",
        destination: "/dashboard/settings",
        permanent: false,
      },
    ];
  },
};

export default nextConfig;
