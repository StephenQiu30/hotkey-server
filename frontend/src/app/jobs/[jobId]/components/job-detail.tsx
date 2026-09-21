"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import {
  ArrowLeftIcon,
  BanIcon,
  RefreshCwIcon,
  RotateCcwIcon,
} from "lucide-react";

import { cancelCollectionJob, getCollectionJob } from "@/api/caijirenwu";
import { PageState } from "@/components/system/page-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ApiRequestError } from "@/request";

type JobDetailProps = {
  jobId: string;
};

type DetailState =
  | { status: "loading" }
  | { status: "ready"; job: HotKeyAPI.JobStatusView }
  | { status: "not-found" }
  | { status: "error"; message: string; requestId?: string };

type ActionError = {
  message: string;
  requestId?: string;
};

const STATUS_LABELS: Record<HotKeyAPI.JobControlStatus, string> = {
  queued: "排队中",
  running: "执行中",
  cancelling: "取消中",
  succeeded: "已完成",
  partially_succeeded: "部分完成",
  failed: "失败",
  cancelled: "已取消",
};

const STAGE_LABELS: Record<HotKeyAPI.JobStage, string> = {
  request: "请求来源",
  parse: "解析响应",
  save: "保存结果",
  analysis: "分析内容",
};

function isInvalidSession(error: unknown): boolean {
  return error instanceof ApiRequestError && error.code === "invalid_session";
}

function toErrorState(
  error: unknown,
): Extract<DetailState, { status: "error" }> {
  if (error instanceof ApiRequestError) {
    return {
      status: "error",
      message: error.message,
      requestId: error.requestId,
    };
  }
  return { status: "error", message: "任务状态加载失败，请稍后重试。" };
}

function formatTime(value: string | null): string {
  if (value === null) {
    return "—";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground text-sm">{label}</dt>
      <dd className="mt-1 text-base font-medium">{value}</dd>
    </div>
  );
}

