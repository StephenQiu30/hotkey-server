import type { Metadata } from "next";
import { Plus_Jakarta_Sans } from "next/font/google";
import "./styles.css";

const sans = Plus_Jakarta_Sans({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

export const metadata: Metadata = {
  title: "AI Hotspot Radar",
  description: "Private-deployable SaaS platform for AI hotspot monitoring.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className={`${sans.variable} font-sans antialiased`} style={{ fontFamily: `-apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", ${sans.style.fontFamily}` }}>
        {children}
      </body>
    </html>
  );
}
