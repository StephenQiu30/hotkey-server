"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Archive,
  BellRing,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  Loader2,
  MoreHorizontal,
  RefreshCw,
  ShieldAlert,
} from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  getAlerts,
  postAlertsIdAcknowledge,
  postAlertsIdResolve,
  postAlertsIdSuppress,
} from "@/services/hotkey/hotkey-server/alerts";
import { formatRadarTime } from "@/lib/radarPresentation";

type AlertState = NonNullable<HotKeyAPI.getAlertsParams["state"]> | "all";
type AlertAction = "acknowledge" | "resolve" | "suppress";

const stateLabels: Record<string, string> = {
  open: "待处理",
  acknowledged: "已确认",
  resolved: "已解决",
  suppressed: "已抑制",
};

function SeverityBadge({ severity }: { severity?: string }) {
  if (severity === "critical") {
    return <Badge variant="destructive">严重</Badge>;
  }
  if (severity === "warning") {
    return (
      <Badge variant="outline" className="border-amber-300 text-amber-700">
        警告
      </Badge>
    );
  }
  return <Badge variant="secondary">提示</Badge>;
}

function SummaryCard({ label, value }: { label: string; value: number }) {
  return (
    <Card className="shadow-none">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-2xl font-semibold tabular-nums">{value}</p>
      </CardContent>
    </Card>
  );
}

