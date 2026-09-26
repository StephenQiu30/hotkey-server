import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import type { ReactNode } from "react";

import "./globals.css";

const geistSans = Geist({
  subsets: ["latin"],
  variable: "--font-geist-sans",
  display: "swap",
});

const geistMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-geist-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: {
    default: "知微见澜 Ripplesight · 从一个关键词，看见正在发生的变化",
    template: "%s · 知微见澜 Ripplesight",
  },
  description:
    "设定你关心的品牌、产品或话题，持续汇集相关讨论，沿着来源和时间看清变化如何发生。",
  applicationName: "知微见澜 Ripplesight",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html
      lang="zh-CN"
      data-scroll-behavior="smooth"
      className={`${geistSans.variable} ${geistMono.variable}`}
    >
      <body>{children}</body>
    </html>
  );
}
