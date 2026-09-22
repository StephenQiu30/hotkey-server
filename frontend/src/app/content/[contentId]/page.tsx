import type { Metadata } from "next";

import { ContentDetail } from "@/app/content/[contentId]/components/content-detail";

export const metadata: Metadata = {
  title: "作品详情",
  robots: { index: false, follow: false },
};

type ContentDetailPageProps = {
  params: Promise<{ contentId: string }>;
};

export default async function ContentDetailPage({
  params,
}: ContentDetailPageProps) {
  const { contentId } = await params;
  return <ContentDetail contentId={contentId} />;
}
