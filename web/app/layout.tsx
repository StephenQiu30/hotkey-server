import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "HotKey Dashboard",
  description: "X 热点监控平台",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
