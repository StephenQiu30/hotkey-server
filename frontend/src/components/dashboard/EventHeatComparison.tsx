"use client";

import { Activity } from "lucide-react";
import { Bar, BarChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

type EventHeatComparisonProps = {
  events: HotKeyAPI.EventResponse[];
};

export function EventHeatComparison({ events }: EventHeatComparisonProps) {
  const data = [...events]
    .sort((a, b) => (b.heat_score ?? 0) - (a.heat_score ?? 0))
    .slice(0, 6)
    .map((event) => ({
      name: event.title_zh?.slice(0, 18) || event.title_en?.slice(0, 18) || `#${event.id}`,
      heat: event.heat_score ?? 0,
    }))
    .reverse();
  const hasHeat = data.some((item) => item.heat > 0);

  return (
    <section className="panel overflow-hidden" aria-labelledby="event-heat-heading">
      <div className="flex items-center justify-between border-b border-border px-5 py-4">
        <div>
          <h3 id="event-heat-heading" className="text-sm font-medium">
            事件热度对比
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">当前事件集合 · Top 6</p>
        </div>
        <Activity className="h-4 w-4 text-muted-foreground" />
      </div>
      {hasHeat ? (
        <div className="h-64 px-4 py-5" data-testid="event-heat-chart">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={data} layout="vertical" margin={{ left: 4, right: 18 }}>
              <XAxis type="number" hide />
              <YAxis
                dataKey="name"
                type="category"
                axisLine={false}
                tickLine={false}
                width={120}
                tick={{ fill: "#65758b", fontSize: 11 }}
              />
              <Tooltip
                contentStyle={{
                  background: "#ffffff",
                  border: "1px solid #dce6f2",
                  borderRadius: 10,
                  color: "#10213b",
                  fontSize: 11,
                  boxShadow: "0 12px 32px rgba(22, 52, 85, .12)",
                }}
              />
              <Bar dataKey="heat" fill="#1769e0" radius={[0, 5, 5, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>
      ) : (
        <div className="flex min-h-52 flex-col items-center justify-center px-6 text-center">
          <Activity className="mb-3 h-5 w-5 text-muted-foreground" />
          <p className="text-sm font-medium">热度尚未计算</p>
          <p className="mt-1 text-xs text-muted-foreground">有真实热度分数后才会展示对比图。</p>
        </div>
      )}
    </section>
  );
}
