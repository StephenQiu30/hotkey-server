import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "知微见澜 Ripplesight",
    short_name: "知微见澜",
    description: "从一个关键词，看见正在发生的变化。",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#171717",
    icons: [
      {
        src: "/icon.png?v=2",
        sizes: "256x256",
        type: "image/png",
        purpose: "any",
      },
    ],
  };
}
