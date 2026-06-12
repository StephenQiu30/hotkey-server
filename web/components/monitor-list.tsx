export interface MonitorSummary {
  id: number;
  name: string;
  queryText: string;
}

export function MonitorList({ monitors }: { monitors: MonitorSummary[] }) {
  return (
    <div>
      {monitors.map((monitor) => (
        <article key={monitor.id}>
          <h2>{monitor.name}</h2>
          <p>{monitor.queryText}</p>
        </article>
      ))}
    </div>
  );
}
