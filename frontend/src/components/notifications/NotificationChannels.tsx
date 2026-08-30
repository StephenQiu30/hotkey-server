"use client";

import Link from "next/link";
import { ArrowUpRight, Mail, Radio } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useAuthStore } from "@/stores/authStore";
import {
  useNotificationStore,
  type NotificationTransport,
} from "@/stores/notificationStore";

const realtimeStatus: Record<NotificationTransport, { label: string; detail: string }> = {
  idle: { label: "等待登录", detail: "登录后自动建立实时连接" },
  connecting: { label: "正在连接", detail: "正在恢复 WebSocket 会话" },
  live: { label: "WebSocket 已连接", detail: "新信号会即时送达当前工作台" },
  polling: { label: "REST 补拉中", detail: "连接恢复前按游标补齐，不会跳过消息" },
};

export function NotificationChannels() {
  const transport = useNotificationStore((state) => state.transport);
  const email = useAuthStore((state) => state.user?.email);
  const realtime = realtimeStatus[transport];

  return (
    <section aria-label="通知方式" className="mt-8">
      <div className="mb-3 flex items-end justify-between gap-4">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
            Delivery channels
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            同一条通知事实分别驱动站内实时消息和高优先级邮件。
          </p>
        </div>
        <Badge variant="outline">双通道</Badge>
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <Card className="relative overflow-hidden p-5">
          <span aria-hidden className="absolute inset-y-0 left-0 w-1 bg-foreground" />
          <div className="flex items-start justify-between gap-4">
            <div className="flex min-w-0 gap-3">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-foreground text-background">
                <Radio className="size-4" />
              </span>
              <div>
                <h2 className="font-semibold">站内实时通知</h2>
                <p className="mt-1 text-sm text-muted-foreground">{realtime.detail}</p>
              </div>
            </div>
            <Badge variant={transport === "live" ? "outline" : "secondary"} className="shrink-0">
              {realtime.label}
            </Badge>
          </div>
          <p className="mt-5 pt-2 text-xs leading-5 text-muted-foreground">
            首帧完成身份认证；断线重连时从最后一条消息 ID 继续补齐。
          </p>
        </Card>

        <Card className="relative overflow-hidden p-5">
          <span aria-hidden className="absolute inset-y-0 left-0 w-1 bg-muted-foreground/45" />
          <div className="flex items-start justify-between gap-4">
            <div className="flex min-w-0 gap-3">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-foreground">
                <Mail className="size-4" />
              </span>
              <div className="min-w-0">
                <h2 className="font-semibold">邮件通知</h2>
                <p className="mt-1 truncate text-sm text-muted-foreground">
                  {email || "当前账户邮箱"}
                </p>
              </div>
            </div>
            <Badge variant="secondary" className="shrink-0">高 / 紧急</Badge>
          </div>
          <div className="mt-5 flex items-center justify-between gap-3 pt-2">
            <p className="text-xs leading-5 text-muted-foreground">
              在监控设置中开启后发送，失败会由后台任务有限重试。
            </p>
            <Button asChild variant="ghost" size="sm" className="shrink-0 gap-1">
              <Link href="/dashboard/settings" aria-label="管理邮件提醒">
                管理 <ArrowUpRight className="size-3.5" />
              </Link>
            </Button>
          </div>
        </Card>
      </div>
    </section>
  );
}
