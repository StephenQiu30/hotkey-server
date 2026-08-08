"use client";

import { useCallback, useEffect, useState } from "react";
import { Eye, FileText, Loader2, Plus, RefreshCw, Send } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { CursorPagination, DEFAULT_PAGE_SIZE, hasNextCursor } from "@/components/dashboard/CursorPagination";
import { ReportStatus, ReportType, UserRole } from "@/lib/domainEnums";
import { reportTypeLabel } from "@/lib/domainPresentation";
import { formatRadarTime } from "@/lib/radarPresentation";
import { getReports, postReports, postReportsIdBuild, postReportsIdPreview, postReportsIdPublish } from "@/services/hotkey/hotkey-server/reports";
import { useAuthStore } from "@/stores/authStore";

const statusLabels: Record<string, string> = { draft: "草稿", published: "已发布", failed: "失败", archived: "已归档" };

export function ReportWorkspace() {
  const role = useAuthStore((state) => state.user?.role);
  const canBuild = role === UserRole.Editor || role === UserRole.Admin;
  const canPublish = role === UserRole.Admin;
  const [reports, setReports] = useState<HotKeyAPI.ReportResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [type, setType] = useState("all");
  const [status, setStatus] = useState("all");
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(number | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<number>();
  const [createOpen, setCreateOpen] = useState(false);
  const [busy, setBusy] = useState<number | "create">();
  const [detailOpen, setDetailOpen] = useState(false);
  const [detail, setDetail] = useState<HotKeyAPI.ReportResponse>();
  const [detailLoading, setDetailLoading] = useState(false);
  const [form, setForm] = useState({ type: ReportType.Daily as ReportType, timezone: "Asia/Shanghai", monitorID: "" });

  const loadPage = useCallback(async (cursor?: number, pageNumber = 1) => {
    setLoading(true);
    setError(undefined);
    try {
      const result = await getReports({ limit: pageSize, ...(cursor ? { cursor } : {}), ...(type !== "all" ? { type } : {}), ...(status !== "all" ? { status } : {}) });
      setReports(result.data?.items ?? []);
      setNextCursor(result.data?.next_cursor);
      setPage(pageNumber);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "报告加载失败");
    } finally { setLoading(false); }
  }, [pageSize, status, type]);

  const resetAndLoad = useCallback(async () => { setCursors([undefined]); await loadPage(undefined, 1); }, [loadPage]);
  useEffect(() => { void resetAndLoad(); }, [resetAndLoad]);

  const create = async () => {
    setBusy("create");
    try {
      const result = await postReports({ type: form.type, timezone: form.timezone, ...(form.monitorID ? { monitor_id: Number(form.monitorID) } : {}) });
      setCreateOpen(false);
      toast.success(`已生成${reportTypeLabel(result.data?.type)}草稿`);
      await resetAndLoad();
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "草稿生成失败"); }
    finally { setBusy(undefined); }
  };

  const openPreview = async (report: HotKeyAPI.ReportResponse) => {
    if (report.id == null) return;
    setDetailOpen(true); setDetail(undefined); setDetailLoading(true);
    try { const result = await postReportsIdPreview({ id: report.id }); setDetail(result.data?.report); }
    catch (reason) { toast.error(reason instanceof Error ? reason.message : "报告预览失败"); setDetailOpen(false); }
    finally { setDetailLoading(false); }
  };

  const mutate = async (report: HotKeyAPI.ReportResponse, action: "build" | "publish") => {
    if (report.id == null) return;
    setBusy(report.id);
    try {
      if (action === "build") await postReportsIdBuild({ id: report.id }); else await postReportsIdPublish({ id: report.id });
      toast.success(action === "build" ? "报告草稿已刷新" : "报告已发布并生成交付与知识提案");
      await resetAndLoad();
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "报告操作失败"); }
    finally { setBusy(undefined); }
  };

  return <section aria-labelledby="reports-title" className="space-y-6">
    <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div><h2 id="reports-title" className="text-2xl font-semibold tracking-[-0.04em]">日报与周报</h2><p className="mt-2 text-sm text-muted-foreground">每个条目固定到周期内的事件版本与证据哈希，发布后不可修改。</p></div>
      {canBuild && <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogTrigger asChild><Button><Plus />新建报告</Button></DialogTrigger>
        <DialogContent><DialogHeader><DialogTitle>生成报告草稿</DialogTitle><DialogDescription>相同类型、监控和周期会刷新同一份草稿。</DialogDescription></DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="space-y-2"><Label>报告周期</Label><Select value={form.type} onValueChange={(value) => setForm({ ...form, type: value as ReportType })}><SelectTrigger aria-label="报告周期"><SelectValue /></SelectTrigger><SelectContent><SelectItem value={ReportType.Daily}>日报</SelectItem><SelectItem value={ReportType.Weekly}>周报</SelectItem></SelectContent></Select></div>
            <div className="space-y-2"><Label>时区</Label><Select value={form.timezone} onValueChange={(timezone) => setForm({ ...form, timezone })}><SelectTrigger aria-label="报告时区"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="Asia/Shanghai">Asia/Shanghai</SelectItem><SelectItem value="UTC">UTC</SelectItem></SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="report-monitor">监控 ID（可选）</Label><Input id="report-monitor" inputMode="numeric" placeholder="全部已启用监控" value={form.monitorID} onChange={(event) => setForm({ ...form, monitorID: event.target.value.replace(/\D/g, "") })} /></div>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setCreateOpen(false)}>取消</Button><Button onClick={create} disabled={busy === "create"}>{busy === "create" && <Loader2 className="animate-spin" />}生成草稿</Button></DialogFooter>
        </DialogContent>
      </Dialog>}
    </div>
    <div className="flex flex-col gap-3 sm:flex-row">
      <Select value={type} onValueChange={setType}><SelectTrigger aria-label="报告类型筛选" className="sm:w-40"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部类型</SelectItem><SelectItem value="daily">日报</SelectItem><SelectItem value="weekly">周报</SelectItem></SelectContent></Select>
      <Select value={status} onValueChange={setStatus}><SelectTrigger aria-label="报告状态筛选" className="sm:w-40"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="draft">草稿</SelectItem><SelectItem value="published">已发布</SelectItem><SelectItem value="failed">失败</SelectItem><SelectItem value="archived">已归档</SelectItem></SelectContent></Select>
      <Button variant="outline" onClick={() => void resetAndLoad()}><RefreshCw />刷新</Button>
    </div>
    {error ? <Alert variant="destructive"><AlertTitle>无法加载报告</AlertTitle><AlertDescription className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><span>{error}</span><Button variant="outline" size="sm" onClick={() => void resetAndLoad()}>重试</Button></AlertDescription></Alert> : loading ? <Card className="space-y-4 p-6">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-12 w-full" />)}</Card> : reports.length === 0 ? <Card><Empty className="h-72"><EmptyHeader><EmptyMedia variant="icon"><FileText /></EmptyMedia><EmptyTitle>当前筛选下没有报告</EmptyTitle><EmptyDescription>{canBuild ? "生成第一份草稿，系统会固定周期内的事件证据。" : "编辑者生成报告后会出现在这里。"}</EmptyDescription></EmptyHeader></Empty></Card> : <Card className="overflow-hidden py-0"><Table><TableHeader><TableRow><TableHead>报告</TableHead><TableHead className="hidden md:table-cell">周期</TableHead><TableHead>状态</TableHead><TableHead className="hidden lg:table-cell">条目</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{reports.map((report) => <TableRow key={report.id}><TableCell><div className="font-medium">{report.title || `${reportTypeLabel(report.type)}报告`}</div><div className="mt-1 text-xs text-muted-foreground">{report.monitor_id ? `监控 #${report.monitor_id}` : "全部监控"} · {report.timezone}</div></TableCell><TableCell className="hidden text-xs text-muted-foreground md:table-cell">{formatRadarTime(report.period_start)}<br />至 {formatRadarTime(report.period_end)}</TableCell><TableCell><Badge variant={report.status === "published" ? "default" : "secondary"}>{statusLabels[report.status ?? ""] ?? report.status}</Badge></TableCell><TableCell className="hidden lg:table-cell">{report.items?.length ?? 0}</TableCell><TableCell><div className="flex flex-wrap justify-end gap-2"><Button variant="ghost" size="sm" onClick={() => void openPreview(report)}><Eye />预览</Button>{canBuild && report.status === ReportStatus.Draft && <Button variant="outline" size="sm" disabled={busy === report.id} onClick={() => void mutate(report, "build")}><RefreshCw />刷新</Button>}{canPublish && report.status === ReportStatus.Draft && <Button size="sm" disabled={busy === report.id} onClick={() => void mutate(report, "publish")}><Send />发布</Button>}</div></TableCell></TableRow>)}</TableBody></Table><CursorPagination hasNext={hasNextCursor(nextCursor)} loading={loading} onNext={() => { if (!nextCursor) return; const next = page + 1; setCursors((items) => [...items.slice(0, page), nextCursor]); void loadPage(nextCursor, next); }} onPageSizeChange={setPageSize} onPrevious={() => { if (page > 1) void loadPage(cursors[page - 2], page - 1); }} page={page} pageSize={pageSize} /></Card>}
    <Dialog open={detailOpen} onOpenChange={setDetailOpen}><DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"><DialogHeader><DialogTitle>{detail?.title ?? "报告预览"}</DialogTitle><DialogDescription>{detail ? `${reportTypeLabel(detail.type)} · ${formatRadarTime(detail.period_start)} 至 ${formatRadarTime(detail.period_end)}` : "正在读取固定快照"}</DialogDescription></DialogHeader>{detailLoading ? <div className="flex h-48 items-center justify-center"><Loader2 className="animate-spin" /></div> : detail ? <div className="space-y-4"><Alert><AlertTitle>{detail.status === "published" ? "已冻结发布版本" : "只读草稿预览"}</AlertTitle><AlertDescription>{detail.summary || "当前周期没有符合条件的事件。"}</AlertDescription></Alert>{detail.items?.length ? detail.items.map((item) => <Card key={item.event_update_id} className="p-4"><div className="flex items-start justify-between gap-4"><div><p className="font-medium">{item.rank}. {item.title}</p><p className="mt-2 text-sm text-muted-foreground">{item.summary}</p></div><Badge variant="outline">热度 {item.heat_score?.toFixed(1)}</Badge></div><dl className="mt-4 grid gap-2 text-xs text-muted-foreground sm:grid-cols-2"><div><dt className="font-medium text-foreground">EventUpdate</dt><dd>#{item.event_update_id}</dd></div><div><dt className="font-medium text-foreground">入选理由</dt><dd>{item.reason_codes?.join(" · ")}</dd></div><div className="sm:col-span-2"><dt className="font-medium text-foreground">证据集哈希</dt><dd className="break-all font-mono">{item.evidence_set_hash}</dd></div></dl></Card>) : <Empty><EmptyHeader><EmptyTitle>本周期没有事件快照</EmptyTitle><EmptyDescription>报告仍可发布为空报，便于维持固定交付节奏。</EmptyDescription></EmptyHeader></Empty>}</div> : null}</DialogContent></Dialog>
  </section>;
}