export function JobDetail({ jobId }: JobDetailProps) {
  const router = useRouter();
  const [state, setState] = useState<DetailState>({ status: "loading" });
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isCancelling, setIsCancelling] = useState(false);
  const [actionError, setActionError] = useState<ActionError | null>(null);

  useEffect(() => {
    let current = true;
    void getCollectionJob({ job_id: jobId })
      .then((job) => {
        if (current) {
          setState({ status: "ready", job });
        }
      })
      .catch((error: unknown) => {
        if (!current) {
          return;
        }
        if (isInvalidSession(error)) {
          router.replace("/login");
        } else if (
          error instanceof ApiRequestError &&
          error.code === "resource_not_found"
        ) {
          setState({ status: "not-found" });
        } else {
          setState(toErrorState(error));
        }
      });
    return () => {
      current = false;
    };
  }, [jobId, router]);

  async function refresh() {
    if (isRefreshing) {
      return;
    }
    setIsRefreshing(true);
    setActionError(null);
    try {
      const job = await getCollectionJob({ job_id: jobId });
      setState({ status: "ready", job });
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
      } else {
        setActionError(
          error instanceof ApiRequestError
            ? { message: error.message, requestId: error.requestId }
            : { message: "刷新失败，当前显示的是上次读取的状态。" },
        );
      }
    } finally {
      setIsRefreshing(false);
    }
  }

  async function cancel() {
    if (isCancelling) {
      return;
    }
    setIsCancelling(true);
    setActionError(null);
    try {
      const job = await cancelCollectionJob({ job_id: jobId });
      setState({ status: "ready", job });
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
      } else {
        setActionError(
          error instanceof ApiRequestError
            ? { message: error.message, requestId: error.requestId }
            : { message: "取消失败，请重试。" },
        );
      }
    } finally {
      setIsCancelling(false);
    }
  }

  if (state.status === "loading") {
    return (
      <PageState
        eyebrow="任务详情"
        title="正在读取任务"
        description="正在读取持久状态与已保存的结果范围。"
      />
    );
  }

  if (state.status === "not-found") {
    return (
      <PageState
        eyebrow="任务不可用"
        title="没有找到这个任务"
        description="任务不存在，或当前使用者无权查看。"
        action={
          <Button asChild variant="secondary">
            <Link href="/events">
              <ArrowLeftIcon data-icon="inline-start" />
              返回工作台
            </Link>
          </Button>
        }
      />
    );
  }

  if (state.status === "error") {
    const description = state.requestId
      ? `${state.message} 请求编号：${state.requestId}`
      : state.message;
    return (
      <PageState
        eyebrow="加载失败"
        title="暂时无法读取任务"
        description={description}
        action={
          <Button type="button" onClick={() => window.location.reload()}>
            <RotateCcwIcon data-icon="inline-start" />
            重新加载
          </Button>
        }
      />
    );
  }

  const { job } = state;
  const canCancel = job.status === "queued" || job.status === "running";
  const isCancellationPending = job.status === "cancelling";
  const savedDescription =
    job.progress.items_saved === 0
      ? "尚未保存结果；这不等于来源返回空结果。"
      : `已持久保存 ${job.progress.items_saved} 条结果。`;

  return (
    <div className="bg-background min-h-screen">
      <header className="mx-auto flex h-16 max-w-5xl items-center justify-between px-5 sm:px-8 xl:px-0">
        <Button asChild variant="ghost" size="navigation">
          <Link href="/events">
            <ArrowLeftIcon data-icon="inline-start" />
            返回工作台
          </Link>
        </Button>
        <span className="text-muted-foreground hidden font-mono text-xs sm:inline">
          {job.id}
        </span>
      </header>

      <main className="mx-auto max-w-5xl px-5 py-10 sm:px-8 sm:py-14 xl:px-0">
        <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <p className="text-muted-foreground font-mono text-xs tracking-wider uppercase">
              Collection job
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-3">
              <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
                采集任务
              </h1>
              <Badge
                variant={
                  job.status === "failed" || job.cancellation?.timed_out
                    ? "destructive"
                    : "secondary"
                }
              >
                {STATUS_LABELS[job.status]}
              </Badge>
            </div>
            <p className="text-muted-foreground mt-3 text-sm">
              配置 {job.observation.configuration_ref} · 版本{" "}
              {job.observation.configuration_version}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => void refresh()}
              disabled={isRefreshing}
            >
              <RefreshCwIcon data-icon="inline-start" />
              {isRefreshing ? "正在刷新" : "刷新状态"}
            </Button>
            {canCancel ? (
              <Button
                type="button"
                variant="destructive"
                onClick={() => void cancel()}
                disabled={isCancelling}
              >
                <BanIcon data-icon="inline-start" />
                {isCancelling ? "正在取消" : "取消任务"}
              </Button>
            ) : isCancellationPending ? (
              <Button type="button" variant="destructive" disabled>
                <BanIcon data-icon="inline-start" />
                等待在途请求
              </Button>
            ) : null}
          </div>
        </div>

        {job.cancellation ? (
          <section
            className={
              job.cancellation.timed_out
                ? "bg-destructive/10 mt-8 rounded-2xl p-5"
                : "bg-muted mt-8 rounded-2xl p-5"
            }
            aria-live="polite"
          >
            <h2 className="font-medium">
              {job.cancellation.timed_out ? "取消收尾已超时" : "已收到取消请求"}
            </h2>
            <p className="text-muted-foreground mt-2 text-sm leading-6">
              系统不会发起新的来源请求；已在途响应仍会按截止时间保存。
              {job.cancellation.deadline_at
                ? ` 截止时间：${formatTime(job.cancellation.deadline_at)}。`
                : " 当前任务无需等待在途请求。"}
            </p>
          </section>
        ) : null}

        {actionError ? (
          <p role="alert" className="text-destructive mt-5 text-sm">
            {actionError.message}
            {actionError.requestId
              ? ` 请求编号：${actionError.requestId}`
              : null}
          </p>
        ) : null}

        <section
          className="mt-10 grid gap-4 sm:grid-cols-3"
          aria-label="任务进度"
        >
          <div className="bg-muted rounded-2xl p-6">
            <p className="text-muted-foreground text-sm">当前阶段</p>
            <p className="mt-2 text-2xl font-semibold">
              {job.progress.stage
                ? STAGE_LABELS[job.progress.stage]
                : "尚未开始"}
            </p>
          </div>
          <div className="bg-muted rounded-2xl p-6">
            <p className="text-muted-foreground text-sm">已发请求</p>
            <p className="mt-2 text-2xl font-semibold">
              {job.progress.requests_sent}
            </p>
          </div>
          <div className="bg-muted rounded-2xl p-6">
            <p className="text-muted-foreground text-sm">已保存结果</p>
            <p className="mt-2 text-2xl font-semibold">
              {job.progress.items_saved}
            </p>
          </div>
        </section>

        <section className="mt-10">
          <h2 className="text-xl font-semibold">已写入结果</h2>
          <div className="bg-muted mt-4 rounded-2xl p-6 sm:p-8">
            <p className="text-base font-medium">{savedDescription}</p>
            <p className="text-muted-foreground mt-2 text-sm leading-6">
              最后进度时间：{formatTime(job.progress.updated_at)}
              。结果正文将在对应来源能力交付后按权限展示。
            </p>
          </div>
        </section>

        <section className="mt-10" aria-labelledby="job-facts-title">
          <h2 id="job-facts-title" className="text-xl font-semibold">
            执行信息
          </h2>
          <dl className="mt-5 grid gap-x-8 gap-y-6 sm:grid-cols-2">
            <DetailItem label="任务类型" value={job.kind} />
            <DetailItem
              label="来源能力"
              value={job.observation.source_capability ?? "未指定"}
            />
            <DetailItem label="创建时间" value={formatTime(job.created_at)} />
            <DetailItem label="开始时间" value={formatTime(job.started_at)} />
            <DetailItem label="完成时间" value={formatTime(job.completed_at)} />
            <DetailItem
              label="计划时间"
              value={formatTime(job.scheduled_for_at)}
            />
          </dl>
        </section>
      </main>
    </div>
  );
}
