"use client";

import { useState } from "react";
import { BellOff, Loader2, Save, Smartphone } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  deleteNotificationsPushSubscriptionsId,
  putNotificationsPushSubscriptionsId,
} from "@/services/hotkey/hotkey-server/notifications";
import { cn } from "@/lib/utils";

type MonitorOption = Pick<HotKeyAPI.MonitorResponse, "id" | "name" | "status">;

type PushSubscriptionDeviceCardProps = {
  subscription: HotKeyAPI.PushSubscriptionResponseDTO;
  monitors: MonitorOption[];
  currentBrowser: boolean;
  onUpdated(subscription: HotKeyAPI.PushSubscriptionResponseDTO): void;
  onDisabled(subscription: HotKeyAPI.PushSubscriptionResponseDTO): Promise<void>;
};

const pushStatusLabels: Record<string, string> = {
  active: "已启用",
  disabled: "已停用",
  expired: "已失效",
};

function activityLabel(value?: string) {
  if (!value) return "尚无投递记录";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "尚无投递记录";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function PushSubscriptionDeviceCard({
  subscription,
  monitors,
  currentBrowser,
  onUpdated,
  onDisabled,
}: PushSubscriptionDeviceCardProps) {
  const [deviceLabel, setDeviceLabel] = useState(subscription.device_label ?? "");
  const [timezone, setTimezone] = useState(subscription.timezone ?? "UTC");
  const [ttlSeconds, setTTLSeconds] = useState(
    String(subscription.ttl_seconds ?? 3600),
  );
  const [quietEnabled, setQuietEnabled] = useState(
    Boolean(subscription.quiet_start && subscription.quiet_end),
  );
  const [quietStart, setQuietStart] = useState(subscription.quiet_start ?? "22:00");
  const [quietEnd, setQuietEnd] = useState(subscription.quiet_end ?? "08:00");
  const [monitorIDs, setMonitorIDs] = useState<number[]>(
    () => subscription.monitor_ids?.filter((id): id is number => Boolean(id)) ?? [],
  );
  const [saving, setSaving] = useState(false);
  const active = subscription.status === "active";
  const subscriptionID = subscription.id ?? 0;
  const version = subscription.version ?? 0;

  const toggleMonitor = (monitorID: number, checked: boolean) => {
    setMonitorIDs((current) =>
      checked
        ? [...new Set([...current, monitorID])].sort((left, right) => left - right)
        : current.filter((id) => id !== monitorID),
    );
  };

  const save = async () => {
    if (!active || subscriptionID <= 0 || version <= 0) return;
    if (monitorIDs.length === 0) {
      toast.error("请至少选择一个需要推送的监控。");
      return;
    }
    if (quietEnabled && quietStart === quietEnd) {
      toast.error("免打扰开始与结束时间不能相同。");
      return;
    }
    setSaving(true);
    try {
      const response = await putNotificationsPushSubscriptionsId(
        { id: subscriptionID },
        {
          device_label: deviceLabel.trim(),
          timezone: timezone.trim(),
          ttl_seconds: Number(ttlSeconds),
          quiet_start: quietEnabled ? quietStart : undefined,
          quiet_end: quietEnabled ? quietEnd : undefined,
          monitor_ids: monitorIDs,
        },
        { headers: { "If-Match": `"v${version}"` } },
      );
      if (!response.data?.id || !response.data.version) {
        throw new Error("设备设置响应不完整");
      }
      onUpdated(response.data);
      toast.success("设备通知设置已保存");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "设备设置保存失败");
    } finally {
      setSaving(false);
    }
  };

  const disable = async () => {
    if (!active || subscriptionID <= 0 || version <= 0) return;
    if (!window.confirm(`确定停用“${subscription.device_label ?? "此设备"}”的通知吗？`)) {
      return;
    }
    setSaving(true);
    try {
      const response = await deleteNotificationsPushSubscriptionsId(
        { id: subscriptionID },
        { headers: { "If-Match": `"v${version}"` } },
      );
      if (!response.data?.id || !response.data.version) {
        throw new Error("停用设备响应不完整");
      }
      await onDisabled(response.data);
      toast.success("设备通知已停用");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "设备停用失败");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card
      className={cn(
        "border p-4 sm:p-5",
        currentBrowser && "border-primary/45 bg-primary/[0.025]",
      )}
    >
      <div className="flex flex-wrap items-start gap-3">
        <span className="rounded-full bg-muted p-2" aria-hidden="true">
          <Smartphone className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="font-medium">{subscription.device_label ?? "未命名设备"}</h3>
            <Badge variant={active ? "default" : "outline"}>
              {pushStatusLabels[subscription.status ?? ""] ?? "状态未知"}
            </Badge>
            {currentBrowser ? <Badge variant="secondary">当前浏览器</Badge> : null}
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            最近成功：{activityLabel(subscription.last_success_at)}
            {subscription.last_failure_at
              ? ` · 最近失败：${activityLabel(subscription.last_failure_at)}`
              : ""}
          </p>
          {subscription.expiration_reason ? (
            <p className="mt-1 text-xs text-destructive">
              失效原因：{subscription.expiration_reason}
            </p>
          ) : null}
        </div>
      </div>

      {active ? (
        <div className="mt-5 space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor={`push-device-label-${subscriptionID}`}>设备名称</Label>
              <Input
                id={`push-device-label-${subscriptionID}`}
                value={deviceLabel}
                maxLength={80}
                onChange={(event) => setDeviceLabel(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`push-device-timezone-${subscriptionID}`}>时区</Label>
              <Input
                id={`push-device-timezone-${subscriptionID}`}
                value={timezone}
                maxLength={64}
                onChange={(event) => setTimezone(event.target.value)}
              />
            </div>
          </div>

          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">接收哪些监控</legend>
            <div className="grid gap-2 sm:grid-cols-2">
              {monitors.map((monitor) => (
                <Label
                  key={monitor.id}
                  className="flex min-h-10 items-center gap-3 rounded-lg border px-3 py-2 font-normal"
                >
                  <Checkbox
                    checked={monitorIDs.includes(monitor.id ?? 0)}
                    onCheckedChange={(checked) =>
                      toggleMonitor(monitor.id ?? 0, checked === true)
                    }
                  />
                  <span className="min-w-0 truncate">{monitor.name ?? `监控 #${monitor.id}`}</span>
                </Label>
              ))}
            </div>
          </fieldset>

          <div className="grid gap-4 sm:grid-cols-[1fr_1fr_1fr]">
            <div className="space-y-2">
              <Label htmlFor={`push-ttl-${subscriptionID}`}>通知有效期</Label>
              <Select value={ttlSeconds} onValueChange={setTTLSeconds}>
                <SelectTrigger id={`push-ttl-${subscriptionID}`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="3600">1 小时</SelectItem>
                  <SelectItem value="21600">6 小时</SelectItem>
                  <SelectItem value="86400">24 小时</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor={`push-quiet-start-${subscriptionID}`}>免打扰开始</Label>
              <Input
                id={`push-quiet-start-${subscriptionID}`}
                type="time"
                value={quietStart}
                disabled={!quietEnabled}
                onChange={(event) => setQuietStart(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`push-quiet-end-${subscriptionID}`}>免打扰结束</Label>
              <Input
                id={`push-quiet-end-${subscriptionID}`}
                type="time"
                value={quietEnd}
                disabled={!quietEnabled}
                onChange={(event) => setQuietEnd(event.target.value)}
              />
            </div>
          </div>
          <Label className="flex items-center gap-3 font-normal">
            <Checkbox
              checked={quietEnabled}
              onCheckedChange={(checked) => setQuietEnabled(checked === true)}
            />
            启用每日免打扰时段
          </Label>

          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button variant="outline" onClick={disable} disabled={saving}>
              <BellOff />
              停用此设备
            </Button>
            <Button
              onClick={save}
              disabled={saving || !deviceLabel.trim() || monitorIDs.length === 0}
            >
              {saving ? <Loader2 className="animate-spin" /> : <Save />}
              保存设备设置
            </Button>
          </div>
        </div>
      ) : null}
    </Card>
  );
}
