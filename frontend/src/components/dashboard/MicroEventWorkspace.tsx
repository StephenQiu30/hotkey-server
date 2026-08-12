"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Activity, GitBranch, Layers3, Loader2, RefreshCw, SearchX, ShieldQuestion } from "lucide-react";
import { MicroEventEvidenceCard } from "@/components/dashboard/MicroEventEvidenceCard";
import { MicroEventReviewPanel } from "@/components/dashboard/MicroEventReviewPanel";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  getMicroEvents,
  getMicroEventsId,
  getMicroEventsIdEvidence,
  postMicroEventsIdEvidenceEvidenceIdFeedback,
} from "@/services/hotkey/hotkey-server/microEvents";
import { postContentLineageDecisionsIdFeedback } from "@/services/hotkey/hotkey-server/contentLineage";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { useAuthStore } from "@/stores/authStore";

const evidenceStateLabels: Record<string, string> = {
  no_citable_body: "没有可引用正文",
  single_origin: "单一独立起源",
  multiple_origins: "多个独立起源",
  conflicting_reports: "存在相反表述",
  publisher_corrected: "发布方已更正",
  publisher_withdrawn: "发布方已撤回",
};
const statusLabels: Record<string, string> = { active: "活跃", review_pending: "待复核", closed: "已关闭", merged: "已合并" };
const sourceTypeLabels: Record<string, string> = {
  rss: "RSS",
  hacker_news: "Hacker News",
  x: "X / Twitter",
  bing_grounding: "Bing",
  bilibili: "B 站",
  weibo: "微博",
  google_agent_search: "Google Search",
};
const relations = ["asserts", "attributes_to", "mentions", "contradicts", "corrects", "withdraws", "unknown"] as const;
const lineageFeedbackTypes = ["duplicate", "not_duplicate", "relation_override", "withdraw"] as const;
const contentRelations = ["exact_copy", "near_duplicate", "syndicated_from", "translation_of", "revision_of", "unrelated"] as const;

