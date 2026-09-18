import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "HotKey 热点事件监控工作台",
    short_name: "HotKey",
    description: "面向公开信息的热点发现、证据整理与研判工作台。",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#171717",
  };
}
