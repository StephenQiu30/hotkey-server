export interface TopicSummary {
  id: number;
  title: string;
  currentHeat: number;
  trendDirection: "up" | "down" | "flat";
}

export function TopicList({ topics }: { topics: TopicSummary[] }) {
  return (
    <ul>
      {topics.map((topic) => (
        <li key={topic.id}>
          <strong>{topic.title}</strong>
          <span>{topic.currentHeat}</span>
          <span>{topic.trendDirection}</span>
        </li>
      ))}
    </ul>
  );
}
