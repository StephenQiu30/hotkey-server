export interface TrendPoint {
  timestamp: string;
  heat: number;
}

export function TrendChart({ points }: { points: TrendPoint[] }) {
  return (
    <div>
      <h3>趋势图</h3>
      <ul>
        {points.map((point, index) => (
          <li key={index}>
            {point.timestamp}: {point.heat}
          </li>
        ))}
      </ul>
    </div>
  );
}
