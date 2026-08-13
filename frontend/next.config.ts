import type { NextConfig } from "next";

const apiOrigin = process.env.HOTKEY_API_ORIGIN ?? "http://127.0.0.1:8866";

const nextConfig: NextConfig = {
  agentRules: false,
  allowedDevOrigins: ["127.0.0.1"],
  output: process.env.NEXT_OUTPUT === "standalone" ? "standalone" : undefined,
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
