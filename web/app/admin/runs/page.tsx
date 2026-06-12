import { AdminRunTable } from "@/components/admin-run-table";

const MOCK_RUNS = [
  { id: 1, status: "success" as const, fetchedCount: 42 },
  { id: 2, status: "failed" as const, fetchedCount: 0 },
  { id: 3, status: "running" as const, fetchedCount: 15 },
];

export default function AdminRunsPage() {
  return (
    <main>
      <h1>任务运行</h1>
      <AdminRunTable runs={MOCK_RUNS} />
    </main>
  );
}
