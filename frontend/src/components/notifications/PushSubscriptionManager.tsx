"use client";

import { useEffect, useMemo, useState } from "react";
import { BellRing, Loader2, RefreshCw, ShieldCheck, Smartphone } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { PushSubscriptionDeviceCard } from "@/components/notifications/PushSubscriptionDeviceCard";
import { AuthStatus } from "@/lib/domainEnums";
import {
  browserDeviceLabel,
  browserPushSubscriptionDTO,
  browserTimezone,
  createPushRegistrationIdempotencyKey,
  currentPushSubscriptionID,
  forgetCurrentPushSubscriptionID,
  getWebPushSupport,
  registerHotKeyServiceWorker,
  rememberCurrentPushSubscriptionID,
  subscribeBrowserPush,
  WEB_PUSH_DEFAULT_TTL_SECONDS,
  type WebPushSupport,
} from "@/lib/webPush";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import {
  getNotificationsPushCapability,
  getNotificationsPushSubscriptions,
  postNotificationsPushSubscriptions,
} from "@/services/hotkey/hotkey-server/notifications";
import { useAuthStore } from "@/stores/authStore";

type MonitorOption = Pick<HotKeyAPI.MonitorResponse, "id" | "name" | "status">;

function validMonitors(items?: HotKeyAPI.MonitorResponse[]) {
  return (items ?? []).filter(
    (monitor): monitor is MonitorOption & { id: number } =>
      Boolean(monitor.id && monitor.status !== "archived"),
  );
}

async function currentBrowserPushSubscription() {
  if (!("serviceWorker" in navigator)) return null;
  const registration = await navigator.serviceWorker.getRegistration("/");
  return registration?.pushManager.getSubscription() ?? null;
}

