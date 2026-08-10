"use client";

import Link from "next/link";
import { memo, useCallback, useEffect, useState } from "react";
import { Check, ChevronDown, Loader2, RefreshCw, SearchCheck, X } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { HotKeyAPIError } from "@/lib/request";
import {
  getMonitorsIdDocumentMatches,
  postMonitorsIdDocumentMatchesMatchDecisionIdOverrides,
} from "@/services/hotkey/hotkey-server/documentMatches";

const decisionLabels: Record<string, string> = {
  accepted: "已标记相关",
  review: "等待人工判断",
  rejected: "已标记不相关",
};

const automaticDecisionLabels: Record<string, string> = {
  accepted: "自动判定相关",
  review: "自动保留复核",
  rejected: "自动判定不相关",
};

const decisionStyles: Record<string, "default" | "secondary" | "outline"> = {
  accepted: "default",
  review: "secondary",
  rejected: "outline",
};

let reviewActionSequence = 0;

function documentMatchReviewKey(monitorID: number, matchDecisionID: number) {
  reviewActionSequence += 1;
  return `document-match-review-${monitorID}-${matchDecisionID}-${Date.now().toString(36)}-${reviewActionSequence.toString(36)}`;
}

function formatDateTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

type ReviewTarget = {
  decision: "accepted" | "rejected";
  match: HotKeyAPI.DocumentMatchResponseDTO;
};

type DocumentMatchReviewDialogProps = {
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (note: string) => void;
  target?: ReviewTarget;
};

