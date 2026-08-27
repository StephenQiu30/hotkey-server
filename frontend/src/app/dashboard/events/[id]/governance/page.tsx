"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  FileWarning,
  GitMerge,
  GitPullRequestArrow,
  Loader2,
  RefreshCw,
  Scale,
  ShieldAlert,
} from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { APIErrorCode, UserRole } from "@/lib/domainEnums";
import { HotKeyAPIError } from "@/lib/request";
import {
  getMicroEvents,
  getMicroEventsId,
  getMicroEventsIdEvidence,
  postMicroEventsIdEvidenceEvidenceIdFeedback,
  postMicroEventsIdFeedback,
} from "@/services/hotkey/hotkey-server/microEvents";
import { useAuthStore } from "@/stores/authStore";

type GovernanceAction =
  | "move_member"
  | "merge_events"
  | "split_event"
  | "withdraw"
  | "close_event"
  | "reopen_event";

const eventStatusLabels: Record<string, string> = {
  active: "活跃",
  review_pending: "待复核",
  closed: "已归档",
  merged: "已合并",
};

const relationLabels: Record<string, string> = {
  asserts: "直接陈述",
  attributes_to: "归因陈述",
  mentions: "提及",
  contradicts: "相互矛盾",
  corrects: "更正",
  withdraws: "撤回",
  unknown: "关系待定",
};

function eventName(event: HotKeyAPI.MicroEventResponseDTO) {
  const subject = event.primary_subject_key?.trim();
  const action = event.primary_action_key?.trim();
  return subject && action ? `${subject} · ${action}` : subject || action || `语义事件 #${event.id ?? "—"}`;
}

function isConflict(reason: unknown) {
  return reason instanceof HotKeyAPIError &&
    (reason.status === 409 || reason.code === APIErrorCode.VersionConflict);
}

function isForbidden(reason: unknown) {
  return reason instanceof HotKeyAPIError &&
    (reason.status === 403 || reason.code === APIErrorCode.Forbidden);
}

