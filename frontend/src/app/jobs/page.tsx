import type { Metadata } from "next";
import { connection } from "next/server";

import { JobHistory } from "@/app/jobs/components/job-history";

export const metadata: Metadata = {
  title: "任务记录",
  robots: { index: false, follow: false },
};

export default async function JobHistoryPage() {
  await connection();
  return <JobHistory />;
}
