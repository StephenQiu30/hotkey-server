import type { Metadata } from "next";
import { connection } from "next/server";

import { ContentList } from "@/app/content/components/content-list";

export const metadata: Metadata = {
  title: "作品资料",
  robots: { index: false, follow: false },
};

export default async function ContentPage() {
  await connection();
  return <ContentList />;
}
