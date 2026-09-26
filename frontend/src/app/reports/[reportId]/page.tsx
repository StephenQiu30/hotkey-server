import type { Metadata } from "next";

import { ReportDetail } from "@/app/reports/[reportId]/components/report-detail";

export const metadata: Metadata = {
  title: "日报详情",
  robots: { index: false, follow: false },
};

export default async function ReportDetailPage({
  params,
}: {
  params: Promise<{ reportId: string }>;
}) {
  const { reportId } = await params;
  return <ReportDetail reportId={reportId} />;
}
