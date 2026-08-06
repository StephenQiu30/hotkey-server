"use client";

import { Activity } from "lucide-react";
import { Bar, BarChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";

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
    <Card className="overflow-hidden" aria-labelledby="event-heat-heading">
      <CardHeader className="flex-row items-center justify-between space-y-0 border-b px-5 py-4">
        <div className="space-y-1">
          <CardTitle
            id="event-heat-heading"
            className="text-sm"
            role="heading"
            aria-level={2}
          >
            事件热度对比
          </CardTitle>
          <CardDescription className="text-xs">当前事件集合 · Top 6</CardDescription>
        </div>
        <Activity className="h-4 w-4 text-muted-foreground" />
      </CardHeader>
      {hasHeat ? (
        <CardContent className="h-64 px-4 py-5" data-testid="event-heat-chart">
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
        </CardContent>
      ) : (
        <Empty className="min-h-52 rounded-none border-0">
          <EmptyHeader>
            <EmptyMedia variant="icon"><Activity /></EmptyMedia>
            <EmptyTitle className="text-sm">热度尚未计算</EmptyTitle>
            <EmptyDescription>有真实热度分数后才会展示对比图。</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
    </Card>
  );
}
