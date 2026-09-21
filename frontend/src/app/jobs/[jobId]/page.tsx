import type { Metadata } from "next";

import { JobDetail } from "@/app/jobs/[jobId]/components/job-detail";

export const metadata: Metadata = {
  title: "任务详情",
  robots: {
    index: false,
    follow: false,
  },
};

type JobPageProps = {
  params: Promise<{ jobId: string }>;
};

export default async function JobPage({ params }: JobPageProps) {
  const { jobId } = await params;
  return <JobDetail jobId={jobId} />;
}
