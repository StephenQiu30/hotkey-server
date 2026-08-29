"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Activity, AlertCircle, Clock3, ExternalLink, Loader2, RefreshCw, TriangleAlert } from "lucide-react";
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
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Surface } from "@/components/ui/surface";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  getOperationsJobs,
  getOperationsOverview,
  postOperationsJobsIdCancel,
  postOperationsJobsIdRetry,
} from "@/services/hotkey/hotkey-server/operations";

const jobStateOptions = [
  { value: "all", label: "全部状态" },
  { value: "available", label: "等待" },
  { value: "running", label: "执行中" },
  { value: "discarded", label: "失败" },
  { value: "cancelled", label: "已取消" },
  { value: "completed", label: "已完成" },
];

const jobStateLabels: Record<string, string> = {
  available: "等待",
  running: "执行中",
  discarded: "失败",
  cancelled: "已取消",
  completed: "已完成",
};

const alertImpactUnits: Record<string, string> = {
  "ALERT-DELIVERY-UNKNOWN": "交付",
  "ALERT-SOURCE-AUTH": "来源",
  "ALERT-MINIO-WRITE": "证据异常",
  "ALERT-CODEX-FAILURE": "智能任务",
  "ALERT-VAULT-CONFLICT": "冲突",
  "ALERT-BACKUP-FAILED": "备份运行",
  "ALERT-SEARCH-BACKLOG": "检索任务",
};

function alertThreshold(alert: HotKeyAPI.RuntimeAlert) {
  const parts: string[] = [];
  if ((alert.threshold_count ?? 0) > 1) parts.push(`${alert.threshold_count} 次`);
  if ((alert.threshold_seconds ?? 0) > 0) parts.push(`${alert.threshold_seconds} 秒`);
  return parts.join(" · ");
}

type PendingAction = {
  action: "cancel" | "retry";
  job: HotKeyAPI.JobResponse;
};

