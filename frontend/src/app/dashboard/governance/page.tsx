"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, Archive, Gauge, Loader2, ScrollText } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { RuntimeOperationsPanel } from "@/components/operations/RuntimeOperationsPanel";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";
import { PageShell } from "@/layouts/PageShell";
import {
  getOperationsAuditLogs, getOperationsRetentionPolicies, getOperationsRetentionRunsId, getOperationsUsage,
  postOperationsRetentionPoliciesIdPreview, postOperationsRetentionRunsIdApprove,
  postOperationsRetentionRunsIdExecute,
} from "@/services/hotkey/hotkey-server/operations";

const AUDIT_LIMIT = 20;
const dataClassLabels: Record<string, string> = {
  captured_items: "采集临时项", content_metric_snapshots: "内容指标", event_metric_snapshots: "事件指标",
  sessions: "失效会话", delivery_attempts: "投递尝试（受保护）", job_attempts: "任务尝试", audit_logs: "审计日志",
};

type RetentionPreview = { policy: HotKeyAPI.RetentionPolicyResponse; result: HotKeyAPI.CleanupResult };

export default function GovernancePage() {
  const user = useAuthStore((state) => state.user);
  const canManage = user?.role === UserRole.Admin;
  const [usage, setUsage] = useState<HotKeyAPI.UsageItem[]>([]);
  const [policies, setPolicies] = useState<HotKeyAPI.RetentionPolicyResponse[]>([]);
  const [audit, setAudit] = useState<HotKeyAPI.AuditRecord[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [cursorHistory, setCursorHistory] = useState<(string | undefined)[]>([undefined]);
  const [auditPage, setAuditPage] = useState(1);
  const [loading, setLoading] = useState(canManage);
  const [auditLoading, setAuditLoading] = useState(false);
  const [error, setError] = useState("");
  const [batchSize, setBatchSize] = useState("100");
  const [actionFilter, setActionFilter] = useState("all");
  const [resourceFilter, setResourceFilter] = useState("all");
  const [resultFilter, setResultFilter] = useState("all");
  const [appliedFilters, setAppliedFilters] = useState({ action: "all", resource: "all", result: "all" });
  const [preview, setPreview] = useState<RetentionPreview>();
  const [busyPolicy, setBusyPolicy] = useState<number>();
  const [handoffRunID, setHandoffRunID] = useState("");
  const [handoffLoading, setHandoffLoading] = useState(false);
  const previewTriggerRef = useRef<HTMLButtonElement>(null);

  const auditParams = useCallback((cursor?: string) => ({
    limit: AUDIT_LIMIT,
    ...(cursor ? { cursor } : {}),
    ...(appliedFilters.action !== "all" ? { action: appliedFilters.action } : {}),
    ...(appliedFilters.resource !== "all" ? { resource_type: appliedFilters.resource } : {}),
    ...(appliedFilters.result !== "all" ? { result: appliedFilters.result } : {}),
  }), [appliedFilters]);

  const load = useCallback(async () => {
    if (!canManage) return;
    setLoading(true);
    setError("");
    try {
      const [usageResult, policyResult, auditResult] = await Promise.all([
        getOperationsUsage(), getOperationsRetentionPolicies(), getOperationsAuditLogs(auditParams()),
      ]);
      setUsage(usageResult.data?.items ?? []);
      setPolicies(policyResult.data ?? []);
      setAudit(auditResult.data?.items ?? []);
      setNextCursor(auditResult.data?.next_cursor);
      setCursorHistory([undefined]);
      setAuditPage(1);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "治理数据加载失败");
    } finally {
      setLoading(false);
    }
  }, [auditParams, canManage]);

  useEffect(() => { void load(); }, [load]);

  const loadAudit = async (cursor: string | undefined, page: number) => {
    setAuditLoading(true);
    try {
      const result = await getOperationsAuditLogs(auditParams(cursor));
      setAudit(result.data?.items ?? []);
      setNextCursor(result.data?.next_cursor);
      setAuditPage(page);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "审计加载失败");
    } finally { setAuditLoading(false); }
  };

  const applyAuditFilters = () => {
    setAppliedFilters({ action: actionFilter, resource: resourceFilter, result: resultFilter });
  };

  const usageGroups = useMemo(() => {
    const byDimension = new Map(usage.map((item) => [item.dimension, item]));
    const aiCost = byDimension.get("ai_cost");
    const aiTokens = byDimension.get("ai_tokens");
    return [
      byDimension.get("active_monitors"), byDimension.get("manual_searches"), byDimension.get("source_calls"),
      aiCost ? { ...aiCost, label: "AI Token / 成本", unit: "", used: `${aiTokens?.used ?? "0"} tokens · $${aiCost.settled ?? aiCost.used ?? "0"}` } : undefined,
      byDimension.get("notification_deliveries"),
    ].filter(Boolean) as HotKeyAPI.UsageItem[];
  }, [usage]);

  const previewRetention = async (policy: HotKeyAPI.RetentionPolicyResponse, trigger: HTMLButtonElement) => {
    if (policy.id == null || policy.version == null) return;
    previewTriggerRef.current = trigger;
    setBusyPolicy(policy.id);
    try {
      const result = await postOperationsRetentionPoliciesIdPreview({ id: policy.id }, { expected_version: policy.version, batch_size: Number(batchSize) });
      if (result.data) setPreview({ policy, result: result.data });
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "保留预览失败"); }
    finally { setBusyPolicy(undefined); }
  };

  const loadRetentionRun = async (trigger: HTMLButtonElement) => {
    const runID = Number(handoffRunID.trim());
    if (!Number.isSafeInteger(runID) || runID <= 0) {
      toast.error("请输入有效的待审批保留运行 ID");
      return;
    }
    previewTriggerRef.current = trigger;
    setHandoffLoading(true);
    try {
      const result = await getOperationsRetentionRunsId({ id: runID });
      if (!result.data || !["pending_approval", "approved"].includes(result.data.status ?? "")) {
        toast.error("该保留运行当前不可审批或执行");
        return;
      }
      const policy = policies.find((item) => item.data_class === result.data?.data_class);
      if (!policy) {
        toast.error("找不到该保留运行对应的数据策略");
        return;
      }
      setPreview({ policy, result: result.data });
      toast.success(`已载入保留运行 #${runID}`);
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "保留运行加载失败"); }
    finally { setHandoffLoading(false); }
  };

  const approveRetention = async () => {
    const runID = preview?.result.run_id;
    const candidateHash = preview?.result.candidate_hash;
    const policyID = preview?.policy.id;
    if (runID == null || !candidateHash || policyID == null) return;
    setBusyPolicy(policyID);
    try {
      const result = await postOperationsRetentionRunsIdApprove({ id: runID }, { candidate_hash: candidateHash });
      if (result.data) setPreview((current) => current ? { ...current, result: result.data! } : current);
      toast.success("固定候选清单已批准，执行前仍会重新校验");
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "保留批准失败"); }
    finally { setBusyPolicy(undefined); }
  };

  const executeRetention = async () => {
    const runID = preview?.result.run_id;
    const candidateHash = preview?.result.candidate_hash;
    const policyID = preview?.policy.id;
    if (runID == null || !candidateHash || policyID == null || preview?.result.status !== "approved") return;
    setBusyPolicy(policyID);
    try {
      const result = await postOperationsRetentionRunsIdExecute({ id: runID }, { candidate_hash: candidateHash });
      toast.success(`已处理 ${result.data?.affected ?? 0} 条${result.data?.has_more ? "，仍有后续批次" : ""}`);
      setPreview(undefined);
      await load();
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "保留执行失败"); }
    finally { setBusyPolicy(undefined); }
  };

  const isPreviewRequester = preview?.result.requested_by_user_id === user?.id;

  if (!canManage) {
    return null;
  }

  return <PageShell>
    <PageHeader eyebrow="Governance" title="配额与审计" description="统一查看成本与调用量，预览后分批清理可删除数据，并追踪关键操作。" action={<Button variant="outline" onClick={() => void load()} disabled={loading}>{loading ? <Loader2 className="animate-spin" /> : null}刷新</Button>} />
    {error ? <Alert variant="destructive" className="mt-6"><AlertCircle /><AlertTitle>无法加载治理数据</AlertTitle><AlertDescription className="flex flex-wrap items-center justify-between gap-3"><span>{error}</span><Button size="sm" variant="outline" onClick={() => void load()}>重试</Button></AlertDescription></Alert> : null}

    <RuntimeOperationsPanel />

    <section className="mt-8" aria-labelledby="usage-title">
      <div className="mb-4 flex items-center gap-2"><Gauge className="h-4 w-4 text-muted-foreground" /><h2 id="usage-title" className="text-base font-semibold">今日用量</h2></div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
        {loading ? Array.from({ length: 5 }, (_, index) => <Skeleton key={index} className="h-40" />) : usageGroups.map((item) => <UsageCard key={item.dimension} item={item} />)}
      </div>
    </section>

    <section className="mt-10" aria-labelledby="retention-title">
      <Card className="overflow-hidden">
        <CardHeader className="flex flex-row flex-wrap items-center gap-3 border-b"><div><CardTitle id="retention-title" className="flex items-center gap-2"><Archive className="h-4 w-4" />数据保留</CardTitle><p className="mt-2 text-sm text-muted-foreground">先生成固定候选清单与 Hash，明确批准后执行；每次最多处理 1000 条。</p></div>
          <Select value={batchSize} onValueChange={setBatchSize}><SelectTrigger aria-label="保留批量上限" className="ml-auto w-36"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="100">每批 100 条</SelectItem><SelectItem value="500">每批 500 条</SelectItem><SelectItem value="1000">每批 1000 条</SelectItem></SelectContent></Select></CardHeader>
          <CardContent className="border-b py-4">
            <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
              <div className="space-y-2"><label htmlFor="retention-run-id" className="text-sm font-medium">待审批保留运行 ID</label><Input id="retention-run-id" inputMode="numeric" value={handoffRunID} onChange={(event) => setHandoffRunID(event.target.value)} placeholder="例如 11" /></div>
              <Button variant="outline" disabled={handoffLoading} onClick={(event) => void loadRetentionRun(event.currentTarget)}>{handoffLoading ? <Loader2 className="animate-spin" /> : null}加载待审批运行</Button>
            </div>
            <p className="mt-2 text-xs text-muted-foreground">由另一名管理员输入发起人共享的 Run ID；服务端会读取冻结 Hash，不返回原始候选数据。</p>
          </CardContent>
          <Table className="min-w-[760px]" scrollAreaLabel="数据保留策略表"><TableHeader><TableRow><TableHead>数据类</TableHead><TableHead>保留期</TableHead><TableHead>状态</TableHead><TableHead>说明</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>{policies.map((policy) => <TableRow key={policy.id}><TableCell className="font-medium">{dataClassLabels[policy.data_class ?? ""] ?? policy.data_class}</TableCell><TableCell>{policy.retention_days} 天</TableCell><TableCell><Badge variant={policy.protected ? "secondary" : policy.enabled ? "default" : "outline"}>{policy.protected ? "受保护" : policy.enabled ? "启用" : "停用"}</Badge></TableCell><TableCell className="max-w-sm text-xs text-muted-foreground">{policy.description}</TableCell><TableCell className="text-right"><Button size="sm" variant="outline" disabled={!policy.enabled || policy.protected || busyPolicy === policy.id} onClick={(event) => void previewRetention(policy, event.currentTarget)}>{busyPolicy === policy.id ? <Loader2 className="animate-spin" /> : null}预览清理</Button></TableCell></TableRow>)}</TableBody>
          </Table>
      </Card>
    </section>

    <section className="mt-10" aria-labelledby="audit-title">
      <Card className="overflow-hidden"><CardHeader className="border-b"><CardTitle id="audit-title" className="flex items-center gap-2"><ScrollText className="h-4 w-4" />审计记录</CardTitle>
        <div className="mt-4 grid gap-3 md:grid-cols-[1fr_1fr_1fr_auto]">
          <Select value={actionFilter} onValueChange={setActionFilter}><SelectTrigger aria-label="筛选审计动作"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部动作</SelectItem><SelectItem value="retention.previewed">保留预览</SelectItem><SelectItem value="retention.approved">保留批准</SelectItem><SelectItem value="retention.executed">保留执行</SelectItem><SelectItem value="retention.blocked">保留阻断</SelectItem><SelectItem value="monitor.published">监控发布</SelectItem><SelectItem value="source.created">来源创建</SelectItem><SelectItem value="subscription.created">订阅创建</SelectItem></SelectContent></Select>
          <Select value={resourceFilter} onValueChange={setResourceFilter}><SelectTrigger aria-label="筛选资源类型"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部资源</SelectItem><SelectItem value="retention_run">保留运行</SelectItem><SelectItem value="monitor">监控</SelectItem><SelectItem value="source_connection">来源连接</SelectItem><SelectItem value="report_subscription">报告订阅</SelectItem></SelectContent></Select>
          <Select value={resultFilter} onValueChange={setResultFilter}><SelectTrigger aria-label="筛选审计结果"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部结果</SelectItem><SelectItem value="success">成功</SelectItem><SelectItem value="failure">失败</SelectItem><SelectItem value="denied">拒绝</SelectItem></SelectContent></Select>
          <Button variant="outline" onClick={applyAuditFilters}>应用筛选</Button>
        </div></CardHeader>
        {auditLoading ? <div className="space-y-3 p-5" aria-label="正在加载审计">{Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-10" />)}</div> : audit.length === 0 ? <Empty className="min-h-56 rounded-none border-0"><EmptyHeader><EmptyMedia variant="icon"><ScrollText /></EmptyMedia><EmptyTitle>暂无匹配审计</EmptyTitle><EmptyDescription>调整筛选条件或在工作区完成一次受审计操作。</EmptyDescription></EmptyHeader></Empty> : <Table className="min-w-[760px]" scrollAreaLabel="审计记录表"><TableHeader><TableRow><TableHead>时间</TableHead><TableHead>动作</TableHead><TableHead>资源</TableHead><TableHead>操作者</TableHead><TableHead>结果</TableHead></TableRow></TableHeader><TableBody>{audit.map((record) => <TableRow key={record.id}><TableCell className="text-xs text-muted-foreground">{record.created_at ? new Date(record.created_at).toLocaleString("zh-CN") : "—"}</TableCell><TableCell className="font-mono text-xs">{record.action}</TableCell><TableCell>{record.resource_type}{record.resource_id ? ` #${record.resource_id}` : ""}</TableCell><TableCell>{record.actor_type}{record.actor_id ? ` #${record.actor_id}` : ""}</TableCell><TableCell><Badge variant={record.result === "success" ? "default" : "destructive"}>{record.result === "success" ? "成功" : record.result === "denied" ? "拒绝" : "失败"}</Badge></TableCell></TableRow>)}</TableBody></Table>}
        <CardContent className="flex items-center justify-between border-t py-4"><p className="text-xs text-muted-foreground">第 {auditPage} 页</p><div className="flex gap-2"><Button size="sm" variant="outline" disabled={auditPage === 1 || auditLoading} onClick={() => void loadAudit(cursorHistory[auditPage - 2], auditPage - 1)}>上一页</Button><Button size="sm" variant="outline" disabled={!nextCursor || auditLoading} onClick={() => { if (!nextCursor) return; setCursorHistory((items) => [...items.slice(0, auditPage), nextCursor]); void loadAudit(nextCursor, auditPage + 1); }}>下一页</Button></div></CardContent>
      </Card>
    </section>

    <AlertDialog open={Boolean(preview)} onOpenChange={(open) => !open && setPreview(undefined)}><AlertDialogContent onCloseAutoFocus={(event) => { event.preventDefault(); previewTriggerRef.current?.focus(); }}><AlertDialogHeader><AlertDialogTitle>{preview?.result.status === "approved" ? "执行已批准保留批次？" : isPreviewRequester ? "等待另一名管理员批准" : "批准固定保留清单？"}</AlertDialogTitle><AlertDialogDescription asChild><div className="space-y-2 text-sm text-muted-foreground"><p>{dataClassLabels[preview?.policy.data_class ?? ""] ?? preview?.policy.data_class} 的 dry-run 找到 {preview?.result.affected ?? 0} 条候选，截止 {preview?.result.cutoff ? new Date(preview.result.cutoff).toLocaleString("zh-CN") : "—"}。{preview?.result.has_more ? "本批完成后仍有后续候选。" : "本批可处理全部候选。"}</p><p>运行 #{preview?.result.run_id ?? "—"}；{preview?.result.status === "approved" ? "候选 Hash 已冻结，执行前将再次校验策略与清单。" : isPreviewRequester ? "请将 Run ID 交给另一名管理员；发起人与批准人必须不同。" : "批准后只能按该 Hash 执行；发起人与批准人必须不同。"}</p><code className="block break-all rounded bg-muted px-2 py-1 text-xs text-foreground">{preview?.result.candidate_hash ?? "—"}</code></div></AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>{isPreviewRequester && preview?.result.status !== "approved" ? "关闭" : "取消"}</AlertDialogCancel>{preview?.result.status === "approved" ? <AlertDialogAction disabled={!preview?.result.affected || busyPolicy != null} onClick={(event) => { event.preventDefault(); void executeRetention(); }}>{busyPolicy != null ? <Loader2 className="animate-spin" /> : null}执行 {preview?.result.affected ?? 0} 条</AlertDialogAction> : !isPreviewRequester ? <AlertDialogAction disabled={!preview?.result.affected || preview?.result.run_id == null || !preview?.result.candidate_hash || busyPolicy != null} onClick={(event) => { event.preventDefault(); void approveRetention(); }}>{busyPolicy != null ? <Loader2 className="animate-spin" /> : null}批准固定清单</AlertDialogAction> : null}</AlertDialogFooter></AlertDialogContent></AlertDialog>
  </PageShell>;
}

function UsageCard({ item }: { item: HotKeyAPI.UsageItem }) {
  const numericUsed = item.dimension === "ai_cost"
    ? Number(item.settled ?? 0) + Number(item.reserved ?? 0)
    : Number(item.used ?? 0);
  const numericLimit = Number(item.limit ?? 0);
  const progress = numericLimit > 0 ? Math.min(100, (numericUsed / numericLimit) * 100) : 0;
  return <Card className="gap-3"><CardHeader className="pb-0"><div className="flex items-center justify-between gap-2"><CardTitle className="text-sm">{item.label}</CardTitle><Badge variant="outline">{item.mode === "hard" ? "硬配额" : "观测"}</Badge></div></CardHeader><CardContent><p className="text-2xl font-semibold tabular-nums">{item.used} <span className="text-xs font-normal text-muted-foreground">{item.unit}</span></p>{item.limit != null ? <><Progress className="mt-4" value={progress} aria-label={`${item.label} 已使用 ${item.used}，上限 ${item.limit}`} /><p className="mt-2 text-xs text-muted-foreground">剩余 {item.remaining ?? "—"} / {item.limit}</p></> : <p className="mt-4 text-xs text-muted-foreground">按事实记录，不设产品硬上限</p>}{item.reserved != null ? <p className="mt-2 text-xs text-muted-foreground">预留 ${item.reserved} · 结算 ${item.settled}</p> : null}</CardContent></Card>;
}
