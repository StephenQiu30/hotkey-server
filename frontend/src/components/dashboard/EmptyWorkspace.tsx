import {
  Activity,
  ArrowUpRight,
  DatabaseZap,
  FileSearch,
  Radar,
  Workflow,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { MonitorStatus } from "@/lib/domainEnums";
import { monitorStatusLabel } from "@/lib/domainPresentation";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { PageShell } from "@/layouts/PageShell";

type EmptyWorkspaceProps = {
  monitors: HotKeyAPI.MonitorResponse[];
  overview?: HotKeyAPI.RuntimeOverview;
  collectionRuns?: HotKeyAPI.CollectionRunResponse[];
  collectedContents?: HotKeyAPI.ContentResponse[];
};

export function EmptyWorkspace({
  monitors,
  overview,
  collectionRuns = [],
  collectedContents = [],
}: EmptyWorkspaceProps) {
  const visibleMonitors = monitors.filter(
    (monitor) => monitor.status !== MonitorStatus.Archived
  );
  const draftCount = visibleMonitors.filter(
    (monitor) => monitor.status === MonitorStatus.Draft
  ).length;
  const publishedCount = visibleMonitors.filter(
    (monitor) =>
      monitor.status === MonitorStatus.Active ||
      monitor.status === MonitorStatus.Paused
  ).length;
  const runningJobs = overview?.running_jobs ?? 0;
  const collectionStarted = collectionRuns.length > 0;
  const contentsReady = collectedContents.length > 0;

  const progressMessage =
    draftCount > 0
      ? "草稿不会创建采集任务"
      : publishedCount === 0
      ? "创建并发布监控后，系统才会开始采集与聚合"
      : !collectionStarted
      ? "监控已发布，但尚未产生采集任务。请确认后台调度器正在运行。"
      : !contentsReady
      ? "采集任务已经产生，内容会在标准化完成后进入工作台。"
      : `最近一页已有 ${collectedContents.length} 条内容，正在等待相关性匹配与事件聚合。`;

  const metrics = [
    { label: "已创建监控", value: visibleMonitors.length, icon: Radar },
    {
      label: "当前页采集批次",
      value: collectionRuns.length,
      icon: DatabaseZap,
    },
    {
      label: "最近入库内容",
      value: collectedContents.length,
      icon: FileSearch,
    },
    { label: "执行中任务", value: runningJobs, icon: Workflow },
  ];

  return (
    <PageShell>
      <PageHeader
        eyebrow="Workspace"
        title="工作台运行概览"
        description="监控配置、采集批次、内容入库与事件聚合是连续阶段，这里展示当前真实进度。"
      />

      <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {metrics.map((metric) => {
          const Icon = metric.icon;
          return (
            <Card key={metric.label}>
              <CardContent className="p-4">
                <div className="flex items-center justify-between">
                  <p className="text-xs text-muted-foreground">
                    {metric.label}
                  </p>
                  <Icon className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="mono mt-3 text-2xl font-medium">{metric.value}</p>
              </CardContent>
            </Card>
          );
        })}
      </div>

      <Card className="mt-5 overflow-hidden">
        <CardHeader className="flex flex-col gap-3 space-y-0 bg-secondary/30 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1">
            <CardTitle className="text-sm" role="heading" aria-level={2}>
              监控准备状态
            </CardTitle>
            <CardDescription className="text-xs">
              {progressMessage}
            </CardDescription>
          </div>
          <Button asChild size="sm" className="self-start sm:self-auto">
            <Link
              href={
                draftCount > 0 ? "/dashboard/settings" : "/dashboard/contents"
              }
            >
              {draftCount > 0 ? "发布监控" : "查看采集内容"}
              <ArrowUpRight />
            </Link>
          </Button>
        </CardHeader>

        {visibleMonitors.length ? (
          <CardContent className="space-y-1 p-1.5">
            {visibleMonitors.slice(0, 6).map((monitor) => {
              return (
                <Link
                  key={monitor.id}
                  href="/dashboard/settings"
                  className="grid gap-3 rounded-lg px-5 py-4 text-foreground no-underline hover:bg-muted/60 sm:grid-cols-[minmax(0,1fr)_120px_110px] sm:items-center"
                >
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">
                      {monitor.name || `监控 #${monitor.id}`}
                    </span>
                    <span className="mono mt-1 block truncate text-xs text-muted-foreground">
                      {monitor.query || monitor.description || "暂无关键词规则"}
                    </span>
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {monitor.sources?.length ?? 0} 个来源
                  </span>
                  <Badge variant="outline" className="w-fit">
                    {monitorStatusLabel(monitor.status)}
                  </Badge>
                </Link>
              );
            })}
          </CardContent>
        ) : (
          <Empty className="min-h-52 rounded-none border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Activity />
              </EmptyMedia>
              <EmptyTitle className="text-sm">还没有配置监控</EmptyTitle>
              <EmptyDescription>
                创建监控并关联数据来源，建立第一条热点检测链路。
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button asChild size="sm">
                <Link href="/dashboard/settings">创建监控</Link>
              </Button>
            </EmptyContent>
          </Empty>
        )}
      </Card>

      <Card className="mt-5 overflow-hidden">
        <CardContent className="grid gap-2 p-2 sm:grid-cols-2 xl:grid-cols-4">
          {[
            {
              step: "01",
              title: "配置监控",
              detail: visibleMonitors.length ? "已完成" : "等待创建",
              active: visibleMonitors.length > 0,
            },
            {
              step: "02",
              title: "创建采集任务",
              detail: collectionStarted
                ? "已产生采集批次"
                : publishedCount
                ? "等待调度"
                : "等待发布草稿",
              active: collectionStarted,
            },
            {
              step: "03",
              title: "内容标准化",
              detail: contentsReady
                ? `${collectedContents.length} 条已入库`
                : "尚无内容",
              active: contentsReady,
            },
            {
              step: "04",
              title: "形成聚合事件",
              detail: "尚无事件",
              active: false,
            },
          ].map((stage) => (
            <div
              key={stage.step}
              className="rounded-lg bg-muted/30 p-5"
            >
              <div className="flex items-center gap-3">
                <span
                  className={`mono flex h-7 w-7 items-center justify-center rounded-full text-[11px] ${
                    stage.active
                      ? "bg-success/12 text-success"
                      : "bg-secondary text-muted-foreground"
                  }`}
                >
                  {stage.step}
                </span>
                <div>
                  <p className="text-sm font-medium">{stage.title}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {stage.detail}
                  </p>
                </div>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
    </PageShell>
  );
}
import Link from "next/link";