function formatTime(value?: string) {
  if (!value) return "时间未提供";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "时间未提供" : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function localDateTimeToRFC3339(value: string) {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? undefined : date.toISOString();
}

export function MicroEventWorkspace() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const role = useAuthStore((state) => state.user?.role);
  const canReview = role === "editor" || role === "admin";
  const initialEventID = Number(searchParams.get("event")) || undefined;
  const [status, setStatus] = useState(searchParams.get("status") || "all");
  const requestedSort = searchParams.get("sort");
  const [sort, setSort] = useState<"heat" | "relevance" | "latest">(
    requestedSort === "latest" || requestedSort === "relevance" ? requestedSort : "heat",
  );
  const [monitorID, setMonitorID] = useState(searchParams.get("monitor_id") || "all");
  const requestedSourceType = searchParams.get("source_type") || "all";
  const [sourceType, setSourceType] = useState(requestedSourceType === "all" || sourceTypeLabels[requestedSourceType] ? requestedSourceType : "all");
  const requestedEvidenceState = searchParams.get("evidence_state") || "all";
  const [evidenceState, setEvidenceState] = useState(
    requestedEvidenceState === "all" || evidenceStateLabels[requestedEvidenceState] ? requestedEvidenceState : "all",
  );
  const [startedFrom, setStartedFrom] = useState("");
  const [startedTo, setStartedTo] = useState("");
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [events, setEvents] = useState<HotKeyAPI.MicroEventResponseDTO[]>([]);
  const [selectedID, setSelectedID] = useState<number | undefined>(initialEventID);
  const [selected, setSelected] = useState<HotKeyAPI.MicroEventResponseDTO>();
  const [evidence, setEvidence] = useState<HotKeyAPI.ClaimEvidenceResponseDTO[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [cursorHistory, setCursorHistory] = useState<string[]>([""]);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [detailError, setDetailError] = useState<string>();
  const [correction, setCorrection] = useState<HotKeyAPI.ClaimEvidenceResponseDTO>();
  const [correctionRelation, setCorrectionRelation] = useState<(typeof relations)[number]>("asserts");
  const [correctionError, setCorrectionError] = useState<string>();
  const [correctionBusy, setCorrectionBusy] = useState(false);
  const [lineageReview, setLineageReview] = useState<HotKeyAPI.ClaimEvidenceResponseDTO>();
  const [lineageFeedbackType, setLineageFeedbackType] = useState<(typeof lineageFeedbackTypes)[number]>("not_duplicate");
  const [contentRelation, setContentRelation] = useState<(typeof contentRelations)[number]>("near_duplicate");
  const [lineageError, setLineageError] = useState<string>();
  const [lineageBusy, setLineageBusy] = useState(false);
  const detailHeading = useRef<HTMLHeadingElement>(null);

  const currentCursor = cursorHistory[page] || undefined;
  const loadEvents = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const result = await getMicroEvents({
        limit: 30,
        sort,
        cursor: currentCursor,
        ...(status !== "all" ? { status } : {}),
        ...(monitorID !== "all" ? { monitor_id: Number(monitorID) } : {}),
        ...(sourceType !== "all" ? { source_type: sourceType } : {}),
        ...(evidenceState !== "all" ? { evidence_state: evidenceState } : {}),
        ...(localDateTimeToRFC3339(startedFrom) ? { started_from: localDateTimeToRFC3339(startedFrom) } : {}),
        ...(localDateTimeToRFC3339(startedTo) ? { started_to: localDateTimeToRFC3339(startedTo) } : {}),
      });
      const items = result.data?.items ?? [];
      setEvents(items);
      setNextCursor(result.data?.next_cursor);
      setSelectedID((current) => {
        const preferred = current ?? initialEventID;
        return items.some((item) => item.id === preferred) ? preferred : items[0]?.id;
      });
    } catch (reason) {
      setEvents([]);
      setError(reason instanceof Error ? reason.message : "微事件加载失败");
    } finally {
      setLoading(false);
    }
  }, [currentCursor, evidenceState, initialEventID, monitorID, sort, sourceType, startedFrom, startedTo, status]);

  const loadDetail = useCallback(async () => {
    if (!selectedID) {
      setSelected(undefined);
      setEvidence([]);
      return;
    }
    setDetailLoading(true);
    setDetailError(undefined);
    const [detailResult, evidenceResult] = await Promise.allSettled([
      getMicroEventsId({ id: selectedID }),
      getMicroEventsIdEvidence({ id: selectedID, limit: 100 }),
    ]);
    if (detailResult.status === "fulfilled" && detailResult.value.data) setSelected(detailResult.value.data);
    else setDetailError(detailResult.status === "rejected" && detailResult.reason instanceof Error ? detailResult.reason.message : "事件详情不可用");
    if (evidenceResult.status === "fulfilled") setEvidence(evidenceResult.value.data?.items ?? []);
    else setEvidence([]);
    setDetailLoading(false);
  }, [selectedID]);

  useEffect(() => { void loadEvents(); }, [loadEvents]);
  useEffect(() => { void loadDetail(); }, [loadDetail]);
  useEffect(() => {
    let active = true;
    void getMonitors({ limit: 100 })
      .then((result) => { if (active) setMonitors(result.data?.items ?? []); })
      .catch(() => { if (active) setMonitors([]); });
    return () => { active = false; };
  }, []);

  const resetPagination = () => {
    setPage(0);
    setCursorHistory([""]);
  };

  const selectEvent = (id?: number) => {
    if (!id) return;
    setSelectedID(id);
    const next = new URLSearchParams(searchParams.toString());
    next.set("event", String(id));
    router.replace(`/dashboard/events?${next.toString()}`, { scroll: false });
    window.setTimeout(() => detailHeading.current?.focus(), 0);
  };

  const evidenceTimeline = useMemo(
    () => evidence.toSorted((left, right) => Date.parse(right.published_at ?? right.captured_at ?? "") - Date.parse(left.published_at ?? left.captured_at ?? "")),
    [evidence],
  );

  return (
    <main className="space-y-6 pb-12">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="eyebrow">Semantic event monitoring</p>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">热点事件与出处证据</h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">按微事件与长期 Storyline 分层浏览。证据状态只描述可引用正文和独立起源，不代表事实真假或来源等级。</p>
        </div>
        <Button onClick={() => void Promise.all([loadEvents(), loadDetail()])} type="button" variant="outline"><RefreshCw />刷新</Button>
      </header>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div className="space-y-2">
          <Label htmlFor="micro-event-sort">排序</Label>
          <Select value={sort} onValueChange={(value) => { setSort(value as "heat" | "relevance" | "latest"); resetPagination(); }}>
            <SelectTrigger id="micro-event-sort"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="heat">正在升温</SelectItem><SelectItem value="relevance">相关性最高</SelectItem><SelectItem value="latest">最新发现</SelectItem></SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="micro-event-status">状态</Label>
          <Select value={status} onValueChange={(value) => { setStatus(value); resetPagination(); }}>
          <SelectTrigger id="micro-event-status"><SelectValue /></SelectTrigger>
          <SelectContent><SelectItem value="all">全部</SelectItem>{Object.entries(statusLabels).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="micro-event-monitor">监控器</Label>
          <Select value={monitorID} onValueChange={(value) => { setMonitorID(value); resetPagination(); }}>
            <SelectTrigger id="micro-event-monitor"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="all">全部监控器</SelectItem>{monitors.map((monitor) => monitor.id ? <SelectItem key={monitor.id} value={String(monitor.id)}>{monitor.name || `监控器 #${monitor.id}`}</SelectItem> : null)}</SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="micro-event-source">来源</Label>
          <Select value={sourceType} onValueChange={(value) => { setSourceType(value); resetPagination(); }}>
            <SelectTrigger id="micro-event-source"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="all">全部来源</SelectItem>{Object.entries(sourceTypeLabels).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="micro-event-evidence-state">证据状态</Label>
          <Select value={evidenceState} onValueChange={(value) => { setEvidenceState(value); resetPagination(); }}>
            <SelectTrigger id="micro-event-evidence-state"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="all">全部证据状态</SelectItem>{Object.entries(evidenceStateLabels).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="micro-event-started-from">事件开始时间从</Label>
          <Input id="micro-event-started-from" onChange={(event) => { setStartedFrom(event.target.value); resetPagination(); }} type="datetime-local" value={startedFrom} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="micro-event-started-to">事件开始时间到</Label>
          <Input id="micro-event-started-to" onChange={(event) => { setStartedTo(event.target.value); resetPagination(); }} type="datetime-local" value={startedTo} />
        </div>
      </div>

      {error ? <Alert variant="destructive"><AlertTitle>事件列表加载失败</AlertTitle><AlertDescription>{error}</AlertDescription></Alert> : null}
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(17rem,0.72fr)_minmax(0,1.55fr)]">
        <section aria-label="微事件列表" className="space-y-3">
          {loading ? <Loading label="加载微事件" /> : null}
          {!loading && events.length === 0 ? <Empty><EmptyHeader><EmptyMedia variant="icon"><SearchX /></EmptyMedia><EmptyTitle>暂无微事件</EmptyTitle><EmptyDescription>只有有效相关性决策和可用谱系事实才会进入微事件。</EmptyDescription></EmptyHeader></Empty> : null}
          {events.map((item) => (
            <button aria-pressed={selectedID === item.id} className="w-full rounded-lg text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" key={item.id} onClick={() => selectEvent(item.id)} type="button">
              <Card className={selectedID === item.id ? "border-primary bg-primary/[0.03]" : "transition-colors hover:border-foreground/30"}>
                <CardContent className="space-y-3 py-5">
                  <div className="flex items-center justify-between gap-2"><Badge variant="outline">{statusLabels[item.status ?? ""] ?? "状态未知"}</Badge><span className="text-xs text-muted-foreground">#{item.id}</span></div>
                  <h2 className="font-semibold leading-6">{item.primary_subject_key || "未命名主体"} · {item.primary_action_key || "未命名动作"}</h2>
                  <p className="line-clamp-2 text-sm text-muted-foreground">{item.storyline?.title || item.storyline?.summary || "尚未关联长期 Storyline"}</p>
                  <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
                    <span>{item.latest_heat?.heat_score != null ? `热度 ${item.latest_heat.heat_score.toFixed(1)}` : "热度待计算"}</span>
                    <span>{item.relevance_score != null ? `相关性 ${Math.round(item.relevance_score * 100)}%` : "相关性待分析"}</span>
                    <span>{item.content_family_count ?? 0} 个独立家族</span><span>{item.document_count ?? 0} 个正文版本</span><span>{formatTime(item.event_started_at)}</span>
                  </div>
                </CardContent>
              </Card>
            </button>
          ))}
          <div className="flex items-center justify-between gap-2 pt-2">
            <Button disabled={page === 0 || loading} onClick={() => setPage((value) => Math.max(0, value - 1))} type="button" variant="outline">上一页</Button>
            <span className="text-xs text-muted-foreground">第 {page + 1} 页</span>
            <Button disabled={!nextCursor || loading} onClick={() => { if (!nextCursor) return; setCursorHistory((history) => [...history.slice(0, page + 1), nextCursor]); setPage((value) => value + 1); }} type="button" variant="outline">下一页</Button>
          </div>
        </section>

        <section aria-busy={detailLoading} aria-label="微事件详情" className="min-w-0 space-y-5">
          {detailLoading ? <Loading label="加载事件详情与证据" /> : null}
          {detailError ? <Alert variant="destructive"><AlertTitle>事件详情加载失败</AlertTitle><AlertDescription>{detailError}</AlertDescription></Alert> : null}
          {!detailLoading && selected ? (
            <>
              <Card>
                <CardHeader className="space-y-4">
                  <div className="flex flex-wrap items-center gap-2"><Badge>{statusLabels[selected.status ?? ""] ?? "状态未知"}</Badge>{selected.evidence_state ? <Badge variant="outline">{evidenceStateLabels[selected.evidence_state.state ?? ""] ?? "证据状态未知"}</Badge> : null}</div>
                  <h2 className="text-xl font-semibold outline-none" ref={detailHeading} tabIndex={-1}>{selected.primary_subject_key || "未命名主体"} · {selected.primary_action_key || "未命名动作"}</h2>
                  <p className="text-sm text-muted-foreground">事件版本 v{selected.version ?? "—"} · {formatTime(selected.event_started_at)}{selected.event_ended_at ? ` 至 ${formatTime(selected.event_ended_at)}` : ""}</p>
                </CardHeader>
                <CardContent className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                  <OverviewMetric icon={<GitBranch />} label="Storyline" value={selected.storyline?.title || "尚未关联"} detail={selected.storyline?.summary} />
                  <OverviewMetric icon={<Activity />} label="Heat v2" value={selected.latest_heat?.heat_score != null ? selected.latest_heat.heat_score.toFixed(1) : "暂无快照"} detail={selected.latest_heat ? `${selected.latest_heat.independent_lineage_root_count ?? 0} 个独立起源；互动缺失会自动重归一化` : undefined} />
                  <OverviewMetric icon={<Layers3 />} label="最高监控相关性" value={selected.relevance_score != null ? `${Math.round(selected.relevance_score * 100)}%` : "暂无结果"} detail="用于监控意图匹配，不等同于事实判断" />
                  <OverviewMetric icon={<ShieldQuestion />} label="证据覆盖" value={evidenceStateLabels[selected.evidence_state?.state ?? ""] ?? "暂无快照"} detail={selected.evidence_state ? `${selected.evidence_state.independent_origin_count ?? 0} 个独立起源` : "不以相关性或模型分数替代证据覆盖"} />
                </CardContent>
              </Card>

              <EvidenceSummary summary={selected.evidence_summary} />

              <div className="flex items-center justify-between gap-3"><div><h3 className="font-semibold">证据时间线</h3><p className="text-sm text-muted-foreground">每条关系绑定 exact DocumentVersion 与 TextQuoteSelector。</p></div><Badge variant="secondary">{evidenceTimeline.length} 条</Badge></div>
              {evidenceTimeline.length === 0 ? <Empty><EmptyHeader><EmptyMedia variant="icon"><Layers3 /></EmptyMedia><EmptyTitle>尚无可展示证据</EmptyTitle><EmptyDescription>没有可引用正文时，系统不会用搜索摘要或生成内容冒充证据。</EmptyDescription></EmptyHeader></Empty> : evidenceTimeline.map((item) => <MicroEventEvidenceCard canReview={canReview} evidence={item} key={item.id} onCorrect={(value) => { setCorrection(value); setCorrectionRelation((value.relation as (typeof relations)[number]) || "unknown"); setCorrectionError(undefined); }} onReviewLineage={(value) => { setLineageReview(value); setLineageFeedbackType("not_duplicate"); setContentRelation("near_duplicate"); setLineageError(undefined); }} />)}
              {canReview ? <MicroEventReviewPanel event={selected} onCompleted={loadDetail} /> : null}
            </>
          ) : null}
        </section>
      </div>

      <EvidenceCorrectionDialog busy={correctionBusy} error={correctionError} evidence={correction} onOpenChange={(open) => { if (!open) setCorrection(undefined); }} onSubmit={async (selectorID, relation, reasonCode, note) => {
        if (!selected?.id || !correction?.id || !correction.version) return;
        setCorrectionBusy(true); setCorrectionError(undefined);
        try {
          await postMicroEventsIdEvidenceEvidenceIdFeedback({ id: selected.id, evidence_id: correction.id }, { expected_claim_version: correction.version, result_text_quote_selector_id: selectorID, result_relation: relation, reason_code: reasonCode, note: note || undefined }, { headers: { "Content-Type": "application/json", "If-Match": `"v${correction.version}"`, "Idempotency-Key": crypto.randomUUID() } });
          setCorrection(undefined); await loadDetail();
        } catch (reason) { setCorrectionError(reason instanceof Error ? reason.message : "证据纠错保存失败"); }
        finally { setCorrectionBusy(false); }
      }} relation={correctionRelation} setRelation={setCorrectionRelation} />
      <ContentLineageReviewDialog busy={lineageBusy} error={lineageError} evidence={lineageReview} feedbackType={lineageFeedbackType} onOpenChange={(open) => { if (!open) setLineageReview(undefined); }} onSubmit={async (targetParentID, targetMemberVersion, reasonCode, note) => {
        if (!lineageReview?.lineage_decision_id || !lineageReview.content_family_member_version) return;
        setLineageBusy(true); setLineageError(undefined);
        try {
          await postContentLineageDecisionsIdFeedback({ id: lineageReview.lineage_decision_id }, {
            expected_member_version: lineageReview.content_family_member_version,
            feedback_type: lineageFeedbackType,
            relation_override: lineageFeedbackType === "relation_override" ? contentRelation : undefined,
            target_parent_document_version_id: targetParentID || undefined,
            expected_target_member_version: targetMemberVersion || undefined,
            reason_code: reasonCode,
            note: note || undefined,
          }, { headers: { "Content-Type": "application/json", "If-Match": `"v${lineageReview.content_family_member_version}"`, "Idempotency-Key": crypto.randomUUID() } });
          setLineageReview(undefined); await loadDetail();
        } catch (reason) { setLineageError(reason instanceof Error ? reason.message : "正文谱系反馈保存失败"); }
        finally { setLineageBusy(false); }
      }} relation={contentRelation} setFeedbackType={setLineageFeedbackType} setRelation={setContentRelation} />
    </main>
  );
}

function Loading({ label }: { label: string }) { return <div className="flex min-h-40 items-center justify-center gap-2 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" /><span>{label}</span></div>; }
function OverviewMetric({ icon, label, value, detail }: { icon: React.ReactNode; label: string; value: string; detail?: string }) { return <div className="rounded-lg border border-border p-4"><div className="flex items-center gap-2 text-xs text-muted-foreground">{icon}<span>{label}</span></div><p className="mt-3 font-medium">{value}</p>{detail ? <p className="mt-2 text-xs leading-5 text-muted-foreground">{detail}</p> : null}</div>; }

function EvidenceSummary({ summary }: { summary?: HotKeyAPI.EvidenceSummaryResponseDTO }) {
  if (!summary?.sentences?.length) return null;
  return (
    <Card aria-label="证据化摘要">
      <CardHeader className="space-y-1">
        <h3 className="font-semibold">证据化摘要</h3>
        <p className="text-sm text-muted-foreground">每句话均引用不可变证据版本；人工编辑内容会明确标记。</p>
      </CardHeader>
      <CardContent>
        <ol className="space-y-4">
          {summary.sentences.map((sentence) => (
            <li className="rounded-lg border border-border p-4" key={sentence.id ?? sentence.ordinal}>
              <div className="flex flex-wrap gap-2">
                {sentence.editorial_note ? <Badge variant="secondary">人工编辑</Badge> : <Badge variant="outline">自动生成</Badge>}
                <span className="text-xs text-muted-foreground">摘要句 {Number(sentence.ordinal ?? 0) + 1}</span>
              </div>
              <p className="mt-3 text-sm leading-6">{sentence.text}</p>
              <p className="mt-3 text-xs text-muted-foreground">
                证据版本：{sentence.claim_evidence_version_ids?.length ? sentence.claim_evidence_version_ids.map((id) => `#${id}`).join("、") : "未绑定（不应发布）"}
              </p>
            </li>
          ))}
        </ol>
        <p className="mt-4 text-xs text-muted-foreground">摘要配置：{summary.summary_profile_version || "未提供"} · 事件版本 v{summary.event_version ?? "—"}</p>
      </CardContent>
    </Card>
  );
}

function EvidenceCorrectionDialog({ evidence, relation, setRelation, busy, error, onOpenChange, onSubmit }: { evidence?: HotKeyAPI.ClaimEvidenceResponseDTO; relation: (typeof relations)[number]; setRelation: (value: (typeof relations)[number]) => void; busy: boolean; error?: string; onOpenChange: (open: boolean) => void; onSubmit: (selectorID: number, relation: string, reasonCode: string, note: string) => Promise<void> }) {
  return <Dialog open={Boolean(evidence)} onOpenChange={onOpenChange}><DialogContent className="max-h-[90vh] overflow-y-auto"><DialogHeader><DialogTitle>纠正证据关系或摘录</DialogTitle><DialogDescription>保存新版本并保留原关系、原选择器和操作人记录。</DialogDescription></DialogHeader>{error ? <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert> : null}<form className="space-y-4" onSubmit={(event) => { event.preventDefault(); const form = new FormData(event.currentTarget); void onSubmit(Number(form.get("selector_id")), relation, String(form.get("reason_code") ?? "").trim(), String(form.get("note") ?? "").trim()); }}><div className="space-y-2"><Label htmlFor="correct-selector">新引用选择器 ID</Label><Input id="correct-selector" name="selector_id" required /></div><div className="space-y-2"><Label htmlFor="correct-relation">新关系</Label><Select onValueChange={(value) => setRelation(value as (typeof relations)[number])} value={relation}><SelectTrigger id="correct-relation"><SelectValue /></SelectTrigger><SelectContent>{relations.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select></div><div className="space-y-2"><Label htmlFor="correct-reason">原因代码</Label><Input id="correct-reason" name="reason_code" required /></div><div className="space-y-2"><Label htmlFor="correct-note">备注（可选）</Label><Input id="correct-note" name="note" /></div><Button disabled={busy} type="submit">{busy ? <Loader2 className="animate-spin" /> : null}保存新证据版本</Button></form></DialogContent></Dialog>;
}

function ContentLineageReviewDialog({ evidence, feedbackType, setFeedbackType, relation, setRelation, busy, error, onOpenChange, onSubmit }: { evidence?: HotKeyAPI.ClaimEvidenceResponseDTO; feedbackType: (typeof lineageFeedbackTypes)[number]; setFeedbackType: (value: (typeof lineageFeedbackTypes)[number]) => void; relation: (typeof contentRelations)[number]; setRelation: (value: (typeof contentRelations)[number]) => void; busy: boolean; error?: string; onOpenChange: (open: boolean) => void; onSubmit: (targetParentID: number, targetMemberVersion: number, reasonCode: string, note: string) => Promise<void> }) {
  const needsParent = feedbackType === "duplicate" || feedbackType === "relation_override" && relation !== "unrelated";
  return <Dialog open={Boolean(evidence)} onOpenChange={onOpenChange}><DialogContent className="max-h-[90vh] overflow-y-auto"><DialogHeader><DialogTitle>复核正文谱系</DialogTitle><DialogDescription>追加人工谱系事实；原自动判定与历史家族成员不会被覆盖。</DialogDescription></DialogHeader>{error ? <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert> : null}<form className="space-y-4" onSubmit={(event) => { event.preventDefault(); const form = new FormData(event.currentTarget); void onSubmit(Number(form.get("target_parent_id")), Number(form.get("target_member_version")), String(form.get("reason_code") ?? "").trim(), String(form.get("note") ?? "").trim()); }}><div className="space-y-2"><Label htmlFor="lineage-feedback-type">反馈类型</Label><Select onValueChange={(value) => setFeedbackType(value as (typeof lineageFeedbackTypes)[number])} value={feedbackType}><SelectTrigger id="lineage-feedback-type"><SelectValue /></SelectTrigger><SelectContent>{lineageFeedbackTypes.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select></div>{feedbackType === "relation_override" ? <div className="space-y-2"><Label htmlFor="lineage-relation">关系覆盖</Label><Select onValueChange={(value) => setRelation(value as (typeof contentRelations)[number])} value={relation}><SelectTrigger id="lineage-relation"><SelectValue /></SelectTrigger><SelectContent>{contentRelations.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select></div> : null}{needsParent ? <div className="grid gap-3 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="lineage-target-parent">目标父正文版本 ID</Label><Input id="lineage-target-parent" name="target_parent_id" required /></div><div className="space-y-2"><Label htmlFor="lineage-target-version">目标成员版本</Label><Input id="lineage-target-version" name="target_member_version" required /></div></div> : null}<div className="space-y-2"><Label htmlFor="lineage-reason">原因代码</Label><Input id="lineage-reason" name="reason_code" required /></div><div className="space-y-2"><Label htmlFor="lineage-note">备注（可选）</Label><Input id="lineage-note" name="note" /></div><Button disabled={busy} type="submit">{busy ? <Loader2 className="animate-spin" /> : null}保存谱系反馈</Button></form></DialogContent></Dialog>;
}
