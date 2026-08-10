import { Activity, AlertCircle } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { formatRadarScore, trendLabel } from "@/lib/radarPresentation";

type EventHeatPanelProps = {
  heat?: HotKeyAPI.HeatResponse;
  loading: boolean;
  error: boolean;
};

const componentLabels: Array<[keyof HotKeyAPI.HeatComponentResponse, string]> =
  [
    ["independence", "独立来源"],
    ["content_velocity", "内容增速"],
    ["source_breadth", "来源广度"],
    ["engagement", "互动表现"],
    ["recency", "证据新鲜度"],
  ];

export function EventHeatPanel({ heat, loading, error }: EventHeatPanelProps) {
  return (
    <section className="border-t pt-5" aria-labelledby="event-heat-heading">
      <div className="flex items-center justify-between gap-3">
        <h3 id="event-heat-heading" className="text-sm font-semibold">
          热度与趋势
        </h3>
        {heat ? (
          <Badge variant="outline" className="font-normal">
            {heat.window_hours ?? 24} 小时 · {heat.heat_version || "heat-v1"}
          </Badge>
        ) : null}
      </div>

      {loading ? (
        <div className="mt-4 space-y-3" aria-label="热度加载中">
          <Skeleton className="h-10 w-28" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : error ? (
        <Alert variant="destructive" className="mt-4">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>热度暂时不可用</AlertTitle>
          <AlertDescription>事件列表仍可使用，请稍后刷新。</AlertDescription>
        </Alert>
      ) : !heat ? (
        <p className="mt-3 text-sm text-muted-foreground">尚无热度快照。</p>
      ) : (
        <div className="mt-4 space-y-4">
          <div className="flex items-end justify-between rounded-lg border bg-muted/30 p-4">
            <div>
              <p className="text-xs text-muted-foreground">综合热度</p>
              <p className="mt-1 text-3xl font-semibold tracking-tight">
                {formatRadarScore(heat.heat_score)}
              </p>
            </div>
            <div className="text-right">
              <p className="text-sm font-medium">
                {trendLabel(heat.trend_status)}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                趋势 {formatRadarScore(heat.trend_score)}
              </p>
            </div>
          </div>

          {heat.components ? (
            <div className="space-y-3">
              {componentLabels.map(([key, label]) => {
                const value = heat.components?.[key];
                return (
                  <div key={key}>
                    <div className="mb-1.5 flex items-center justify-between text-xs">
                      <span className="text-muted-foreground">{label}</span>
                      <span>
                        {value == null ? "不可用" : formatRadarScore(value)}
                      </span>
                    </div>
                    {value == null ? (
                      <>
                        <div
                          className="h-2 rounded-full bg-muted"
                          aria-hidden="true"
                        />
                        <span className="sr-only">{label}不可用</span>
                      </>
                    ) : (
                      <Progress
                        value={value}
                        aria-label={`${label} ${formatRadarScore(value)}`}
                      />
                    )}
                  </div>
                );
              })}
              {heat.components.engagement == null ? (
                <p className="flex gap-2 text-xs leading-5 text-muted-foreground">
                  <Activity className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  当前事实源未提供可用互动指标，系统已按版本规则重新分配权重，并未按
                  0 分处理。
                </p>
              ) : null}
            </div>
          ) : (
            <Alert>
              <Activity className="h-4 w-4" />
              <AlertTitle>组成尚未记录</AlertTitle>
              <AlertDescription>
                这是旧版热度快照，综合分仍可参考。
              </AlertDescription>
            </Alert>
          )}
        </div>
      )}
    </section>
  );
}
