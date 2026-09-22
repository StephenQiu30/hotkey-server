import type { Metadata } from "next";
import { connection } from "next/server";

import { SourceCapabilityMatrix } from "@/app/sources/components/source-capability-matrix";

export const metadata: Metadata = {
  title: "来源能力",
  robots: { index: false, follow: false },
};

export default async function SourcesPage() {
  await connection();

  return <SourceCapabilityMatrix />;
}
