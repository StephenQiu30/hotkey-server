"use client";

import { useEffect, useState } from "react";
import { BarChart2, Flame, Radio, TrendingUp } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api, AnalyticsSourceResponse, AnalyticsSentimentResponse, AnalyticsTrendResponse } from "@/lib/api";

type AnalyticsState = {
  trend: AnalyticsTrendResponse | null;
  sources: AnalyticsSourceResponse | null;
  sentiment: AnalyticsSentimentResponse | null;
  loading: boolean;
  error: string | null;
};

const DEFAULT_DAYS = 14;

export function AnalyticsClient() {
  const [state, setState] = useState<AnalyticsState>({
    trend: null,
    sources: null,
    sentiment: null,
    loading: true,
    error: null,
  });

  async function load() {
    const [trend, sources, sentiment] = await Promise.all([
      api<AnalyticsTrendResponse>(`/api/analytics/trend?days=${DEFAULT_DAYS}`),
      api<AnalyticsSourceResponse>(`/api/analytics/sources?days=${DEFAULT_DAYS}&limit=6`),
      api<AnalyticsSentimentResponse>(`/api/analytics/sentiment?days=${DEFAULT_DAYS}`),
    ]);
    setState({ trend, sources, sentiment, loading: false, error: null });
  }

  useEffect(() => {
    load().catch((err) => setState((prev) => ({ ...prev, error: err.message, loading: false })));
  }, []);

  if (state.loading) {
    return <Skeleton className="h-80 ios-shell-card" />;
  }

  const maxTrend = Math.max(1, ...(state.trend?.points.map((item) => item.total_count) || [1]));
  const maxSource = Math.max(1, ...(state.sources?.items.map((item) => item.hotspot_count) || [1]));
  const sentimentTotal = state.sentiment?.total || 1;

  return (
    <div className="grid gap-5">
      {state.error ? (
        <p className="ios-card-muted border-destructive/35 bg-destructive/10 border p-3 text-sm text-destructive" role="alert">
          {state.error}
        </p>
      ) : null}

      <section className="grid gap-4 md:grid-cols-3">
        <StatCard
          icon={TrendingUp}
          label="趋势天数"
          value={`${state.trend?.period_days || DEFAULT_DAYS} 天`}
          helper="按抓取时间聚合新增热点。"
        />
        <StatCard
          icon={Flame}
          label="活跃热点"
          value={String((state.trend?.points ?? []).reduce((sum, item) => sum + item.active_count, 0))}
          helper="高于阈值且未被判假。"
        />
        <StatCard
          icon={Radio}
          label="来源总数"
          value={String(state.sources?.items.length || 0)}
          helper="本周期发生过数据写入的来源。"
        />
      </section>

      <section className="grid gap-4 xl:grid-cols-[1.5fr_1fr]">
        <Card className="ios-shell-card">
          <CardHeader>
            <CardTitle>热点趋势（新增数量）</CardTitle>
            <CardDescription>每天新增热点总量、active 与 filtered 分离。</CardDescription>
          </CardHeader>
          <CardContent>
            {state.trend?.points.length === 0 ? <Empty message="无趋势数据，先执行检测后刷新。"/> : null}
            <div className="grid gap-2">
              {state.trend?.points.map((item) => (
                <div className="grid gap-2" key={item.date}>
                  <div className="flex items-center justify-between text-sm">
                    <span>{item.date}</span>
                    <span className="text-muted-foreground">{item.total_count}</span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-muted/70">
                    <div
                      className="h-full rounded-full bg-primary/80 transition-all"
                      style={{ width: `${(item.total_count / maxTrend) * 100}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        <Card className="ios-shell-card">
          <CardHeader>
            <CardTitle>来源排行（最近 {state.sources?.period_days || DEFAULT_DAYS} 天）</CardTitle>
            <CardDescription>按热点总量排序。</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            {state.sources?.items.length ? (
              <>
                {state.sources.items.map((item) => (
                  <div className="grid gap-2" key={item.source_id}>
                    <div className="flex items-center justify-between text-sm">
                      <span className="font-semibold">{item.source_name}</span>
                      <span className="text-muted-foreground">{item.hotspot_count}</span>
                    </div>
                    <div className="h-2 overflow-hidden rounded-full bg-muted/70">
                      <div
                        className="h-full rounded-full bg-emerald-400/85 transition-all"
                        style={{ width: `${(item.hotspot_count / maxSource) * 100}%` }}
                      />
                    </div>
                  </div>
                ))}
              </>
            ) : null}
            {state.sources?.items.length === 0 ? <Empty message="该周期暂无来源热点。"/> : null}
          </CardContent>
        </Card>
      </section>

      <section>
        <Card className="ios-shell-card">
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>热点重要性分布</CardTitle>
              <CardDescription>按 AI 重要性标签聚合。</CardDescription>
            </div>
            <BarChart2 className="h-4 w-4 text-primary" />
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-2">
            {state.sentiment?.by_importance.map((item) => (
                <div className="grid gap-2" key={item.importance}>
                  <div className="flex items-center justify-between text-sm">
                    <span className="font-semibold">{item.importance}</span>
                    <span className="text-muted-foreground">{item.count}</span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-muted/70">
                    <div
                      className="h-full rounded-full bg-amber-400 transition-all"
                      style={{ width: `${sentimentTotal === 0 ? 0 : (item.count / sentimentTotal) * 100}%` }}
                    />
                  </div>
                </div>
              ))}
            {state.sentiment?.by_importance.length === 0 ? <Empty message="暂无 AI 分析记录。"/> : null}
          </CardContent>
        </Card>
      </section>
    </div>
  );
}

function StatCard({ icon: Icon, label, value, helper }: { icon: typeof TrendingUp; label: string; value: string; helper: string }) {
  return (
    <Card className="ios-shell-card ios-reveal">
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardDescription>{label}</CardDescription>
          <CardTitle className="mt-2 text-3xl text-slate-950">{value}</CardTitle>
          <CardDescription className="mt-2">{helper}</CardDescription>
        </div>
        <Icon className="h-5 w-5 text-primary" />
      </CardHeader>
    </Card>
  );
}

function Empty({ message }: { message: string }) {
  return <p className="ios-card-muted border-dashed border border-sky-200/80 bg-sky-50/70 p-3 text-sm text-muted-foreground">{message}</p>;
}