export default function EventGovernancePage() {
  const params = useParams<{ id: string }>();
  const user = useAuthStore((state) => state.user);
  const role = user?.role as string | undefined;
  const id = Number(params.id);
  const validID = Number.isSafeInteger(id) && id > 0;
  const canGovern = role === UserRole.Editor || role === UserRole.Admin;
  const analystUnavailable = role === "analyst";
  const [event, setEvent] = useState<HotKeyAPI.MicroEventResponseDTO>();
  const [evidence, setEvidence] = useState<HotKeyAPI.ClaimEvidenceResponseDTO[]>([]);
  const [targets, setTargets] = useState<HotKeyAPI.MicroEventResponseDTO[]>([]);
  const [targetID, setTargetID] = useState("");
  const [loading, setLoading] = useState(validID);
  const [error, setError] = useState("");
  const [forbidden, setForbidden] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [mutationError, setMutationError] = useState("");
  const [busy, setBusy] = useState("");
  const [reasonCode, setReasonCode] = useState("editor_reviewed");
  const [note, setNote] = useState("");
  const [correction, setCorrection] = useState<HotKeyAPI.ClaimEvidenceResponseDTO>();
  const [replacementSelectorID, setReplacementSelectorID] = useState("");
  const [replacementRelation, setReplacementRelation] = useState("mentions");

  const load = useCallback(async () => {
    if (!validID) {
      setError("事件编号无效，请从语义事件列表重新进入。");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    setForbidden(false);
    setConflict(false);
    setMutationError("");
    try {
      const [detailResult, evidenceResult, targetResult] = await Promise.all([
        getMicroEventsId({ id }),
        getMicroEventsIdEvidence({ id, limit: 50 }),
        getMicroEvents({ limit: 100, sort: "latest", status: "active,review_pending" }),
      ]);
      if (!detailResult.data) throw new Error("事件详情为空");
      setEvent(detailResult.data);
      setEvidence(evidenceResult.data?.items ?? []);
      const candidates = (targetResult.data?.items ?? []).filter((item) => item.id != null && item.id !== id);
      setTargets(candidates);
      setTargetID((current) => candidates.some((item) => String(item.id) === current) ? current : "");
    } catch (reason) {
      setEvent(undefined);
      setEvidence([]);
      setTargets([]);
      if (isForbidden(reason)) setForbidden(true);
      else setError(reason instanceof Error ? reason.message : "事件治理数据暂时无法读取");
    } finally {
      setLoading(false);
    }
  }, [id, validID]);

  useEffect(() => {
    void load();
  }, [load]);

  const selectedTarget = useMemo(
    () => targets.find((item) => String(item.id) === targetID),
    [targetID, targets],
  );

  const governanceBody = (
    action: GovernanceAction,
    member?: HotKeyAPI.MicroEventMemberResponseDTO,
  ): HotKeyAPI.MicroEventGovernanceRequestDTO | undefined => {
    if (!event?.version || !reasonCode.trim()) return undefined;
    const targetRequired = action === "move_member" || action === "merge_events";
    if (targetRequired && (!selectedTarget?.id || !selectedTarget.version)) return undefined;
    return {
      action,
      expected_event_version: event.version,
      reason_code: reasonCode.trim(),
      note: note.trim(),
      ...(member ? {
        membership_decision_id: member.membership_decision_id,
        content_family_id: member.content_family_id,
        expected_member_version: member.version,
      } : {}),
      ...(targetRequired ? {
        target_micro_event_id: selectedTarget?.id,
        expected_target_event_version: selectedTarget?.version,
      } : {}),
    };
  };

  const govern = async (action: GovernanceAction, member?: HotKeyAPI.MicroEventMemberResponseDTO) => {
    if (!canGovern || !event?.id || !event.version) return;
    const body = governanceBody(action, member);
    if (!body) {
      setMutationError("请填写治理理由；移动或合并操作还需要选择目标事件。");
      return;
    }
    const operation = member ? `${action}:${member.id}` : action;
    setBusy(operation);
    setConflict(false);
    setMutationError("");
    try {
      await postMicroEventsIdFeedback(
        { id: event.id },
        body,
        { headers: { "If-Match": `"v${event.version}"`, "Idempotency-Key": crypto.randomUUID() } },
      );
      toast.success("治理事实已追加，正在读取最新事件版本");
      await load();
    } catch (reason) {
      if (isConflict(reason)) setConflict(true);
      else if (isForbidden(reason)) setForbidden(true);
      else setMutationError(reason instanceof Error ? reason.message : "治理操作失败");
    } finally {
      setBusy("");
    }
  };

  const correctEvidence = async () => {
    if (!canGovern || !event?.id || !correction?.id || !correction.claim_version) return;
    const selectorID = Number(replacementSelectorID);
    if (!Number.isSafeInteger(selectorID) || selectorID <= 0 || !reasonCode.trim()) {
      setMutationError("请输入有效的新引用定位器编号与治理理由。");
      return;
    }
    setBusy(`evidence:${correction.id}`);
    setConflict(false);
    setMutationError("");
    try {
      await postMicroEventsIdEvidenceEvidenceIdFeedback(
        { id: event.id, evidence_id: correction.id },
        {
          expected_claim_version: correction.claim_version,
          result_text_quote_selector_id: selectorID,
          result_relation: replacementRelation,
          reason_code: reasonCode.trim(),
          note: note.trim(),
        },
        { headers: { "If-Match": `"v${correction.claim_version}"`, "Idempotency-Key": crypto.randomUUID() } },
      );
      toast.success("Evidence 纠正已追加，原始引用仍保留");
      setCorrection(undefined);
      setReplacementSelectorID("");
      await load();
    } catch (reason) {
      if (isConflict(reason)) {
        setConflict(true);
        setCorrection(undefined);
      } else if (isForbidden(reason)) {
        setForbidden(true);
        setCorrection(undefined);
      } else {
        setMutationError(reason instanceof Error ? reason.message : "Evidence 纠正失败");
      }
    } finally {
      setBusy("");
    }
  };

  if (loading) {
    return <div className="app-page" aria-label="正在加载事件治理" role="status">
      <Skeleton className="h-28" />
      <div className="mt-6 grid gap-4 lg:grid-cols-2"><Skeleton className="h-72" /><Skeleton className="h-72" /></div>
    </div>;
  }

  if (forbidden) {
    return <div className="app-page">
      <Alert aria-label="权限不足"><ShieldAlert /><AlertTitle>权限不足</AlertTitle>
        <AlertDescription>服务端拒绝了当前账号的事件治理访问或操作；页面不会通过隐藏按钮绕过权限。</AlertDescription>
      </Alert>
      <Button asChild className="mt-4" variant="outline"><Link href="/dashboard/events"><ArrowLeft />返回语义事件</Link></Button>
    </div>;
  }

  if (error || !event) {
    return <div className="app-page">
      <Alert variant="destructive"><FileWarning /><AlertTitle>事件治理加载失败</AlertTitle>
        <AlertDescription className="flex flex-wrap items-center justify-between gap-3"><span>{error || "事件不存在"}</span>
          <Button onClick={() => void load()} size="sm" variant="outline">重试</Button></AlertDescription>
      </Alert>
      <Button asChild className="mt-4" variant="outline"><Link href="/dashboard/events"><ArrowLeft />返回语义事件</Link></Button>
    </div>;
  }

  const members = event.members ?? [];
  const noGovernanceFacts = members.length === 0 && evidence.length === 0 && event.status !== "closed";

  return <div className="app-page">
    <PageHeader
      eyebrow="Event Governance"
      title={eventName(event)}
      description={`事件 #${event.id} · 当前版本 v${event.version}。所有操作都追加 Revision/Audit，不覆盖原始 Observation、Document 或 Evidence。`}
      action={<div className="flex flex-wrap gap-2">
        <Button asChild variant="outline"><Link href="/dashboard/events"><ArrowLeft />返回事件</Link></Button>
        <Button disabled={loading || Boolean(busy)} onClick={() => void load()} variant="outline"><RefreshCw />刷新版本</Button>
      </div>}
    />

    <div className="mt-6 flex flex-wrap gap-2">
      <Badge variant="outline">{eventStatusLabels[event.status ?? ""] ?? event.status}</Badge>
      <Badge variant="secondary">v{event.version}</Badge>
      <Badge variant="outline">{members.length} 个当前成员</Badge>
      <Badge variant="outline">{evidence.length} 条 ClaimEvidence</Badge>
    </div>

    {!canGovern ? <Alert aria-label="只读权限" className="mt-6"><ShieldAlert /><AlertTitle>{analystUnavailable ? "Analyst 角色尚未启用" : "当前账号为只读角色"}</AlertTitle>
      <AlertDescription>{analystUnavailable ? "当前服务端角色契约尚未包含 Analyst，因此按真实角色拒绝治理写入；该迁移将在 005 中统一完成。" : "Viewer 可以核对事件、成员和证据，但治理写操作只允许 Editor/Admin，并由服务端再次校验。"}</AlertDescription>
    </Alert> : null}

    {conflict ? <Alert aria-label="并发冲突" className="mt-6" variant="destructive"><RefreshCw /><AlertTitle>事件版本冲突</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center justify-between gap-3"><span>其他操作者已更新该事件。当前表单未自动覆盖新版本，请刷新后重新核对。</span>
        <Button onClick={() => void load()} size="sm" variant="outline">刷新最新版本</Button></AlertDescription>
    </Alert> : null}

    {mutationError ? <Alert className="mt-6" variant="destructive"><FileWarning /><AlertTitle>治理操作未提交</AlertTitle><AlertDescription>{mutationError}</AlertDescription></Alert> : null}

    {noGovernanceFacts ? <Card className="mt-6 border-dashed"><Empty className="min-h-64 border-0"><EmptyHeader><EmptyMedia variant="icon"><Scale /></EmptyMedia>
      <EmptyTitle>暂无可治理成员或证据</EmptyTitle><EmptyDescription>事件事实已建立，但当前没有有效成员句柄或 ClaimEvidence；页面不会伪造可执行操作。</EmptyDescription>
    </EmptyHeader></Empty></Card> : null}

    <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
      <div className="space-y-6">
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><GitPullRequestArrow className="size-4" />当前成员</CardTitle></CardHeader>
          <CardContent>
            {members.length === 0 ? <p className="text-sm text-muted-foreground">当前没有有效成员；历史成员仍保留在不可变决策与审计中。</p> :
              <Table className="min-w-[720px]" scrollAreaLabel="事件治理成员表"><TableHeader><TableRow><TableHead>内容家族</TableHead><TableHead>成员版本</TableHead><TableHead>原始决策</TableHead><TableHead className="text-right">治理动作</TableHead></TableRow></TableHeader>
                <TableBody>{members.map((member) => <TableRow key={member.id}><TableCell className="font-mono">#{member.content_family_id}</TableCell><TableCell>v{member.version}</TableCell><TableCell className="font-mono">#{member.membership_decision_id}</TableCell>
                  <TableCell><div className="flex flex-wrap justify-end gap-2">
                    <Button disabled={!canGovern || Boolean(busy)} onClick={() => void govern("move_member", member)} size="sm" variant="outline">移动</Button>
                    <Button disabled={!canGovern || Boolean(busy)} onClick={() => void govern("split_event", member)} size="sm" variant="outline">拆分</Button>
                    <Button disabled={!canGovern || Boolean(busy)} onClick={() => void govern("withdraw", member)} size="sm" variant="destructive">移出</Button>
                  </div></TableCell></TableRow>)}</TableBody></Table>}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><Scale className="size-4" />ClaimEvidence</CardTitle></CardHeader>
          <CardContent>
            {evidence.length === 0 ? <p className="text-sm text-muted-foreground">暂无可引用 Evidence。没有证据不等于事实为假。</p> :
              <Table className="min-w-[760px]" scrollAreaLabel="事件证据治理表"><TableHeader><TableRow><TableHead>Claim</TableHead><TableHead>关系</TableHead><TableHead>引用定位器</TableHead><TableHead>可用性</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>{evidence.map((item) => <TableRow key={item.id}><TableCell className="max-w-sm"><p className="font-medium">{item.subject} · {item.predicate}</p><p className="line-clamp-2 text-xs text-muted-foreground">{item.object}</p></TableCell><TableCell>{relationLabels[item.relation ?? ""] ?? item.relation}</TableCell><TableCell className="font-mono">#{item.text_quote_selector_id}</TableCell><TableCell><Badge variant={item.availability === "ready" ? "success" : "outline"}>{item.availability}</Badge></TableCell>
                  <TableCell className="text-right"><Button disabled={!canGovern || Boolean(busy)} onClick={() => { setCorrection(item); setReplacementSelectorID(""); setReplacementRelation(item.relation ?? "mentions"); }} size="sm" variant="outline">纠正 Evidence #{item.id}</Button></TableCell></TableRow>)}</TableBody></Table>}
          </CardContent>
        </Card>
      </div>

      <Card className="h-fit">
        <CardHeader><CardTitle className="flex items-center gap-2"><GitMerge className="size-4" />治理参数</CardTitle></CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-2"><Label htmlFor="governance-reason">理由代码</Label><Input disabled={!canGovern || Boolean(busy)} id="governance-reason" maxLength={64} onChange={(input) => setReasonCode(input.target.value)} value={reasonCode} /></div>
          <div className="space-y-2"><Label htmlFor="governance-note">审计说明</Label><Textarea disabled={!canGovern || Boolean(busy)} id="governance-note" maxLength={1000} onChange={(input) => setNote(input.target.value)} placeholder="说明依据与预期结果" value={note} /></div>
          <div className="space-y-2"><Label>目标事件</Label><Select disabled={!canGovern || Boolean(busy) || targets.length === 0} onValueChange={setTargetID} value={targetID}><SelectTrigger aria-label="选择目标事件"><SelectValue placeholder={targets.length ? "选择移动或合并目标" : "暂无其他活动事件"} /></SelectTrigger><SelectContent>{targets.map((item) => <SelectItem key={item.id} value={String(item.id)}>#{item.id} · {eventName(item)} · v{item.version}</SelectItem>)}</SelectContent></Select></div>
          <div className="grid gap-2">
            {event.status === "closed" ? <Button disabled={!canGovern || Boolean(busy)} onClick={() => void govern("reopen_event")}>恢复事件</Button> :
              <Button disabled={!canGovern || Boolean(busy)} onClick={() => void govern("close_event")} variant="outline">归档事件</Button>}
            <Button disabled={!canGovern || Boolean(busy) || !selectedTarget} onClick={() => void govern("merge_events")} variant="outline">合并到目标事件</Button>
          </div>
          <p className="text-xs leading-5 text-muted-foreground">移动/合并会同时校验源事件、目标事件与成员版本；冲突时不会自动重放到新版本。</p>
        </CardContent>
      </Card>
    </div>

    <Dialog open={Boolean(correction)} onOpenChange={(open) => !open && setCorrection(undefined)}>
      <DialogContent><DialogHeader><DialogTitle>纠正 ClaimEvidence #{correction?.id}</DialogTitle><DialogDescription>系统将追加新 Evidence Version 与反馈审计，原始版本不会被覆盖。</DialogDescription></DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2"><Label htmlFor="replacement-selector">新引用定位器编号</Label><Input id="replacement-selector" inputMode="numeric" onChange={(input) => setReplacementSelectorID(input.target.value)} placeholder={`当前 #${correction?.text_quote_selector_id ?? "—"}`} value={replacementSelectorID} /></div>
          <div className="space-y-2"><Label>新证据关系</Label><Select onValueChange={setReplacementRelation} value={replacementRelation}><SelectTrigger aria-label="选择新证据关系"><SelectValue /></SelectTrigger><SelectContent>{Object.entries(relationLabels).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select></div>
        </div>
        <DialogFooter><Button onClick={() => setCorrection(undefined)} variant="outline">取消</Button><Button disabled={Boolean(busy)} onClick={() => void correctEvidence()}>{busy ? <Loader2 className="animate-spin" /> : null}追加纠正</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </div>;
}
