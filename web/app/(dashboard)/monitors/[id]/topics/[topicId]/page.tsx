import { TrendChart } from "@/components/trend-chart";

const MOCK_TREND = [
  { timestamp: "2026-06-10", heat: 80 },
  { timestamp: "2026-06-11", heat: 123 },
  { timestamp: "2026-06-12", heat: 150 },
];

export default function TopicDetailPage({
  params,
}: {
  params: { id: string; topicId: string };
}) {
  return (
    <main>
      <h1>主题详情 #{params.topicId}</h1>
      <TrendChart points={MOCK_TREND} />
    </main>
  );
}
