"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Activity, Flame, Loader2, RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { getMicroEvents } from "@/services/hotkey/hotkey-server/microEvents";

const statusLabels: Record<string, string> = {
  active: "活跃",
  monitoring: "持续观察",
  closed: "已结束",
  merged: "已合并",
  split: "已拆分",
  review: "待复核",
};

const evidenceLabels: Record<string, string> = {
  no_citable_body: "无可引用正文",
  single_origin: "单一出处",
  multiple_independent_origins: "多个独立出处",
  disputed: "存在争议",
  publisher_corrected: "发布者已更正",
  publisher_withdrawn: "发布者已撤回",
};

function formatDateTime(value?: string) {
  if (!value) return "时间未知";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "时间未知";
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(parsed);
}

function eventTitle(event: HotKeyAPI.MicroEventResponseDTO) {
  const subject = event.primary_subject_key?.trim();
  const action = event.primary_action_key?.trim();
  if (subject && action) return `${subject} · ${action}`;
  return subject || action || `语义事件 #${event.id ?? "—"}`;
}

function MicroEventCard({ event }: { event: HotKeyAPI.MicroEventResponseDTO }) {
  const heat = event.latest_heat?.heat_score;
  const evidenceState = event.evidence_state?.state;

  return (
    <Card aria-label={eventTitle(event)}>
      <CardHeader className="gap-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="break-words text-lg">{eventTitle(event)}</CardTitle>
            <p className="mono mt-2 break-all text-xs text-muted-foreground">
              {event.event_key || `micro-event:${event.id ?? "unknown"}`} · v{event.version ?? "—"}
            </p>
          </div>
          <Badge variant={event.status === "active" ? "success" : "outline"}>
            {statusLabels[event.status || ""] || event.status || "状态未知"}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <p className="text-xs text-muted-foreground">事件开始</p>
          <p className="mt-1 font-medium">{formatDateTime(event.event_started_at)}</p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">聚合规模</p>
          <p className="mt-1 font-medium">
            {event.content_family_count ?? 0} 个内容家族 · {event.document_count ?? 0} 篇文档
          </p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">Heat v2</p>
          <p className="mt-1 inline-flex items-center gap-1 font-medium">
            <Flame className="size-4 text-warning" />
            {heat === undefined ? "预热中" : heat.toFixed(1)}
          </p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">证据状态</p>
          <p className="mt-1 font-medium">
            {evidenceLabels[evidenceState || ""] || evidenceState || "尚未计算"}
          </p>
        </div>
        {event.latest_heat?.reason_codes?.length ? (
          <p className="mono break-words text-xs text-muted-foreground sm:col-span-2 lg:col-span-4">
            Heat 理由：{event.latest_heat.reason_codes.join(" · ")}
          </p>
        ) : null}
        {event.id ? (
          <div className="sm:col-span-2 lg:col-span-4">
            <Button asChild size="sm" variant="outline">
              <Link href={`/dashboard/events/${event.id}/governance`}>查看治理与证据</Link>
            </Button>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

export default function EventsPage() {
  const [items, setItems] = useState<HotKeyAPI.MicroEventResponseDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const response = await getMicroEvents({ limit: 50, sort: "heat" });
      setItems(response.data?.items ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "语义事件暂时无法读取");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <main className="mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
      <PageHeader
        action={
          <Button disabled={loading} onClick={() => void load()} variant="outline">
            {loading ? <Loader2 className="animate-spin" /> : <RefreshCw />}
            刷新事件
          </Button>
        }
        description="查看已接受匹配按内容家族去重后形成的语义事件，以及对应 Heat 与证据状态。"
        eyebrow="Events"
        title="语义事件"
      />

      {error ? (
        <Alert variant="destructive">
          <AlertTitle>事件加载失败</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{error}</span>
            <Button onClick={() => void load()} size="sm" variant="outline">重试</Button>
          </AlertDescription>
        </Alert>
      ) : null}

      {loading && items.length === 0 ? (
        <div aria-live="polite" className="flex min-h-72 items-center justify-center" role="status">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
          <span className="sr-only">正在加载语义事件</span>
        </div>
      ) : null}

      {!loading && !error && items.length === 0 ? (
        <Card className="border-dashed">
          <Empty className="min-h-72 border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon"><Activity /></EmptyMedia>
              <EmptyTitle>暂时没有语义事件</EmptyTitle>
              <EmptyDescription>已接受的文档匹配完成内容家族与事件投影后会显示在这里。</EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : null}

      {items.length ? (
        <section aria-live="polite" className="space-y-4">
          {items.map((event, index) => (
            <MicroEventCard event={event} key={event.id ?? event.event_key ?? index} />
          ))}
        </section>
      ) : null}
    </main>
  );
}