export default function AlertsPage() {
  const [state, setState] = useState<AlertState>("open");
  const [threads, setThreads] = useState<HotKeyAPI.AlertThreadResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [actioning, setActioning] = useState<number>();
  const [pendingSuppression, setPendingSuppression] =
    useState<HotKeyAPI.AlertThreadResponse>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const result = await getAlerts({
        ...(state === "all" ? {} : { state }),
        limit: 50,
      });
      setThreads(result.data?.items ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "告警加载失败");
    } finally {
      setLoading(false);
    }
  }, [state]);

  useEffect(() => {
    void load();
  }, [load]);

  const counts = useMemo(
    () => ({
      critical: threads.filter((thread) => thread.severity === "critical")
        .length,
      warning: threads.filter((thread) => thread.severity === "warning").length,
    }),
    [threads],
  );

  const operate = async (
    thread: HotKeyAPI.AlertThreadResponse,
    action: AlertAction,
  ) => {
    if (thread.id == null || thread.version == null) return;
    setActioning(thread.id);
    const payload = {
      expected_version: thread.version,
      reason_code:
        action === "acknowledge"
          ? "user_acknowledged"
          : action === "resolve"
            ? "user_resolved"
            : "user_suppressed",
    };
    try {
      if (action === "acknowledge") {
        await postAlertsIdAcknowledge({ id: thread.id }, payload);
      } else if (action === "resolve") {
        await postAlertsIdResolve({ id: thread.id }, payload);
      } else {
        await postAlertsIdSuppress({ id: thread.id }, payload);
      }
      toast.success(
        action === "acknowledge"
          ? "告警已确认"
          : action === "resolve"
            ? "告警已解决"
            : "同类告警已抑制",
      );
      await load();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "告警操作失败");
    } finally {
      setActioning(undefined);
    }
  };

  return (
    <div className="app-page radar-page !max-w-[1360px]">
      <header className="flex flex-col gap-5 border-b pb-6 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium text-primary">
            <BellRing className="h-4 w-4" />
            低噪声告警
          </div>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            告警中心
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
            同一监控与事件的重复触发会聚合成线程，处理记录可持续追溯。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Select
            value={state}
            onValueChange={(value) => setState(value as AlertState)}
          >
            <SelectTrigger aria-label="告警状态" className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="open">待处理</SelectItem>
              <SelectItem value="acknowledged">已确认</SelectItem>
              <SelectItem value="resolved">已解决</SelectItem>
              <SelectItem value="suppressed">已抑制</SelectItem>
              <SelectItem value="all">全部</SelectItem>
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            size="icon"
            onClick={load}
            aria-label="刷新告警"
          >
            <RefreshCw />
          </Button>
        </div>
      </header>

      <section className="mt-6 grid gap-3 sm:grid-cols-3" aria-label="告警摘要">
        <SummaryCard label="当前列表" value={threads.length} />
        <SummaryCard label="严重" value={counts.critical} />
        <SummaryCard label="警告" value={counts.warning} />
      </section>

      {loading ? (
        <div
          className="flex h-80 items-center justify-center"
          aria-label="正在加载告警"
        >
          <Loader2 className="h-5 w-5 animate-spin text-primary" />
        </div>
      ) : error ? (
        <Alert variant="destructive" className="mt-6">
          <CircleAlert className="h-4 w-4" />
          <AlertTitle>告警加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : threads.length === 0 ? (
        <Card className="mt-6 border-dashed shadow-none">
          <CardContent className="flex h-80 flex-col items-center justify-center text-center">
            <CheckCircle2 className="h-7 w-7 text-emerald-600" />
            <h2 className="mt-4 text-base font-semibold">当前没有这类告警</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Radar 会在事件达到监控阈值时自动创建告警线程。
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card className="mt-6 overflow-hidden shadow-none">
          <Table aria-label="告警线程列表">
            <TableHeader>
              <TableRow>
                <TableHead className="min-w-[320px]">告警</TableHead>
                <TableHead>级别</TableHead>
                <TableHead>触发次数</TableHead>
                <TableHead>最近触发</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {threads.map((thread) => (
                <TableRow key={thread.id}>
                  <TableCell className="py-4 align-top">
                    <div className="flex items-start gap-3">
                      {thread.severity === "critical" ? (
                        <ShieldAlert className="mt-1 h-4 w-4 shrink-0 text-destructive" />
                      ) : (
                        <CircleAlert className="mt-1 h-4 w-4 shrink-0 text-muted-foreground" />
                      )}
                      <div className="min-w-0">
                        <p className="font-medium leading-6">
                          {thread.title || `告警 #${thread.id}`}
                        </p>
                        <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                          {thread.reason || "事件信号达到当前监控阈值。"}
                        </p>
                        <Link
                          href={`/dashboard/events?event=${thread.event_id ?? ""}`}
                          className="mt-2 inline-flex items-center gap-1 text-xs text-primary no-underline"
                        >
                          查看关联事件
                          <ChevronRight className="h-3.5 w-3.5" />
                        </Link>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <SeverityBadge severity={thread.severity} />
                  </TableCell>
                  <TableCell className="tabular-nums">
                    {thread.occurrence_count ?? 1} 次
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    <p>{formatRadarTime(thread.last_triggered_at)}</p>
                    <p className="mt-1">
                      {stateLabels[thread.state || ""] || thread.state}
                    </p>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center justify-end gap-2">
                      {thread.state === "open" ? (
                        <Button
                          size="sm"
                          onClick={() => operate(thread, "acknowledge")}
                          disabled={actioning === thread.id}
                        >
                          {actioning === thread.id ? (
                            <Loader2 className="animate-spin" />
                          ) : (
                            <Check />
                          )}
                          确认告警
                        </Button>
                      ) : null}
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label="打开告警操作"
                          >
                            <MoreHorizontal />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          {thread.state === "open" ||
                          thread.state === "acknowledged" ? (
                            <DropdownMenuItem
                              onSelect={() => operate(thread, "resolve")}
                              disabled={actioning === thread.id}
                            >
                              <CheckCircle2 />
                              解决告警
                            </DropdownMenuItem>
                          ) : null}
                          {thread.state !== "suppressed" ? (
                            <>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem
                                onSelect={() => setPendingSuppression(thread)}
                                disabled={actioning === thread.id}
                                className="text-destructive focus:text-destructive"
                              >
                                <Archive />
                                抑制同类告警
                              </DropdownMenuItem>
                            </>
                          ) : null}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      <AlertDialog
        open={Boolean(pendingSuppression)}
        onOpenChange={(open) => {
          if (!open) setPendingSuppression(undefined);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>抑制同类告警？</AlertDialogTitle>
            <AlertDialogDescription>
              后续符合相同规则的信号将不再创建待处理告警。此操作会记录原因，可在审计记录中追溯。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              aria-label="确认抑制"
              onClick={() => {
                if (pendingSuppression) {
                  void operate(pendingSuppression, "suppress");
                }
                setPendingSuppression(undefined);
              }}
            >
              确认抑制
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
