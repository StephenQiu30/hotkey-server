import { PostFeed } from "@/components/post-feed";
import { TopicList } from "@/components/topic-list";
import Link from "next/link";

const MOCK_POSTS = [
  { id: 1, authorName: "user1", text: "OpenAI released new agent framework", score: 95 },
  { id: 2, authorName: "user2", text: "Claude gets tool use upgrade", score: 88 },
];

const MOCK_TOPICS = [
  { id: 1, title: "Agent launch", currentHeat: 123, trendDirection: "up" as const },
  { id: 2, title: "API pricing", currentHeat: 45, trendDirection: "down" as const },
];

export default function MonitorDetailPage({
  params,
}: {
  params: { id: string };
}) {
  return (
    <main>
      <h1>监控详情 #{params.id}</h1>
      <section>
        <h2>热点内容</h2>
        <PostFeed posts={MOCK_POSTS} />
      </section>
      <section>
        <h2>关联主题</h2>
        <TopicList topics={MOCK_TOPICS} />
        <Link href={`/monitors/${params.id}/topics/1`}>查看主题详情</Link>
      </section>
    </main>
  );
}
