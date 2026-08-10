import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "HotKey · AI 热点情报平台",
    short_name: "HotKey",
    description: "追踪热点事件、来源与可引用原文证据。",
    start_url: "/dashboard/notifications",
    scope: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#0f172a",
    lang: "zh-CN",
    categories: ["productivity", "news"],
    icons: [
      {
        src: "/icons/hotkey-192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "any",
      },
      {
        src: "/icons/hotkey-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "any",
      },
    ],
  };
}