export function PushSubscriptionManager() {
  const authStatus = useAuthStore((state) => state.status);
  const userID = useAuthStore((state) => state.user?.id ?? 0);
  const [support, setSupport] = useState<WebPushSupport | null>(null);
  const [permission, setPermission] = useState<NotificationPermission | "unsupported">(
    "unsupported",
  );
  const [capability, setCapability] = useState<HotKeyAPI.PushCapabilityResponseDTO | null>(
    null,
  );
  const [subscriptions, setSubscriptions] = useState<
    HotKeyAPI.PushSubscriptionResponseDTO[]
  >([]);
  const [monitors, setMonitors] = useState<MonitorOption[]>([]);
  const [currentSubscriptionID, setCurrentSubscriptionID] = useState<number | null>(null);
  const [browserSubscriptionPresent, setBrowserSubscriptionPresent] = useState(false);
  const [deviceLabel, setDeviceLabel] = useState("");
  const [timezone, setTimezone] = useState("UTC");
  const [monitorIDs, setMonitorIDs] = useState<number[]>([]);
  const [quietEnabled, setQuietEnabled] = useState(false);
  const [quietStart, setQuietStart] = useState("22:00");
  const [quietEnd, setQuietEnd] = useState("08:00");
  const [ttlSeconds, setTTLSeconds] = useState(String(WEB_PUSH_DEFAULT_TTL_SECONDS));
  const [loading, setLoading] = useState(true);
  const [enabling, setEnabling] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  const activeSubscriptions = useMemo(
    () => subscriptions.filter((subscription) => subscription.status === "active"),
    [subscriptions],
  );
  const canEnable =
    support?.available === true &&
    capability?.available === true &&
    Boolean(capability.vapid_public_key) &&
    permission !== "denied" &&
    monitorIDs.length > 0 &&
    deviceLabel.trim().length > 0;

  const load = async () => {
    if (authStatus !== AuthStatus.Authenticated || userID <= 0) return;
    setLoading(true);
    setLoadError(null);
    const detectedSupport = getWebPushSupport();
    setSupport(detectedSupport);
    setPermission(
      detectedSupport.available && "Notification" in window
        ? Notification.permission
        : "unsupported",
    );
    setDeviceLabel((value) => value || browserDeviceLabel());
    setTimezone((value) => (value === "UTC" ? browserTimezone() : value));

    try {
      const [capabilityResponse, subscriptionsResponse, monitorsResponse, browserSubscription] =
        await Promise.all([
          getNotificationsPushCapability(),
          getNotificationsPushSubscriptions(),
          getMonitors({ limit: 100 }),
          detectedSupport.available ? currentBrowserPushSubscription() : Promise.resolve(null),
        ]);
      const loadedSubscriptions = subscriptionsResponse.data?.items ?? [];
      const loadedMonitors = validMonitors(monitorsResponse.data?.items);
      setCapability(capabilityResponse.data ?? { available: false });
      setSubscriptions(loadedSubscriptions);
      setMonitors(loadedMonitors);
      setBrowserSubscriptionPresent(Boolean(browserSubscription));

      const rememberedID = currentPushSubscriptionID(userID);
      const remembered = loadedSubscriptions.find((item) => item.id === rememberedID);
      if (browserSubscription && remembered?.status === "active") {
        setCurrentSubscriptionID(rememberedID);
      } else {
        if (browserSubscription && remembered && remembered.status !== "active") {
          await browserSubscription.unsubscribe().catch(() => false);
          setBrowserSubscriptionPresent(false);
        }
        forgetCurrentPushSubscriptionID(userID);
        setCurrentSubscriptionID(null);
      }
    } catch (reason) {
      setLoadError(reason instanceof Error ? reason.message : "设备通知设置加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // Loading is intentionally keyed only by durable auth identity. User edits
    // are interaction-owned and must not be reset by incidental renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authStatus, userID]);

  const toggleMonitor = (monitorID: number, checked: boolean) => {
    setMonitorIDs((current) =>
      checked
        ? [...new Set([...current, monitorID])].sort((left, right) => left - right)
        : current.filter((id) => id !== monitorID),
    );
  };

  const enable = async () => {
    if (!canEnable || userID <= 0 || !capability?.vapid_public_key) return;
    if (quietEnabled && quietStart === quietEnd) {
      toast.error("免打扰开始与结束时间不能相同。");
      return;
    }
    setEnabling(true);
    let createdBrowserSubscription: PushSubscription | null = null;
    try {
      const requestedPermission = await Notification.requestPermission();
      setPermission(requestedPermission);
      if (requestedPermission !== "granted") {
        throw new Error(
          requestedPermission === "denied"
            ? "浏览器已拒绝通知权限，可在站点设置中重新授权。"
            : "未获得通知权限，未创建任何订阅。",
        );
      }

      const registration = await registerHotKeyServiceWorker();
      const browserResult = await subscribeBrowserPush(
        registration,
        capability.vapid_public_key,
      );
      if (browserResult.created) createdBrowserSubscription = browserResult.subscription;
      const browserDTO = browserPushSubscriptionDTO(browserResult.subscription);
      const response = await postNotificationsPushSubscriptions(
        {
          endpoint: browserDTO.endpoint,
          keys: browserDTO.keys,
          device_label: deviceLabel.trim(),
          timezone: timezone.trim(),
          quiet_start: quietEnabled ? quietStart : undefined,
          quiet_end: quietEnabled ? quietEnd : undefined,
          ttl_seconds: Number(ttlSeconds),
          monitor_ids: monitorIDs,
        },
        { headers: { "Idempotency-Key": createPushRegistrationIdempotencyKey() } },
      );
      const created = response.data;
      if (!created?.id || !created.version || created.status !== "active") {
        throw new Error("设备订阅响应不完整");
      }
      rememberCurrentPushSubscriptionID(userID, created.id);
      setCurrentSubscriptionID(created.id);
      setBrowserSubscriptionPresent(true);
      setSubscriptions((current) => {
        const withoutCreated = current.filter((item) => item.id !== created.id);
        return [...withoutCreated, created].sort((left, right) => (left.id ?? 0) - (right.id ?? 0));
      });
      toast.success("浏览器通知已启用");
    } catch (reason) {
      if (createdBrowserSubscription) {
        await createdBrowserSubscription.unsubscribe().catch(() => false);
      }
      toast.error(reason instanceof Error ? reason.message : "浏览器通知启用失败");
    } finally {
      setEnabling(false);
    }
  };

  const updateSubscription = (updated: HotKeyAPI.PushSubscriptionResponseDTO) => {
    setSubscriptions((current) =>
      current.map((subscription) =>
        subscription.id === updated.id ? updated : subscription,
      ),
    );
  };

  const disableSubscription = async (disabled: HotKeyAPI.PushSubscriptionResponseDTO) => {
    updateSubscription(disabled);
    if (disabled.id !== currentSubscriptionID || userID <= 0) return;
    const browserSubscription = await currentBrowserPushSubscription().catch(() => null);
    if (browserSubscription) await browserSubscription.unsubscribe().catch(() => false);
    forgetCurrentPushSubscriptionID(userID);
    setCurrentSubscriptionID(null);
    setBrowserSubscriptionPresent(false);
  };

  const deploymentUnavailable = capability && !capability.available;
  const permissionDenied = permission === "denied";

  return (
    <section aria-labelledby="push-notification-heading" className="mt-8">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h2 id="push-notification-heading" className="text-lg font-semibold">
              手机与浏览器通知
            </h2>
            {capability?.available ? <Badge variant="outline">Web Push</Badge> : null}
          </div>
          <p className="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">
            在设备离开当前页面后继续接收所选监控的事件通知。权限必须由你主动开启，推送内容不包含正文、摘录或登录凭据。
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={load} disabled={loading}>
          {loading ? <Loader2 className="animate-spin" /> : <RefreshCw />}
          刷新设备
        </Button>
      </div>

      {loadError ? (
        <Alert variant="destructive" className="mt-4">
          <AlertTitle>设备设置加载失败</AlertTitle>
          <AlertDescription>{loadError}</AlertDescription>
        </Alert>
      ) : null}
      {support && !support.available ? (
        <Alert className="mt-4">
          <Smartphone />
          <AlertTitle>当前设备暂不支持推送</AlertTitle>
          <AlertDescription>{support.reason}</AlertDescription>
        </Alert>
      ) : null}
      {deploymentUnavailable ? (
        <Alert className="mt-4">
          <ShieldCheck />
          <AlertTitle>服务端尚未启用 Web Push</AlertTitle>
          <AlertDescription>
            站内实时通知仍可正常使用；管理员配置 VAPID 与订阅加密密钥后，这里会自动开放设备订阅。
          </AlertDescription>
        </Alert>
      ) : null}
      {permissionDenied ? (
        <Alert variant="destructive" className="mt-4">
          <AlertTitle>浏览器通知权限已被拒绝</AlertTitle>
          <AlertDescription>
            HotKey 不会反复弹出权限请求。请在浏览器站点设置中允许通知后再刷新设备状态。
          </AlertDescription>
        </Alert>
      ) : null}
      {browserSubscriptionPresent && currentSubscriptionID === null ? (
        <Alert className="mt-4">
          <AlertTitle>发现尚未关联的浏览器订阅</AlertTitle>
          <AlertDescription>
            请选择监控并点击启用，系统会重新核对并关联此浏览器；不会在本地保存推送地址或密钥。
          </AlertDescription>
        </Alert>
      ) : null}

      {loading ? (
        <Card className="mt-4 flex min-h-32 items-center justify-center border">
          <Loader2 className="animate-spin text-muted-foreground" aria-label="正在加载设备通知设置" />
        </Card>
      ) : support?.available && capability?.available && currentSubscriptionID === null ? (
        <Card className="mt-4 border p-4 sm:p-5">
          <div className="flex items-start gap-3">
            <span className="rounded-full bg-primary/10 p-2 text-primary" aria-hidden="true">
              <BellRing className="h-4 w-4" />
            </span>
            <div>
              <h3 className="font-medium">在此设备启用通知</h3>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                先选择需要接收的监控，再由浏览器询问权限。未点击按钮前不会请求系统通知权限。
              </p>
            </div>
          </div>

          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="new-push-device-label">设备名称</Label>
              <Input
                id="new-push-device-label"
                value={deviceLabel}
                maxLength={80}
                onChange={(event) => setDeviceLabel(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="new-push-timezone">时区</Label>
              <Input
                id="new-push-timezone"
                value={timezone}
                maxLength={64}
                onChange={(event) => setTimezone(event.target.value)}
              />
            </div>
          </div>

          <fieldset className="mt-5 space-y-2">
            <legend className="text-sm font-medium">接收哪些监控</legend>
            {monitors.length === 0 ? (
              <p className="rounded-lg border border-dashed p-3 text-sm text-muted-foreground">
                当前没有可订阅的监控。请先创建并保留至少一个未归档监控。
              </p>
            ) : (
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
            )}
          </fieldset>

          <div className="mt-5 grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="new-push-ttl">通知有效期</Label>
              <Select value={ttlSeconds} onValueChange={setTTLSeconds}>
                <SelectTrigger id="new-push-ttl">
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
              <Label htmlFor="new-push-quiet-start">免打扰开始</Label>
              <Input
                id="new-push-quiet-start"
                type="time"
                value={quietStart}
                disabled={!quietEnabled}
                onChange={(event) => setQuietStart(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="new-push-quiet-end">免打扰结束</Label>
              <Input
                id="new-push-quiet-end"
                type="time"
                value={quietEnd}
                disabled={!quietEnabled}
                onChange={(event) => setQuietEnd(event.target.value)}
              />
            </div>
          </div>
          <Label className="mt-4 flex items-center gap-3 font-normal">
            <Checkbox
              checked={quietEnabled}
              onCheckedChange={(checked) => setQuietEnabled(checked === true)}
            />
            启用每日免打扰时段
          </Label>

          <div className="mt-5 flex flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs leading-5 text-muted-foreground">
              权限状态：
              {permission === "granted"
                ? "已允许"
                : permission === "denied"
                  ? "已拒绝"
                  : "等待你的操作"}
            </p>
            <Button onClick={enable} disabled={!canEnable || enabling}>
              {enabling ? <Loader2 className="animate-spin" /> : <BellRing />}
              启用此设备通知
            </Button>
          </div>
        </Card>
      ) : null}

      {subscriptions.length > 0 ? (
        <div className="mt-5 space-y-3" aria-label="已登记的通知设备">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-medium">已登记设备</h3>
            <span className="text-xs text-muted-foreground">
              {activeSubscriptions.length} 台启用 / {subscriptions.length} 台记录
            </span>
          </div>
          {subscriptions.map((subscription) => (
            <PushSubscriptionDeviceCard
              key={`${subscription.id}-${subscription.version}`}
              subscription={subscription}
              monitors={monitors}
              currentBrowser={subscription.id === currentSubscriptionID}
              onUpdated={updateSubscription}
              onDisabled={disableSubscription}
            />
          ))}
        </div>
      ) : null}
    </section>
  );
}
