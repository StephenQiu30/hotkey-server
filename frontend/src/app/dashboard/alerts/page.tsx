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
  Mail,
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
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
  getAlertsId,
  postAlertsIdAcknowledge,
  postAlertsIdResolve,
  postAlertsIdSuppress,
} from "@/services/hotkey/hotkey-server/alerts";
import { formatRadarTime } from "@/lib/radarPresentation";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

type AlertState = NonNullable<HotKeyAPI.getAlertsParams["state"]> | "all";
type AlertAction = "acknowledge" | "resolve" | "suppress";

const stateLabels: Record<string, string> = {
  open: "待处理",
  acknowledged: "已确认",
  resolved: "已解决",
  suppressed: "已抑制",
};

const deliveryLabels: Record<string, string> = {
  queued: "等待投递",
  claimed: "投递中",
  retrying: "等待重试",
  succeeded: "已送达",
  failed: "投递失败",
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
        <CardTitle
          className="text-sm font-medium text-muted-foreground"
          role="heading"
          aria-level={2}
        >
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
  const role = useAuthStore((auth) => auth.user?.role);
  const canSuppress = role === UserRole.Editor || role === UserRole.Admin;
  const [state, setState] = useState<AlertState>("open");
  const [threads, setThreads] = useState<HotKeyAPI.AlertThreadResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [actioning, setActioning] = useState<number>();
  const [pendingSuppression, setPendingSuppression] =
    useState<HotKeyAPI.AlertThreadResponse>();
  const [detailOpen, setDetailOpen] = useState(false);
  const [detail, setDetail] = useState<HotKeyAPI.AlertDetailResponse>();
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string>();

  const openDetail = async (thread: HotKeyAPI.AlertThreadResponse) => {
    if (thread.id == null) return;
    setDetailOpen(true);
    setDetail(undefined);
    setDetailError(undefined);
    setDetailLoading(true);
    try {
      const result = await getAlertsId({ id: thread.id });
      setDetail(result.data);
    } catch (reason) {
      setDetailError(
        reason instanceof Error ? reason.message : "告警详情加载失败"
      );
    } finally {
      setDetailLoading(false);
    }
  };

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
    [threads]
  );

  const operate = async (
    thread: HotKeyAPI.AlertThreadResponse,
    action: AlertAction
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
          : "同类告警已抑制"
      );
      await load();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "告警操作失败");
    } finally {
      setActioning(undefined);
    }
  };

  return (
    <div className="app-page radar-page">
      <header className="flex flex-col gap-6 border-b pb-8 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium text-primary">
            <BellRing className="h-4 w-4" />
            低噪声告警
          </div>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em]">
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
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{error}</span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void load()}
              aria-label="重试告警"
            >
              重试
            </Button>
          </AlertDescription>
        </Alert>
      ) : threads.length === 0 ? (
        <Card className="mt-6 border-dashed">
          <Empty className="h-80 border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <CheckCircle2 className="text-emerald-600" />
              </EmptyMedia>
              <EmptyTitle className="text-base">当前没有这类告警</EmptyTitle>
              <EmptyDescription>
                Radar 会在事件达到监控阈值时自动创建告警线程。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : (
        <Card className="mt-6 overflow-hidden shadow-none">
          <Table aria-label="告警线程列表" scrollAreaLabel="告警线程列表">
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
                        <Button
                          variant="link"
                          className="h-auto justify-start p-0 text-left font-medium leading-6"
                          onClick={() => void openDetail(thread)}
                        >
                          {thread.title || `告警 #${thread.id}`}
                        </Button>
                        <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                          {thread.reason || "事件信号达到当前监控阈值。"}
                        </p>
                        <Link
                          href={`/dashboard/events?event=${
                            thread.event_id ?? ""
                          }`}
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
                          {canSuppress && thread.state !== "suppressed" ? (
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

      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{detail?.thread?.title ?? "告警详情"}</DialogTitle>
            <DialogDescription>
              {detail?.thread?.reason ?? "查看触发依据、历史和交付状态。"}
            </DialogDescription>
          </DialogHeader>
          {detailLoading ? (
            <div
              className="flex h-48 items-center justify-center"
              aria-label="正在加载告警详情"
            >
              <Loader2 className="animate-spin" />
            </div>
          ) : detailError ? (
            <Alert variant="destructive">
              <CircleAlert />
              <AlertTitle>详情加载失败</AlertTitle>
              <AlertDescription>{detailError}</AlertDescription>
            </Alert>
          ) : detail?.thread ? (
            <div className="space-y-6">
              <section aria-labelledby="alert-evidence-title">
                <div className="flex items-center justify-between gap-3">
                  <h2 id="alert-evidence-title" className="text-sm font-medium">
                    触发依据
                  </h2>
                  <SeverityBadge severity={detail.thread.severity} />
                </div>
                <dl className="mt-3 grid grid-cols-2 gap-3 rounded-md border p-4 text-sm sm:grid-cols-4">
                  <div>
                    <dt className="text-xs text-muted-foreground">
                      综合重要性
                    </dt>
                    <dd className="mt-1 font-medium tabular-nums">
                      ≥ {detail.thread.threshold ?? 0}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">最低热度</dt>
                    <dd className="mt-1 font-medium tabular-nums">
                      ≥ {detail.thread.min_heat ?? 0}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">最低动量</dt>
                    <dd className="mt-1 font-medium tabular-nums">
                      ≥ {detail.thread.min_momentum ?? 0}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">最低宽度</dt>
                    <dd className="mt-1 font-medium tabular-nums">
                      ≥ {detail.thread.min_breadth ?? 0}
                    </dd>
                  </div>
                </dl>
              </section>
              <section aria-labelledby="alert-occurrences-title">
                <h2
                  id="alert-occurrences-title"
                  className="text-sm font-medium"
                >
                  变化记录
                </h2>
                <div className="mt-3 space-y-2">
                  {(detail.occurrences ?? []).map((occurrence) => (
                    <div
                      key={occurrence.id}
                      className="rounded-md border p-3 text-sm"
                    >
                      <div className="flex items-center justify-between gap-3">
                        <SeverityBadge severity={occurrence.severity} />
                        <time className="text-xs text-muted-foreground">
                          {formatRadarTime(occurrence.triggered_at)}
                        </time>
                      </div>
                      <p className="mt-2 text-xs text-muted-foreground">
                        综合 {occurrence.final_score ?? 0} · 热度{" "}
                        {occurrence.heat_score ?? 0} · 动量{" "}
                        {occurrence.momentum_score ?? 0} · 宽度{" "}
                        {occurrence.breadth_score ?? 0}
                      </p>
                      {!!occurrence.reason_codes?.length && (
                        <p className="mt-1 text-xs">
                          原因：{occurrence.reason_codes.join("、")}
                        </p>
                      )}
                    </div>
                  ))}
                </div>
              </section>
              <section aria-labelledby="alert-delivery-title">
                <h2
                  id="alert-delivery-title"
                  className="flex items-center gap-2 text-sm font-medium"
                >
                  <Mail className="h-4 w-4" />
                  邮件交付
                </h2>
                <div className="mt-3 space-y-2">
                  {(detail.email_deliveries ?? []).length ? (
                    detail.email_deliveries?.map((delivery) => (
                      <div
                        key={delivery.id}
                        className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-3 text-sm"
                      >
                        <div>
                          <p className="font-medium">
                            {deliveryLabels[delivery.status ?? ""] ??
                              delivery.status}
                          </p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            尝试 {delivery.attempt_count ?? 0}/5 次
                            {delivery.last_error
                              ? ` · ${delivery.last_error}`
                              : ""}
                          </p>
                        </div>
                        <SeverityBadge severity={delivery.severity} />
                      </div>
                    ))
                  ) : (
                    <p className="rounded-md border border-dashed p-4 text-xs text-muted-foreground">
                      该告警未规划邮件，或仍处于冷却期。
                    </p>
                  )}
                </div>
              </section>
              <section aria-labelledby="alert-audit-title">
                <h2 id="alert-audit-title" className="text-sm font-medium">
                  状态审计
                </h2>
                <ol className="mt-3 space-y-2 text-xs text-muted-foreground">
                  {(detail.audits ?? []).length ? (
                    detail.audits?.map((audit) => (
                      <li key={audit.id} className="rounded-md border p-3">
                        {stateLabels[audit.from_state ?? ""] ??
                          audit.from_state}{" "}
                        → {stateLabels[audit.to_state ?? ""] ?? audit.to_state}{" "}
                        · {audit.reason_code} ·{" "}
                        {formatRadarTime(audit.created_at)}
                      </li>
                    ))
                  ) : (
                    <li className="rounded-md border border-dashed p-4">
                      暂无状态变更。
                    </li>
                  )}
                </ol>
              </section>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

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
