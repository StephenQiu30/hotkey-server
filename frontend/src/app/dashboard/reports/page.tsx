"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { FileCheck2, FileText, Loader2, RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Surface } from "@/components/ui/surface";
import { HotKeyAPIError } from "@/lib/request";
import { UserRole } from "@/lib/domainEnums";
import {
  getReports,
  postReports,
  postReportsIdApprove,
  postReportsIdBuild,
  postReportsIdReject,
  postReportsIdSubmit,
} from "@/services/hotkey/hotkey-server/reports";
import { useAuthStore } from "@/stores/authStore";
import { PageShell } from "@/layouts/PageShell";

const statusPresentation: Readonly<Record<string, { label: string; variant: "default" | "secondary" | "outline" | "destructive" }>> = {
  draft: { label: "草稿", variant: "secondary" },
  pending_approval: { label: "待审批", variant: "outline" },
  published: { label: "已发布", variant: "default" },
  rejected: { label: "已驳回", variant: "destructive" },
  failed: { label: "失败", variant: "destructive" },
  archived: { label: "已归档", variant: "outline" },
};

function formatTime(value?: string) {
  if (!value) return "时间未知";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "时间未知";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(parsed);
}

function reportFromQuery(items: HotKeyAPI.ReportResponse[]) {
  const raw = new URLSearchParams(window.location.search).get("report");
  if (!raw || !/^[1-9][0-9]{0,18}$/.test(raw)) return items[0];
  return items.find((item) => item.id === Number(raw)) ?? items[0];
}

