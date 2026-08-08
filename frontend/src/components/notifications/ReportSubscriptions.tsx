"use client";

import { useCallback, useEffect, useState } from "react";
import {
  AlertCircle,
  Copy,
  Loader2,
  Plus,
  Rss,
  RotateCw,
  Send,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
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
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  deleteReportSubscriptionsId,
  getReportSubscriptions,
  patchReportSubscriptionsId,
  postReportSubscriptions,
  postReportSubscriptionsIdRssTokenRotate,
} from "@/services/hotkey/hotkey-server/delivery";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";
import { DeliveryChannel, ReportType } from "@/lib/domainEnums";
import {
  deliveryChannelLabel,
  reportTypeLabel,
} from "@/lib/domainPresentation";

export function ReportSubscriptions() {
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [subscriptions, setSubscriptions] = useState<
    HotKeyAPI.SubscriptionResponse[]
  >([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [dialog, setDialog] = useState(false);
  const [action, setAction] = useState<number>();
  const [deleteTarget, setDeleteTarget] =
    useState<HotKeyAPI.SubscriptionResponse>();
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [rssToken, setRssToken] = useState<string>();
  const [form, setForm] = useState({
    recipient: "",
    channel: DeliveryChannel.Email as DeliveryChannel,
    reportType: ReportType.Daily as ReportType,
    monitorID: "",
    timezone: "Asia/Shanghai",
    hour: "9",
  });
  const recipientValid =
    form.channel === DeliveryChannel.RSS ||
    /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.recipient.trim());

  const loadPage = useCallback(
    async (cursor: string | undefined, pageNumber: number) => {
      setLoading(true);
      setError(undefined);
      try {
        const subscriptionResult = await getReportSubscriptions({
          limit: pageSize,
          ...(cursor ? { cursor } : {}),
        });
        setSubscriptions(subscriptionResult.data?.items ?? []);
        setNextCursor(subscriptionResult.data?.next_cursor);
        setPage(pageNumber);
      } catch (reason) {
        setError(reason instanceof Error ? reason.message : "订阅加载失败");
      } finally {
        setLoading(false);
      }
    },
    [pageSize]
  );

  const load = useCallback(async () => {
    setCursors([undefined]);
    await loadPage(undefined, 1);
  }, [loadPage]);
  useEffect(() => {
    load();
  }, [load]);

  const nextPage = () => {
    if (!nextCursor) return;
    const nextPageNumber = page + 1;
    setCursors((history) => [...history.slice(0, page), nextCursor]);
    void loadPage(nextCursor, nextPageNumber);
  };

  const previousPage = () => {
    if (page <= 1) return;
    void loadPage(cursors[page - 2], page - 1);
  };

  const changePageSize = (nextPageSize: number) => {
    setPageSize(nextPageSize);
  };

  const create = async () => {
    try {
      const result = await postReportSubscriptions({
        channel: form.channel,
        report_type: form.reportType,
        ...(form.channel === DeliveryChannel.Email
          ? { recipient: form.recipient.trim() }
          : {}),
        ...(form.monitorID ? { monitor_id: Number(form.monitorID) } : {}),
        schedule:
          form.reportType === ReportType.Daily
            ? `0 ${form.hour} * * *`
            : `0 ${form.hour} * * 1`,
        timezone: form.timezone,
        enabled: true,
      });
      setDialog(false);
      await load();
      if (result.data?.rss_token) setRssToken(result.data.rss_token);
      else toast.success("报告订阅已创建");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "订阅创建失败");
    }
  };

  const toggle = async (subscription: HotKeyAPI.SubscriptionResponse) => {
    if (subscription.id == null) return;
    setAction(subscription.id);
    try {
      await patchReportSubscriptionsId(
        { id: subscription.id },
        {
          expected_version: subscription.version ?? 0,
          enabled: !subscription.enabled,
        }
      );
      await load();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "订阅更新失败");
    } finally {
      setAction(undefined);
    }
  };

  const rotate = async (subscription: HotKeyAPI.SubscriptionResponse) => {
    if (subscription.id == null) return;
    setAction(subscription.id);
    try {
      const result = await postReportSubscriptionsIdRssTokenRotate(
        { id: subscription.id },
        { expected_version: subscription.version ?? 0 }
      );
      if (result.data?.rss_token) setRssToken(result.data.rss_token);
      toast.success("私有 Feed 地址已轮换，旧地址立即失效");
      await load();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Token 轮换失败");
    } finally {
      setAction(undefined);
    }
  };

  const deleteSubscription = async () => {
    if (deleteTarget?.id == null || deleteTarget.enabled) return;
    setAction(deleteTarget.id);
    try {
      await deleteReportSubscriptionsId(
        { id: deleteTarget.id },
        { expected_version: deleteTarget.version ?? 0 }
      );
      setDeleteTarget(undefined);
      await load();
      toast.success("发布订阅已删除，历史投递记录仍保留");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "订阅删除失败");
    } finally {
      setAction(undefined);
    }
  };

  return (
    <section aria-labelledby="report-subscriptions-title">
      <header className="flex flex-col gap-6 border-b border-border pb-8 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <p className="eyebrow">Delivery</p>
          <h2
            id="report-subscriptions-title"
            className="mt-3 text-2xl font-semibold tracking-[-0.04em]"
          >
            报告订阅
          </h2>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
            按监控订阅日报或周报，通过邮件或私有 RSS 获取。
          </p>
        </div>
        <div className="w-full shrink-0 sm:w-auto">
          <Dialog open={dialog} onOpenChange={setDialog}>
            <DialogTrigger asChild>
              <Button className="gap-2">
                <Plus />
                新建订阅
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>新建报告订阅</DialogTitle>
                <DialogDescription>
                  选择报告周期与交付方式；私有 Feed 密钥只展示一次。
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 py-2">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label>报告周期</Label>
                    <Select
                      value={form.reportType}
                      onValueChange={(value) =>
                        setForm({ ...form, reportType: value as ReportType })
                      }
                    >
                      <SelectTrigger aria-label="报告周期">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={ReportType.Daily}>日报</SelectItem>
                        <SelectItem value={ReportType.Weekly}>周报</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>交付方式</Label>
                    <Select
                      value={form.channel}
                      onValueChange={(value) =>
                        setForm({ ...form, channel: value as DeliveryChannel })
                      }
                    >
                      <SelectTrigger aria-label="交付方式">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={DeliveryChannel.Email}>
                          电子邮件
                        </SelectItem>
                        <SelectItem value={DeliveryChannel.RSS}>
                          私有 Feed
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                {form.channel === DeliveryChannel.Email && (
                  <div>
                    <Label htmlFor="recipient">收件邮箱</Label>
                    <Input
                      id="recipient"
                      type="email"
                      className="mt-2"
                      value={form.recipient}
                      onChange={(event) =>
                        setForm({ ...form, recipient: event.target.value })
                      }
                    />
                    {form.recipient && !recipientValid && (
                      <p className="mt-2 text-xs text-destructive" role="alert">
                        请输入有效的邮箱地址。
                      </p>
                    )}
                  </div>
                )}
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="monitor-id">监控 ID（可选）</Label>
                    <Input
                      id="monitor-id"
                      inputMode="numeric"
                      value={form.monitorID}
                      onChange={(event) =>
                        setForm({
                          ...form,
                          monitorID: event.target.value.replace(/\D/g, ""),
                        })
                      }
                      placeholder="全部已启用监控"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>发送时间</Label>
                    <Select
                      value={form.hour}
                      onValueChange={(hour) => setForm({ ...form, hour })}
                    >
                      <SelectTrigger aria-label="发送时间">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="9">09:00</SelectItem>
                        <SelectItem value="18">18:00</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>时区</Label>
                  <Select
                    value={form.timezone}
                    onValueChange={(timezone) => setForm({ ...form, timezone })}
                  >
                    <SelectTrigger aria-label="订阅时区">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="Asia/Shanghai">
                        Asia/Shanghai
                      </SelectItem>
                      <SelectItem value="UTC">UTC</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setDialog(false)}>
                  取消
                </Button>
                <Button onClick={create} disabled={!recipientValid}>
                  创建订阅
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </header>
      {error ? (
        <Alert variant="destructive" className="mt-6">
          <AlertCircle />
          <AlertTitle>无法加载报告订阅</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{error}</span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void load()}
              aria-label="重试订阅"
            >
              重试
            </Button>
          </AlertDescription>
        </Alert>
      ) : loading ? (
        <div className="flex h-72 items-center justify-center">
          <Loader2 className="animate-spin text-muted-foreground" />
        </div>
      ) : !subscriptions.length ? (
        <Card className="mt-6">
          <Empty className="h-72">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Send />
              </EmptyMedia>
              <EmptyTitle>暂时没有发布订阅</EmptyTitle>
              <EmptyDescription>
                创建订阅后，日报或周报会按计划发送到指定邮箱。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : (
        <Card className="mt-6 gap-0 overflow-hidden py-0">
          <div className="hidden grid-cols-[minmax(0,1fr)_100px_120px_250px] gap-4 border-b border-border px-5 py-3 text-xs text-muted-foreground md:grid">
            <span>订阅</span>
            <span>状态</span>
            <span>计划</span>
            <span className="text-right">操作</span>
          </div>
          <div className="divide-y divide-border">
            {subscriptions.map((subscription) => (
              <div
                key={subscription.id}
                className="grid gap-3 px-4 py-4 md:grid-cols-[minmax(0,1fr)_100px_120px_250px] md:items-center md:gap-4 md:px-5"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    {subscription.channel === DeliveryChannel.RSS ? (
                      <Rss className="h-3.5 w-3.5 text-orange-400" />
                    ) : (
                      <Send className="h-3.5 w-3.5 text-muted-foreground" />
                    )}
                    <p className="text-sm font-medium">
                      {reportTypeLabel(subscription.report_type)} ·{" "}
                      {deliveryChannelLabel(subscription.channel)}
                    </p>
                  </div>
                  <p className="mt-1 truncate text-xs text-muted-foreground">
                    {subscription.monitor_id == null
                      ? "全部已启用监控"
                      : `监控 #${subscription.monitor_id}`}{" "}
                    · {subscription.recipient || subscription.timezone}
                  </p>
                </div>
                <Badge variant="outline" className="w-fit">
                  {subscription.enabled ? "已启用" : "已停用"}
                </Badge>
                <span className="mono text-xs text-muted-foreground">
                  {subscription.schedule}
                </span>
                <div className="flex flex-wrap justify-start gap-2 md:justify-end">
                  {subscription.channel === DeliveryChannel.RSS && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => rotate(subscription)}
                      disabled={action === subscription.id}
                      className="gap-1.5"
                    >
                      <RotateCw />
                      轮换
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => toggle(subscription)}
                    disabled={action === subscription.id}
                  >
                    {subscription.enabled ? "停用" : "启用"}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setDeleteTarget(subscription)}
                    disabled={
                      action === subscription.id || subscription.enabled
                    }
                    title={subscription.enabled ? "请先停用订阅" : "删除订阅"}
                    className="gap-1.5 text-destructive hover:text-destructive"
                  >
                    <Trash2 />
                    删除
                  </Button>
                </div>
              </div>
            ))}
          </div>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPageSizeChange={changePageSize}
            onPrevious={previousPage}
            page={page}
            pageSize={pageSize}
          />
        </Card>
      )}
      <ConfirmDeleteDialog
        open={deleteTarget != null}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title="删除发布订阅"
        description="订阅配置会从工作区移除；已生成报告、投递结果和审计记录仍会保留。"
        resourceName={`${reportTypeLabel(
          deleteTarget?.report_type
        )} · ${deliveryChannelLabel(deleteTarget?.channel)}`}
        onConfirm={deleteSubscription}
        loading={action === deleteTarget?.id}
      />
      <Dialog
        open={rssToken != null}
        onOpenChange={(open) => !open && setRssToken(undefined)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>保存私有 Feed 地址</DialogTitle>
            <DialogDescription>
              密钥不会再次展示。轮换后旧地址立即失效。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            {["rss", "atom"].map((format) => {
              const url =
                typeof window === "undefined"
                  ? `/feeds/${rssToken}?format=${format}`
                  : `${window.location.origin}/feeds/${rssToken}?format=${format}`;
              return (
                <div key={format} className="space-y-2">
                  <Label>{format.toUpperCase()}</Label>
                  <div className="flex gap-2">
                    <Input
                      readOnly
                      value={url}
                      aria-label={`${format.toUpperCase()} 私有地址`}
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      aria-label={`复制 ${format.toUpperCase()} 地址`}
                      onClick={() => void navigator.clipboard.writeText(url)}
                    >
                      <Copy />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
          <DialogFooter>
            <Button onClick={() => setRssToken(undefined)}>我已保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
