import { MonitorList } from "@/components/monitor-list";

const MOCK_MONITORS = [
  { id: 1, name: "OpenAI", queryText: "openai agent" },
  { id: 2, name: "Anthropic", queryText: "claude" },
];

export default function MonitorsPage() {
  return (
    <main>
      <h1>监控任务</h1>
      <MonitorList monitors={MOCK_MONITORS} />
    </main>
  );
}