export function RuntimeOperationsPanel() {
  const [overview, setOverview] = useState<HotKeyAPI.RuntimeOverview>();
  const [jobs, setJobs] = useState<HotKeyAPI.JobResponse[]>([]);
  const [stateFilter, setStateFilter] = useState("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<PendingAction>();
  const [mutating, setMutating] = useState(false);
  const [focusRefreshRequested, setFocusRefreshRequested] = useState(false);
  const refreshButtonRef = useRef<HTMLButtonElement>(null);
  const actionButtonRef = useRef<HTMLButtonElement | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [overviewResult, jobsResult] = await Promise.all([
        getOperationsOverview(),
        getOperationsJobs({
          limit: 20,
          ...(stateFilter === "all" ? {} : { state: stateFilter }),
        }),
      ]);
      setOverview(overviewResult.data);
      setJobs(jobsResult.data?.items ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "运行状态加载失败");
    } finally {
      setLoading(false);
    }
  }, [stateFilter]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!focusRefreshRequested || loading || pending) return;
    refreshButtonRef.current?.focus();
    setFocusRefreshRequested(false);
  }, [focusRefreshRequested, loading, pending]);

  const mutate = async () => {
    if (!pending?.job.id) return;
    setMutating(true);
    try {
      if (pending.action === "retry") {
        await postOperationsJobsIdRetry({ id: pending.job.id });
        toast.success(`任务 #${pending.job.id} 已重新排队`);
      } else {
        await postOperationsJobsIdCancel({ id: pending.job.id });
        toast.success(`任务 #${pending.job.id} 已取消`);
      }
      actionButtonRef.current = null;
      setPending(undefined);
      await load();
      setFocusRefreshRequested(true);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "任务操作失败");
    } finally {
      setMutating(false);
    }
  };

  const summary = [
    { label: "等待任务", value: overview?.available_jobs ?? 0 },
    { label: "执行中", value: overview?.running_jobs ?? 0 },
    { label: "失败任务", value: overview?.discarded_jobs ?? 0 },
    {
      label: "队列滞后",
      value: `${Math.round(overview?.queue_lag_seconds ?? 0)} 秒`,
    },
  ];

  return (
    <section className="mt-8" aria-labelledby="runtime-title">
      <Card className="overflow-hidden">
        <CardHeader className="flex flex-row flex-wrap items-center gap-3 border-b">
          <div>
            <CardTitle
              id="runtime-title"
              role="heading"
              aria-level={2}
              className="flex items-center gap-2"
            >
              <Activity className="h-4 w-4" />
              运行状态
            </CardTitle>
            <p className="mt-2 text-sm text-muted-foreground">
              通过安全任务元数据定位积压与失败，不展示任务参数或上游原始错误。
            </p>
          </div>
          <div className="ml-auto flex w-full gap-2 sm:w-auto">
            <Select value={stateFilter} onValueChange={setStateFilter}>
              <SelectTrigger aria-label="筛选任务状态" className="min-w-36 flex-1 sm:flex-none">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {jobStateOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              ref={refreshButtonRef}
              variant="outline"
              size="icon"
              aria-label="刷新运行状态"
              onClick={() => void load()}
              disabled={loading}
            >
              {loading ? <Loader2 className="animate-spin" /> : <RefreshCw />}
            </Button>
          </div>
        </CardHeader>

        {error ? (
          <CardContent className="pt-6">
            <Alert variant="destructive">
              <AlertCircle />
              <AlertTitle>无法加载运行状态</AlertTitle>
              <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
                <span>{error}</span>
                <Button size="sm" variant="outline" onClick={() => void load()}>
                  重试运行状态
                </Button>
              </AlertDescription>
            </Alert>
          </CardContent>
        ) : (
          <>
            <CardContent className="grid gap-3 border-b p-4 sm:grid-cols-2 xl:grid-cols-4">
              {loading
                ? Array.from({ length: 4 }, (_, index) => (
                    <Skeleton key={index} className="h-20" />
                  ))
                : summary.map((item) => (
                    <Surface key={item.label} className="p-4" variant="subtle">
                      <p className="text-xs text-muted-foreground">{item.label}</p>
                      <p className="mt-2 text-xl font-semibold tabular-nums">{item.value}</p>
                    </Surface>
                  ))}
            </CardContent>

            {!loading && (overview?.alerts?.length ?? 0) > 0 ? (
              <CardContent className="space-y-3 border-b bg-destructive/5 p-4">
                <div className="flex items-center gap-2">
                  <TriangleAlert className="h-4 w-4 text-destructive" aria-hidden="true" />
                  <h3 className="font-semibold">运行告警</h3>
                  {overview?.alert_policy_version ? (
                    <Badge variant="outline">策略 {overview.alert_policy_version}</Badge>
                  ) : null}
                </div>
                <div className="grid gap-3 lg:grid-cols-2">
                  {overview?.alerts?.map((alert) => (
                    <Surface asChild key={alert.alert_id} variant="danger">
                    <article className="bg-background p-4">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="destructive">{alert.severity?.toUpperCase() ?? "P1"}</Badge>
                        <code className="text-xs font-semibold">{alert.alert_id}</code>
                        <span className="text-xs text-muted-foreground">
                          影响 {alert.affected_count ?? 0} 个{alertImpactUnits[alert.alert_id ?? ""] ?? "任务"}
                        </span>
                      </div>
                      <p className="mt-3 font-mono text-xs text-muted-foreground">{alert.reason_code}</p>
                      {alert.owner || alertThreshold(alert) ? (
                        <p className="mt-2 text-xs text-muted-foreground">
                          {alert.owner ? `责任人 ${alert.owner}` : ""}
                          {alert.owner && alertThreshold(alert) ? " · " : ""}
                          {alertThreshold(alert) ? `阈值 ${alertThreshold(alert)}` : ""}
                        </p>
                      ) : null}
                      {alert.job_id ? (
                        <p className="mt-2 text-sm">
                          任务 #{alert.job_id}
                          {alert.event_id ? ` · 事件 #${alert.event_id}` : ""}
                        </p>
                      ) : null}
                      {alert.notification_id || alert.attempt_id ? (
                        <p className="mt-2 text-sm">
                          {alert.notification_id ? `通知 #${alert.notification_id}` : ""}
                          {alert.notification_id && alert.attempt_id ? " · " : ""}
                          {alert.attempt_id ? `尝试 #${alert.attempt_id}` : ""}
                        </p>
                      ) : null}
                      {alert.resource_type && alert.resource_id ? (
                        <p className="mt-2 font-mono text-xs text-muted-foreground">
                          {alert.resource_type} #{alert.resource_id}
                        </p>
                      ) : null}
                      {alert.trace_id ? (
                        <p className="mt-2 break-all font-mono text-xs" aria-label="Trace ID">
                          {alert.trace_id}
                        </p>
                      ) : null}
                      {alert.runbook_url ? (
                        <a
                          className="mt-3 inline-flex items-center gap-1 text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          href={alert.runbook_url}
                          target="_blank"
                          rel="noreferrer"
                        >
                          打开处置手册
                          <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                        </a>
                      ) : null}
                    </article>
                    </Surface>
                  ))}
                </div>
              </CardContent>
            ) : null}

            {loading ? (
              <div className="space-y-3 p-5" aria-label="正在加载运行任务">
                {Array.from({ length: 3 }, (_, index) => (
                  <Skeleton key={index} className="h-10" />
                ))}
              </div>
            ) : jobs.length === 0 ? (
              <Empty className="min-h-48 rounded-none border-0">
                <EmptyHeader>
                  <EmptyMedia variant="icon"><Clock3 /></EmptyMedia>
                  <EmptyTitle>暂无匹配任务</EmptyTitle>
                  <EmptyDescription>调整状态筛选，或等待后台调度创建任务。</EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <Table className="min-w-[820px]" scrollAreaLabel="运行任务表">
                <TableHeader>
                  <TableRow>
                    <TableHead>任务</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>资源</TableHead>
                    <TableHead>尝试</TableHead>
                    <TableHead>失败码</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.map((job) => (
                    <TableRow key={job.id}>
                      <TableCell>
                        <span className="block font-mono text-xs">{job.kind}</span>
                        <span className="mt-1 block text-xs text-muted-foreground">#{job.id}</span>
                      </TableCell>
                      <TableCell>
                        <Badge variant={job.state === "discarded" ? "destructive" : "outline"}>
                          {jobStateLabels[job.state ?? ""] ?? job.state ?? "未知"}
                        </Badge>
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {job.resource_id ? `#${job.resource_id}` : "—"}
                      </TableCell>
                      <TableCell>{job.attempt ?? 0} / {job.max_attempts ?? 0}</TableCell>
                      <TableCell className="font-mono text-xs">{job.failure_code || "—"}</TableCell>
                      <TableCell className="text-right">
                        {job.state === "available" ? (
                          <Button
                            size="sm"
                            variant="outline"
                            aria-label={`取消任务 ${job.id}`}
                            onClick={(event) => {
                              actionButtonRef.current = event.currentTarget;
                              setPending({ action: "cancel", job });
                            }}
                          >
                            取消
                          </Button>
                        ) : job.state === "discarded" || job.state === "cancelled" ? (
                          <Button
                            size="sm"
                            variant="outline"
                            aria-label={`重试任务 ${job.id}`}
                            onClick={(event) => {
                              actionButtonRef.current = event.currentTarget;
                              setPending({ action: "retry", job });
                            }}
                          >
                            重试
                          </Button>
                        ) : (
                          <span className="text-xs text-muted-foreground">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </>
        )}
      </Card>

      <AlertDialog open={Boolean(pending)} onOpenChange={(open) => !open && setPending(undefined)}>
        <AlertDialogContent
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            const target = actionButtonRef.current;
            if (target?.isConnected) target.focus();
            else refreshButtonRef.current?.focus();
          }}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>
              {pending?.action === "retry" ? "确认重试任务？" : "确认取消任务？"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              任务 #{pending?.job.id} · {pending?.job.kind}。此操作不会修改原始任务参数；
              {pending?.action === "retry" ? "只会重新排队当前安全任务。" : "只允许取消尚未执行的任务。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutating}>返回</AlertDialogCancel>
            <AlertDialogAction
              disabled={mutating}
              onClick={(event) => {
                event.preventDefault();
                void mutate();
              }}
            >
              {mutating ? <Loader2 className="animate-spin" /> : null}
              {pending?.action === "retry" ? "确认重试" : "确认取消"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
