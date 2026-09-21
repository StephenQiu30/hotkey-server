import type { Metadata } from "next";

import { TopicForm } from "@/app/monitors/new/components/topic-form";

export const metadata: Metadata = {
  title: "新建监控主题",
  robots: { index: false, follow: false },
};

export default function NewMonitorTopicPage() {
  return <TopicForm />;
}
