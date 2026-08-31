import {
  Archive,
  Loader2,
  Pause,
  Pencil,
  Play,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Surface } from "@/components/ui/surface";
import { MonitorStatus } from "@/lib/domainEnums";
import { monitorStatusLabel } from "@/lib/domainPresentation";
import {
  formatMonitorTime,
  monitorIntervalLabel,
  monitorQuery,
  monitorScanStatusLabels,
  type MonitorScanState,
} from "@/lib/monitorWorkflow";
import { sourceTypeLabel } from "@/lib/sourceLabels";

type MonitorLifecycleAction = "pause" | "resume" | "archive" | "restore";

type MonitorCardProps = {
  monitor: HotKeyAPI.MonitorResponse;
  scan?: MonitorScanState;
  busy: boolean;
  canManage: boolean;
  canAdmin: boolean;
  onCollect: (monitor: HotKeyAPI.MonitorResponse) => void;
  onEdit: (monitor: HotKeyAPI.MonitorResponse) => void;
  onLifecycle: (
    monitor: HotKeyAPI.MonitorResponse,
    action: MonitorLifecycleAction
  ) => void;
  onDelete: (monitor: HotKeyAPI.MonitorResponse) => void;
};

export function MonitorCard({
  monitor,
  scan,
  busy,
  canManage,
  canAdmin,
  onCollect,
  onEdit,
  onLifecycle,
  onDelete,
}: MonitorCardProps) {
  const query = monitorQuery(monitor);
  const latest = scan?.items[0];
  return (
    <Card>
      <CardHeader className="gap-4 p-5 pb-3 sm:p-6 sm:pb-3">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle className="text-xl">
                <h2>{monitor.name}</h2>
              </CardTitle>
              <Badge variant="outline">
                {monitorStatusLabel(monitor.status)}
              </Badge>
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              监控词：{query}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              每 {monitorIntervalLabel(monitor.collection_interval_seconds)}扫描 ·{" "}
              {monitor.sources?.length ?? 0} 个来源
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {canManage && monitor.status === MonitorStatus.Active ? (
              <Button
                aria-label={`立即扫描 ${monitor.name}`}
                disabled={busy}
                size="sm"
                onClick={() => onCollect(monitor)}
              >
                {busy ? <Loader2 className="animate-spin" /> : <Search />}
                立即扫描
              </Button>
            ) : null}
            {canManage && monitor.status !== MonitorStatus.Archived ? (
              <Button
                aria-label={`编辑 ${monitor.name}`}
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => onEdit(monitor)}
              >
                <Pencil />
                编辑
              </Button>
            ) : null}
            {canManage && monitor.status === MonitorStatus.Active ? (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => onLifecycle(monitor, "pause")}
              >
                <Pause />
                暂停
              </Button>
            ) : null}
            {canManage && monitor.status === MonitorStatus.Paused ? (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => onLifecycle(monitor, "resume")}
              >
                <Play />
                恢复
              </Button>
            ) : null}
            {canManage && monitor.status !== MonitorStatus.Archived ? (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => onLifecycle(monitor, "archive")}
              >
                <Archive />
                归档
              </Button>
            ) : null}
            {canManage && monitor.status === MonitorStatus.Archived ? (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onLifecycle(monitor, "restore")}
                >
                  <RotateCcw />
                  恢复
                </Button>
                {canAdmin ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => onDelete(monitor)}
                  >
                    <Trash2 />
                    删除
                  </Button>
                ) : null}
              </>
            ) : null}
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-5 pt-2 sm:p-6 sm:pt-2">
        <h3 className="mb-3 text-sm font-medium">最近扫描</h3>
        {scan?.queued ? (
          <Surface
            className="mb-3 flex items-center gap-2 px-3 py-2 text-sm"
            variant="subtle"
          >
            <Loader2 className="h-4 w-4 animate-spin" />
            已排队，等待来源返回
          </Surface>
        ) : null}
        {latest ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
              <div className="flex items-center gap-2">
                <Badge
                  variant={latest.status === "failed" ? "destructive" : "secondary"}
                >
                  {monitorScanStatusLabels[latest.status ?? ""] ?? latest.status}
                </Badge>
                <span className="text-muted-foreground">
                  接受 {latest.accepted_count ?? 0} / 候选 {latest.candidate_count ?? 0}
                </span>
              </div>
              <span className="text-xs text-muted-foreground">
                {formatMonitorTime(
                  latest.finished_at || latest.started_at || latest.scheduled_at
                )}
              </span>
            </div>
            <div className="grid gap-2 md:grid-cols-2">
              {(latest.sources ?? []).map((item) => (
                <Surface
                  key={`${item.run_id}-${item.source_connection_id}`}
                  className="p-3"
                  variant="ring"
                >
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-medium">
                      {item.source_name || sourceTypeLabel(item.source_type)}
                    </p>
                    <Badge
                      variant={item.status === "failed" ? "destructive" : "secondary"}
                    >
                      {monitorScanStatusLabels[item.status ?? ""] ?? item.status}
                    </Badge>
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {item.status === "succeeded"
                      ? `成功 · 接受 ${item.accepted_count ?? 0} / 候选 ${
                          item.candidate_count ?? 0
                        }`
                      : item.error_code || "等待来源返回"}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {formatMonitorTime(
                      item.finished_at || item.started_at || item.scheduled_at
                    )}
                  </p>
                </Surface>
              ))}
            </div>
          </div>
        ) : !scan?.queued ? (
          <p className="text-sm text-muted-foreground">尚无扫描记录。</p>
        ) : null}
      </CardContent>
    </Card>
  );
}