const DocumentMatchReviewDialog = memo(function DocumentMatchReviewDialog({
  busy,
  onOpenChange,
  onSubmit,
  target,
}: DocumentMatchReviewDialogProps) {
  const [note, setNote] = useState("");
  const accepted = target?.decision === "accepted";

  useEffect(() => {
    setNote("");
  }, [target?.match.match_decision_id, target?.decision]);

  return (
    <Dialog open={target != null} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{accepted ? "标记为相关" : "标记为不相关"}</DialogTitle>
          <DialogDescription>
            该操作只覆盖这条内容与当前监控版本的相关性判定，不代表事实真伪或来源可靠性。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2 py-2">
          <Label htmlFor="document-match-review-note">复核说明（可选）</Label>
          <textarea
            className="min-h-28 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-50"
            disabled={busy}
            id="document-match-review-note"
            maxLength={8000}
            onChange={(event) => setNote(event.target.value)}
            placeholder="记录判断依据，避免写入敏感正文。"
            value={note}
          />
        </div>
        <DialogFooter>
          <Button disabled={busy} onClick={() => onOpenChange(false)} type="button" variant="outline">
            取消
          </Button>
          <Button
            disabled={busy}
            onClick={() => onSubmit(note.trim())}
            type="button"
            variant={accepted ? "default" : "destructive"}
          >
            {busy ? <Loader2 className="animate-spin" /> : accepted ? <Check /> : <X />}
            {accepted ? "确认标记相关" : "确认标记不相关"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
});

type DocumentMatchCardProps = {
  canReview: boolean;
  match: HotKeyAPI.DocumentMatchResponseDTO;
  onReview: (match: HotKeyAPI.DocumentMatchResponseDTO, decision: "accepted" | "rejected") => void;
};

const DocumentMatchCard = memo(function DocumentMatchCard({
  canReview,
  match,
  onReview,
}: DocumentMatchCardProps) {
  const effectiveDecision = match.effective_decision ?? "review";
  const automaticDecision = match.automatic_decision ?? "review";
  const probability = match.relevance_probability;

  return (
    <article className="rounded-xl border border-border p-4 sm:p-5 [content-visibility:auto]">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          {match.document_version_id ? (
            <Link
              className="font-semibold underline-offset-4 hover:underline"
              href={`/dashboard/document-versions/${match.document_version_id}`}
            >
              正文版本 #{match.document_version_id}
            </Link>
          ) : (
            <p className="font-semibold">正文版本不可用</p>
          )}
          <p className="mono mt-1 text-xs text-muted-foreground">
            MonitorVersion #{match.monitor_version_id ?? "—"} · CompiledProfile #{match.compiled_profile_id ?? "—"}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={decisionStyles[effectiveDecision] ?? "outline"}>
            {decisionLabels[effectiveDecision] ?? "未知判定"}
          </Badge>
          {match.degraded ? <Badge variant="outline">降级召回</Badge> : null}
        </div>
      </div>

      <div className="mt-4 grid gap-3 rounded-lg bg-muted/50 p-4 text-sm sm:grid-cols-3">
        <div>
          <p className="text-xs text-muted-foreground">相关概率</p>
          <p className="mt-1 font-medium">
            {typeof probability === "number"
              ? `${(probability * 100).toFixed(1)}%（仅表示相关）`
              : "相关概率尚未校准"}
          </p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">融合排名分</p>
          <p className="mono mt-1 font-medium">
            {typeof match.rrf_score === "number" ? match.rrf_score.toFixed(6) : "—"}
          </p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">自动决策</p>
          <p className="mt-1 font-medium">
            {automaticDecisionLabels[automaticDecision] ?? "未知自动决策"}
          </p>
        </div>
      </div>

      {(match.signals ?? []).length > 0 ? (
        <div className="mt-4 flex flex-wrap gap-2 text-xs text-muted-foreground">
          {(match.signals ?? []).map((signal) => (
            <span
              className="rounded-md border border-border px-2 py-1"
              key={`${signal.channel}-${signal.algorithm_version}-${signal.rank}`}
            >
              {signal.channel ?? "unknown"} · 排名 {signal.rank ?? "—"} · 原始信号{" "}
              {typeof signal.raw_score === "number" ? signal.raw_score.toFixed(4) : "—"}
            </span>
          ))}
        </div>
      ) : null}
      {(match.reason_codes ?? []).length > 0 ? (
        <p className="mt-3 break-words text-xs text-muted-foreground">
          判定原因：{match.reason_codes?.join("；")}
        </p>
      ) : null}
      <p className="mt-3 text-xs text-muted-foreground">
        判定于 {formatDateTime(match.decided_at)} · 匹配算法 {match.matching_algorithm_version ?? "—"} ·
        资源版本 v{match.resource_version ?? 0}
      </p>

      {canReview && match.match_decision_id ? (
        <div className="mt-4 flex flex-col gap-2 border-t border-border pt-4 sm:flex-row sm:justify-end">
          <Button
            onClick={() => onReview(match, "rejected")}
            type="button"
            variant="outline"
          >
            <X />
            标记不相关
          </Button>
          <Button onClick={() => onReview(match, "accepted")} type="button">
            <Check />
            标记相关
          </Button>
        </div>
      ) : null}
    </article>
  );
});

type DocumentMatchWorkspaceProps = {
  canReview: boolean;
  monitorID: number;
};

export function DocumentMatchWorkspace({ canReview, monitorID }: DocumentMatchWorkspaceProps) {
  const [decision, setDecision] = useState("review");
  const [items, setItems] = useState<HotKeyAPI.DocumentMatchResponseDTO[]>([]);
  const [nextCursor, setNextCursor] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadFailure, setLoadFailure] = useState<string>();
  const [reviewTarget, setReviewTarget] = useState<ReviewTarget>();
  const [reviewing, setReviewing] = useState(false);

  const load = useCallback(
    async (cursor?: string) => {
      if (cursor) setLoadingMore(true);
      else setLoading(true);
      setLoadFailure(undefined);
      try {
        const result = await getMonitorsIdDocumentMatches({
          id: monitorID,
          decision,
          ...(cursor ? { cursor } : {}),
          limit: 50,
        });
        if (!result.data) throw new Error("相关性判定响应为空");
        setItems((current) => (cursor ? [...current, ...(result.data?.items ?? [])] : result.data?.items ?? []));
        setNextCursor(result.data.next_cursor ?? "");
      } catch (reason) {
        setLoadFailure(reason instanceof Error ? reason.message : "相关性判定加载失败");
      } finally {
        setLoading(false);
        setLoadingMore(false);
      }
    },
    [decision, monitorID],
  );

  useEffect(() => {
    void load();
  }, [load]);

  const openReview = useCallback(
    (match: HotKeyAPI.DocumentMatchResponseDTO, nextDecision: "accepted" | "rejected") => {
      setReviewTarget({ decision: nextDecision, match });
    },
    [],
  );

  const submitReview = useCallback(
    async (note: string) => {
      const target = reviewTarget;
      const matchDecisionID = target?.match.match_decision_id;
      if (!target || !matchDecisionID) return;
      const expectedVersion = target.match.resource_version ?? 0;
      setReviewing(true);
      try {
        const result = await postMonitorsIdDocumentMatchesMatchDecisionIdOverrides(
          { id: monitorID, match_decision_id: matchDecisionID },
          {
            decision: target.decision,
            reason_code: target.decision === "accepted" ? "manual_relevant" : "manual_irrelevant",
            note,
          },
          {
            headers: {
              "If-Match": `"v${expectedVersion}"`,
              "Idempotency-Key": documentMatchReviewKey(monitorID, matchDecisionID),
            },
          },
        );
        const receipt = result.data;
        if (
          !receipt ||
          receipt.monitor_id !== monitorID ||
          receipt.match_decision_id !== matchDecisionID ||
          receipt.decision !== target.decision ||
          receipt.resource_version !== expectedVersion + 1
        ) {
          throw new Error("相关性复核响应与请求不一致");
        }
        setItems((current) =>
          current.map((item) =>
            item.match_decision_id === matchDecisionID
              ? { ...item, effective_decision: target.decision, resource_version: receipt.resource_version }
              : item,
          ),
        );
        setReviewTarget(undefined);
        toast.success(target.decision === "accepted" ? "已标记相关" : "已标记不相关");
      } catch (reason) {
        toast.error(reason instanceof Error ? reason.message : "相关性复核失败");
        if (reason instanceof HotKeyAPIError && reason.status === 409) {
          setReviewTarget(undefined);
          await load();
        }
      } finally {
        setReviewing(false);
      }
    },
    [load, monitorID, reviewTarget],
  );

  return (
    <div className="space-y-5">
      <Card className="gap-4 p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <div className="flex items-center gap-2">
              <SearchCheck className="text-muted-foreground" />
              <h2 className="text-base font-semibold">精确版本相关性判定</h2>
            </div>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              每条记录绑定监控版本、正文版本、召回算法和校准配置。分数只回答是否与监控目标相关。
            </p>
          </div>
          <div className="min-w-44 space-y-2">
            <Label htmlFor="document-match-decision-filter">判定筛选</Label>
            <div className="relative">
              <select
                className="h-9 w-full appearance-none rounded-lg bg-muted px-3 pr-9 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
                id="document-match-decision-filter"
                onChange={(event) => setDecision(event.target.value)}
                value={decision}
              >
                <option value="review">等待人工判断</option>
                <option value="accepted">已标记相关</option>
                <option value="rejected">已标记不相关</option>
              </select>
              <ChevronDown className="pointer-events-none absolute right-3 top-2.5 h-4 w-4 text-muted-foreground" />
            </div>
          </div>
        </div>
      </Card>

      {loading ? (
        <div className="flex min-h-64 items-center justify-center" aria-label="加载相关性判定">
          <Loader2 className="animate-spin text-muted-foreground" />
        </div>
      ) : loadFailure ? (
        <Card className="items-center p-8 text-center" role="alert">
          <p className="font-medium">相关性判定加载失败</p>
          <p className="text-sm text-muted-foreground">{loadFailure}</p>
          <Button onClick={() => void load()} type="button" variant="outline">
            <RefreshCw />
            重试
          </Button>
        </Card>
      ) : items.length === 0 ? (
        <Card className="items-center p-10 text-center">
          <SearchCheck className="text-muted-foreground" />
          <p className="font-medium">当前筛选下暂无判定</p>
          <p className="text-sm text-muted-foreground">已发布监控产生匹配事实后会显示在这里。</p>
        </Card>
      ) : (
        <div className="space-y-4">
          {items.map((match) => (
            <DocumentMatchCard
              canReview={canReview}
              key={match.match_decision_id}
              match={match}
              onReview={openReview}
            />
          ))}
          {nextCursor ? (
            <div className="flex justify-center">
              <Button
                disabled={loadingMore}
                onClick={() => void load(nextCursor)}
                type="button"
                variant="outline"
              >
                {loadingMore ? <Loader2 className="animate-spin" /> : null}
                加载更多
              </Button>
            </div>
          ) : null}
        </div>
      )}

      <DocumentMatchReviewDialog
        busy={reviewing}
        onOpenChange={(open) => !open && setReviewTarget(undefined)}
        onSubmit={(note) => void submitReview(note)}
        target={reviewTarget}
      />
    </div>
  );
}
