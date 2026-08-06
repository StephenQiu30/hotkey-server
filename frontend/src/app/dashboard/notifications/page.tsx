"use client";

import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, Rss, RotateCw, Send, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  deleteReportSubscriptionsId,
  getReportSubscriptions,
  patchReportSubscriptionsId,
  postReportSubscriptions,
  postReportSubscriptionsIdRssTokenRotate,
} from "@/services/hotkey/hotkey-server/delivery";
import { PageHeader } from "@/components/dashboard/PageHeader";
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

export default function SubscriptionsPage() {
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [subscriptions, setSubscriptions] = useState<
    HotKeyAPI.SubscriptionResponse[]
  >([]);
  const [loading, setLoading] = useState(true);
  const [dialog, setDialog] = useState(false);
  const [action, setAction] = useState<number>();
  const [deleteTarget, setDeleteTarget] = useState<HotKeyAPI.SubscriptionResponse>();
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [form, setForm] = useState({
    recipient: "",
  });
  const recipientValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(
    form.recipient.trim(),
  );

  const loadPage = useCallback(async (cursor: string | undefined, pageNumber: number) => {
    setLoading(true);
    try {
      const subscriptionResult = await getReportSubscriptions({
        limit: pageSize,
        ...(cursor ? { cursor } : {}),
      });
      setSubscriptions(subscriptionResult.data?.items ?? []);
      setNextCursor(subscriptionResult.data?.next_cursor);
      setPage(pageNumber);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "订阅加载失败");
    } finally {
      setLoading(false);
    }
  }, [pageSize]);

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
        channel: DeliveryChannel.Email,
        report_type: ReportType.Daily,
        recipient: form.recipient.trim(),
        schedule: "0 9 * * *",
        timezone: "Asia/Shanghai",
        enabled: true,
      });
      setDialog(false);
      await load();
      if (result.data?.rss_token)
        toast.success(`RSS Token 已生成：${result.data.rss_token}`);
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
        },
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
        { expected_version: subscription.version ?? 0 },
      );
      toast.success(`新 RSS Token：${result.data?.rss_token || "已生成"}`);
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
        { expected_version: deleteTarget.version ?? 0 },
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
    <div className="app-page">
      <PageHeader
        eyebrow="Delivery"
        title="发布订阅"
        description="按监控订阅日报或周报，通过邮件或私有 RSS 获取。"
        action={
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
              </DialogHeader>
              <div className="space-y-4 py-2">
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
                  <p className="mt-2 text-xs text-muted-foreground">
                    系统每天 09:00 自动整理所有已启用关键词监测结果并发送邮件。
                  </p>
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setDialog(false)}>
                  取消
                </Button>
                <Button
                  onClick={create}
                  disabled={!recipientValid}
                >
                  创建订阅
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        }
      />
      {loading ? (
        <div className="flex h-72 items-center justify-center">
          <Loader2 className="animate-spin text-muted-foreground" />
        </div>
      ) : !subscriptions.length ? (
        <div className="panel mt-6 flex h-72 flex-col items-center justify-center">
          <Send className="mb-3 h-6 w-6 text-muted-foreground" />
          <p className="text-sm">暂时没有发布订阅</p>
        </div>
      ) : (
        <div className="panel mt-6 overflow-hidden">
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
                      <Send className="h-3.5 w-3.5 text-blue-400" />
                    )}
                    <p className="text-sm font-medium">
                      {reportTypeLabel(subscription.report_type)} ·{" "}
                      {deliveryChannelLabel(subscription.channel)}
                    </p>
                  </div>
                  <p className="mt-1 truncate text-xs text-muted-foreground">
                    {subscription.monitor_id == null
                      ? "全部已启用监控"
                      : `监控 #${subscription.monitor_id}`} ·{" "}
                    {subscription.recipient || subscription.timezone}
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
                    disabled={action === subscription.id || subscription.enabled}
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
        </div>
      )}
      <ConfirmDeleteDialog
        open={deleteTarget != null}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title="删除发布订阅"
        description="订阅配置会从工作区移除；已生成报告、投递结果和审计记录仍会保留。"
        resourceName={`${reportTypeLabel(deleteTarget?.report_type)} · ${deliveryChannelLabel(deleteTarget?.channel)}`}
        onConfirm={deleteSubscription}
        loading={action === deleteTarget?.id}
      />
    </div>
  );
}
