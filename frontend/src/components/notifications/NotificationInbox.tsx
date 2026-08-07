"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Bell, ExternalLink, Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { getNotifications } from "@/services/hotkey/hotkey-server/notifications";
import { useNotificationStore, type NotificationTransport } from "@/stores/notificationStore";
import { cn } from "@/lib/utils";

const transportPresentation: Record<NotificationTransport, { label: string; variant: "default" | "secondary" | "outline" }> = {
  idle: { label: "未连接", variant: "outline" },
  connecting: { label: "连接中", variant: "secondary" },
  live: { label: "实时", variant: "default" },
  polling: { label: "轮询中", variant: "secondary" },
};

const eventTypeLabels: Record<string, string> = {
  "event.updated": "事件更新",
  "alert.triggered": "热点告警",
  "report.published": "报告发布",
  "report.failed": "报告失败",
  "collection.succeeded": "采集完成",
  "collection.failed": "采集失败",
};

function resourceHref(notification: HotKeyAPI.NotificationResponse) {
  const resourceID = notification.resource_id;
  if (!resourceID) return "/dashboard/notifications";
  switch (notification.resource_type) {
    case "event":
      return `/dashboard/events?event_id=${resourceID}`;
    case "alert":
      return "/dashboard/alerts";
    case "report":
      return "/dashboard/favorites";
    case "collection_run":
      return "/dashboard/contents";
    default:
      return "/dashboard/notifications";
  }
}

function occurredAtLabel(value?: string) {
  if (!value) return "刚刚";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "刚刚";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function NotificationInbox() {
  const items = useNotificationStore((state) => state.items);
  const lastEventID = useNotificationStore((state) => state.lastEventID);
  const readThroughID = useNotificationStore((state) => state.readThroughID);
  const transport = useNotificationStore((state) => state.transport);
  const markAllRead = useNotificationStore((state) => state.markAllRead);
  const [refreshing, setRefreshing] = useState(false);
  const presentation = transportPresentation[transport];

  useEffect(() => {
    if (lastEventID > readThroughID) markAllRead();
  }, [lastEventID, markAllRead, readThroughID]);

  const refresh = async () => {
    setRefreshing(true);
    try {
      const afterID = useNotificationStore.getState().lastEventID;
      const result = await getNotifications({ after_id: afterID, limit: 100 });
      useNotificationStore.getState().ingest(result.data?.items ?? []);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "通知加载失败");
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <section aria-label="站内通知">
      <PageHeader
        eyebrow="Notifications"
        title="站内通知"
        description="实时接收事件变化、热点告警、报告状态和采集结果；断线后会从上次游标自动补齐。"
        action={
          <div className="flex gap-2">
            <Badge variant={presentation.variant} aria-live="polite" className="h-9 px-3">
              {presentation.label}
            </Badge>
            <Button variant="outline" size="sm" onClick={refresh} disabled={refreshing} className="h-9 gap-2">
              {refreshing ? <Loader2 className="animate-spin" /> : <RefreshCw />}
              刷新
            </Button>
          </div>
        }
      />

      {items.length === 0 ? (
        <Card className="mt-6">
          <Empty className="h-64">
            <EmptyHeader>
              <EmptyMedia variant="icon"><Bell /></EmptyMedia>
              <EmptyTitle>暂时没有站内通知</EmptyTitle>
              <EmptyDescription>事件、告警、报告或采集状态变化后会显示在这里。</EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : (
        <Card className="mt-6 gap-0 overflow-hidden py-0">
          <div className="divide-y divide-border">
            {items.map((notification) => {
              const unread = (notification.id ?? 0) > readThroughID;
              return (
                <article
                  key={notification.id}
                  className={cn("flex gap-4 px-4 py-4 sm:px-5", unread && "bg-muted/35")}
                >
                  <span className={cn("mt-2 h-2 w-2 shrink-0 rounded-full", unread ? "bg-primary" : "bg-border")} aria-hidden="true" />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline">{eventTypeLabels[notification.event_type ?? ""] ?? "系统通知"}</Badge>
                      {unread ? <span className="text-xs font-medium text-primary">未读</span> : null}
                      <time className="ml-auto text-xs text-muted-foreground" dateTime={notification.occurred_at}>
                        {occurredAtLabel(notification.occurred_at)}
                      </time>
                    </div>
                    <h2 className="mt-2 text-sm font-medium">{notification.payload?.title ?? "系统通知"}</h2>
                    {notification.payload?.summary ? (
                      <p className="mt-1 line-clamp-2 text-sm leading-6 text-muted-foreground">{notification.payload.summary}</p>
                    ) : null}
                  </div>
                  <Button asChild variant="ghost" size="icon" className="shrink-0" aria-label="查看通知关联内容">
                    <Link href={resourceHref(notification)}><ExternalLink /></Link>
                  </Button>
                </article>
              );
            })}
          </div>
        </Card>
      )}
    </section>
  );
}
