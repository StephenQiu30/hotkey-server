"use client";

import { useEffect, useState } from "react";
import { Gauge, Loader2 } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { getOperationsUsage } from "@/services/hotkey/hotkey-server/operations";

export function ManualSearchQuota({ refreshKey = 0 }: { refreshKey?: number }) {
  const [usage, setUsage] = useState<HotKeyAPI.UsageItem>();
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void getOperationsUsage()
      .then((result) => {
        if (!cancelled) {
          setUsage(result.data?.items?.find((item) => item.dimension === "manual_searches"));
        }
      })
      .catch(() => {
        if (!cancelled) setUsage(undefined);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [refreshKey]);

  const used = Number(usage?.used ?? 0);
  const limit = Number(usage?.limit ?? 0);
  const percent = limit > 0 ? Math.min(100, (used / limit) * 100) : 0;

  return (
    <Card className="mt-6 gap-3 px-5 py-4" aria-label="手动搜索配额">
      <div className="flex flex-wrap items-center gap-3">
        <Gauge className="h-4 w-4 text-muted-foreground" />
        <p className="text-sm font-medium">今日手动搜索</p>
        {loading ? <Loader2 className="ml-auto h-4 w-4 animate-spin text-muted-foreground" /> : usage ? (
          <p className="ml-auto text-sm tabular-nums"><span className="font-medium">{usage.remaining ?? "0"}</span> / {usage.limit ?? "0"} 次可用</p>
        ) : <p className="ml-auto text-xs text-muted-foreground">额度暂不可用</p>}
      </div>
      {usage ? <>
        <Progress value={percent} aria-label={`已使用 ${usage.used ?? "0"} 次，共 ${usage.limit ?? "0"} 次`} />
        <p className="text-xs text-muted-foreground">{usage.reset_at ? `${new Date(usage.reset_at).toLocaleString("zh-CN")} 重置` : "按 UTC 自然日重置"}</p>
      </> : null}
    </Card>
  );
}
