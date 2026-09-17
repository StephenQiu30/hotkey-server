import type { Metadata, Viewport } from "next";
import { GeistMono } from "geist/font/mono";
import { GeistSans } from "geist/font/sans";
import "../style.css";

export const metadata: Metadata = {
  title: {
    default: "HotKey 热点事件观察",
    template: "%s | HotKey",
  },
  description:
    "HotKey 用于管理热点监控、查看采集进度、整理事件资料与检索知识。当前工作台仅供登录用户使用。",
  robots: {
    index: false,
    follow: false,
  },
  icons: {
    icon: [{ url: "/logo.svg", type: "image/svg+xml" }],
  },
};

export const viewport: Viewport = {
  themeColor: "#ffffff",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN">
      <body className={`${GeistSans.variable} ${GeistMono.variable}`}>
        {children}
      </body>
    </html>
  );
}
