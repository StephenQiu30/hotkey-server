import type { Metadata } from "next";

import { TopicEditor } from "@/app/monitors/[topicId]/components/topic-editor";

export const metadata: Metadata = {
  title: "编辑监控主题",
  robots: { index: false, follow: false },
};

type MonitorTopicPageProps = {
  params: Promise<{ topicId: string }>;
};

export default async function MonitorTopicPage({
  params,
}: MonitorTopicPageProps) {
  const { topicId } = await params;
  return <TopicEditor topicId={topicId} />;
}
