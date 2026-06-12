export interface AdminRunSummary {
  id: number;
  status: "success" | "failed" | "running";
  fetchedCount: number;
}

export function AdminRunTable({ runs }: { runs: AdminRunSummary[] }) {
  return (
    <table>
      <thead>
        <tr>
          <th>ID</th>
          <th>状态</th>
          <th>采集数量</th>
        </tr>
      </thead>
      <tbody>
        {runs.map((run) => (
          <tr key={run.id}>
            <td>{run.id}</td>
            <td>{run.status}</td>
            <td>{run.fetchedCount}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