export default function ReportsPage() {
  const role = useAuthStore((state) => state.user?.role);
  const [items, setItems] = useState<HotKeyAPI.ReportResponse[]>([]);
  const [selectedID, setSelectedID] = useState<number>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string>();
  const [error, setError] = useState<string>();
  const [permissionDenied, setPermissionDenied] = useState(false);

  const contributor = role === UserRole.Analyst || role === UserRole.Editor || role === UserRole.Admin;
  const approver = role === UserRole.Editor || role === UserRole.Admin;

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    setPermissionDenied(false);
    try {
      const result = await getReports({ limit: 50 });
      const reports = result.data?.items ?? [];
      setItems(reports);
      const selected = reportFromQuery(reports);
      setSelectedID((current) => current ?? selected?.id);
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "日报加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const selected = useMemo(
    () => items.find((item) => item.id === selectedID) ?? items[0],
    [items, selectedID],
  );

  function mergeReport(report?: HotKeyAPI.ReportResponse) {
    if (!report?.id) return;
    setItems((current) => {
      const exists = current.some((item) => item.id === report.id);
      return exists ? current.map((item) => item.id === report.id ? report : item) : [report, ...current];
    });
    setSelectedID(report.id);
  }

  async function mutate(label: string, operation: () => Promise<{ data?: HotKeyAPI.ReportResponse }>) {
    setBusy(label);
    setError(undefined);
    setPermissionDenied(false);
    try {
      const result = await operation();
      mergeReport(result.data);
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "日报操作失败");
    } finally {
      setBusy(undefined);
    }
  }

  return (
    <PageShell>
      <PageHeader
        eyebrow="REPORT REVISION"
        title="日报工作台"
        description="从冻结事件与逐句 Evidence 构建日报；提交后由 Editor 或 Admin 审批，已发布 Revision 不可覆盖。"
        action={
          <div className="flex flex-wrap gap-2">
            {contributor ? (
              <Button
                variant="outline"
                disabled={Boolean(busy)}
                onClick={() => void mutate("create", () => postReports({ type: "daily", timezone: "Asia/Shanghai" }))}
              >
                <FileText />新建今日草稿
              </Button>
            ) : null}
            <Button variant="outline" onClick={() => void load()} disabled={loading || Boolean(busy)}>
              <RefreshCw />刷新
            </Button>
          </div>
        }
      />

      {permissionDenied ? (
        <Alert variant="destructive" className="mt-6" aria-label="日报权限不足">
          <AlertTitle>没有日报操作权限</AlertTitle>
          <AlertDescription>当前角色不能执行该日报动作；服务端权限边界没有被前端隐藏替代。</AlertDescription>
        </Alert>
      ) : error ? (
        <Alert variant="destructive" className="mt-6">
          <AlertTitle>日报请求失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {loading ? (
        <section aria-label="正在加载日报" className="mt-6 grid gap-4 lg:grid-cols-[20rem_1fr]">
          <Skeleton className="h-72" />
          <Skeleton className="h-96" />
        </section>
      ) : items.length === 0 ? (
        <Card className="mt-6">
          <Empty className="h-72">
            <EmptyHeader>
              <EmptyMedia variant="icon"><FileText /></EmptyMedia>
              <EmptyTitle role="heading" aria-level={2}>尚无日报 Revision</EmptyTitle>
              <EmptyDescription>{contributor ? "创建今日草稿后，系统会冻结当前时间窗与证据清单。" : "日报发布后会显示在这里。"}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : (
        <div className="mt-6 grid min-w-0 gap-5 lg:grid-cols-[20rem_minmax(0,1fr)]">
          <nav aria-label="日报列表" className="space-y-3">
            {items.map((report) => {
              const presentation = statusPresentation[report.status ?? ""] ?? { label: report.status ?? "未知", variant: "outline" as const };
              return (
                <Surface key={report.id} asChild variant="interactive">
                  <button
                    type="button"
                    aria-current={selected?.id === report.id ? "true" : undefined}
                    onClick={() => setSelectedID(report.id)}
                    className="w-full p-4 text-left aria-[current=true]:bg-muted/50 aria-[current=true]:[box-shadow:var(--shadow-control)]"
                  >
                    <div className="flex items-center gap-2">
                      <Badge variant={presentation.variant}>{presentation.label}</Badge>
                      <span className="ml-auto text-xs text-muted-foreground">Revision {report.version_no ?? 1}</span>
                    </div>
                    <strong className="mt-3 block text-sm">{report.title || `日报 #${report.id}`}</strong>
                    <span className="mt-1 block text-xs text-muted-foreground">{formatTime(report.period_start)}</span>
                  </button>
                </Surface>
              );
            })}
          </nav>

          {selected ? (
            <Card aria-label={`日报 ${selected.id}`} className="min-w-0">
              <CardHeader className="gap-3 border-b border-border">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={(statusPresentation[selected.status ?? ""] ?? { variant: "outline" }).variant}>
                    {(statusPresentation[selected.status ?? ""] ?? { label: selected.status ?? "未知" }).label}
                  </Badge>
                  {selected.frozen ? <Badge variant="outline"><FileCheck2 />已冻结</Badge> : null}
                  <span className="ml-auto text-xs text-muted-foreground">资源版本 {selected.version ?? 0}</span>
                </div>
                <CardTitle role="heading" aria-level={2}>{selected.title || `日报 #${selected.id}`}</CardTitle>
                <p className="text-sm text-muted-foreground">{selected.summary || "该日报没有额外摘要。"}</p>
                <div className="flex flex-wrap gap-2">
                  {contributor && selected.status === "draft" ? (
                    <>
                      <Button size="sm" variant="outline" disabled={Boolean(busy)} onClick={() => void mutate("build", () => postReportsIdBuild({ id: selected.id! }))}>
                        {busy === "build" ? <Loader2 className="animate-spin" /> : null}重新构建
                      </Button>
                      <Button size="sm" disabled={Boolean(busy)} onClick={() => void mutate("submit", () => postReportsIdSubmit({ id: selected.id! }, { expected_resource_version: selected.version! }))}>
                        {busy === "submit" ? <Loader2 className="animate-spin" /> : null}提交审批
                      </Button>
                    </>
                  ) : null}
                  {approver && selected.status === "pending_approval" ? (
                    <>
                      <Button size="sm" disabled={Boolean(busy)} onClick={() => void mutate("approve", () => postReportsIdApprove({ id: selected.id! }, { expected_resource_version: selected.version! }))}>
                        {busy === "approve" ? <Loader2 className="animate-spin" /> : null}批准并冻结
                      </Button>
                      <Button size="sm" variant="outline" disabled={Boolean(busy)} onClick={() => void mutate("reject", () => postReportsIdReject({ id: selected.id! }, { expected_resource_version: selected.version!, reason_code: "needs_revision" }))}>
                        驳回
                      </Button>
                    </>
                  ) : null}
                </div>
              </CardHeader>
              <CardContent className="space-y-5 p-5 sm:p-6">
                {selected.body ? <p className="whitespace-pre-wrap text-sm leading-7">{selected.body}</p> : null}
                {(selected.items ?? []).length === 0 ? (
                  <p className="text-sm text-muted-foreground">该时间窗没有入选事件。</p>
                ) : (selected.items ?? []).map((item, index) => (
                  <Surface key={`${item.micro_event_id ?? item.event_id ?? index}-${item.rank ?? index}`} asChild variant="ring">
                    <article className="p-4">
                      <div className="flex flex-wrap items-center gap-2">
                        <h2 className="font-medium">{item.title || `入选事件 ${index + 1}`}</h2>
                        <Badge variant="outline">热度 {(item.heat_score ?? 0).toFixed(1)}</Badge>
                      </div>
                      {item.summary ? <p className="mt-2 text-sm text-muted-foreground">{item.summary}</p> : null}
                      {(item.sentences ?? []).map((sentence) => (
                        <div key={`${sentence.source_summary_sentence_id}-${sentence.ordinal}`} className="mt-3 border-l-2 border-border pl-3 text-sm">
                          <p>{sentence.text}</p>
                          <p className="mt-1 text-xs text-muted-foreground">Evidence IDs：{(sentence.claim_evidence_version_ids ?? []).join(", ") || "无"}</p>
                        </div>
                      ))}
                    </article>
                  </Surface>
                ))}
              </CardContent>
            </Card>
          ) : null}
        </div>
      )}
    </PageShell>
  );
}
