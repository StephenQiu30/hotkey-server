import type { Metadata } from "next";
import { connection } from "next/server";

import { ReportList } from "@/app/reports/components/report-list";

export const metadata: Metadata = {
  title: "日报",
  robots: { index: false, follow: false },
};

export default async function ReportsPage() {
  await connection();
  return <ReportList />;
}
