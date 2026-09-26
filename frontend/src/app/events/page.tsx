import type { Metadata } from "next";
import { connection } from "next/server";

import { EventsWorkspace } from "@/app/events/components/events-workspace";

export const metadata: Metadata = {
  title: "工作台",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function EventsPage() {
  await connection();

  return <EventsWorkspace />;
}
