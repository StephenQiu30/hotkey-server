"use client";

import { memo } from "react";
import { Check, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Surface } from "@/components/ui/surface";

type ExpansionCandidateReviewProps = {
  busyCandidateID?: string;
  canAdmin: boolean;
  candidates: HotKeyAPI.IntentExpansionCandidateResponseDTO[];
  onReview: (candidate: HotKeyAPI.IntentExpansionCandidateResponseDTO, decision: "approved" | "rejected") => void;
};

const statusLabel = (status?: string) => {
  if (status === "approved") return "已批准";
  if (status === "rejected") return "已拒绝";
  return "待审批";
};

export const ExpansionCandidateReview = memo(function ExpansionCandidateReview({
  busyCandidateID,
  canAdmin,
  candidates,
  onReview,
}: ExpansionCandidateReviewProps) {
  if (candidates.length === 0) {
    return (
      <Surface asChild variant="subtle">
        <p className="p-4 text-sm text-muted-foreground">
          暂无扩展候选。模型生成的内容必须保留出处并经管理员审批后才参与匹配。
        </p>
      </Surface>
    );
  }

  return (
    <div className="space-y-3">
      {candidates.map((candidate) => {
        const pending = candidate.approval_status === "pending";
        const value = candidate.value ?? "未命名候选";
        return (
          <Surface asChild key={candidate.id ?? value} variant="ring">
          <article className="p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="break-words text-sm font-semibold">{value}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {candidate.reason || "未提供生成理由"}
                </p>
              </div>
              <Badge variant={pending ? "secondary" : "outline"}>
                {statusLabel(candidate.approval_status)}
              </Badge>
            </div>
            <dl className="mt-3 grid gap-x-5 gap-y-2 text-xs text-muted-foreground sm:grid-cols-2">
              <div>
                <dt className="inline font-medium text-foreground">建议来源：</dt>
                <dd className="inline">{candidate.source || "未提供"}</dd>
              </div>
              <div>
                <dt className="inline font-medium text-foreground">语义接近度：</dt>
                <dd className="inline">
                  {typeof candidate.similarity === "number" ? candidate.similarity.toFixed(3) : "未提供"}
                </dd>
              </div>
              <div>
                <dt className="inline font-medium text-foreground">模型版本：</dt>
                <dd className="mono inline break-all">{candidate.model_version || "未提供"}</dd>
              </div>
              <div>
                <dt className="inline font-medium text-foreground">提示版本：</dt>
                <dd className="mono inline break-all">{candidate.prompt_version || "未提供"}</dd>
              </div>
              <div>
                <dt className="inline font-medium text-foreground">风险：</dt>
                <dd className="inline">{candidate.risk || "未提供"}</dd>
              </div>
              <div>
                <dt className="inline font-medium text-foreground">输入摘要：</dt>
                <dd className="mono inline break-all">{candidate.input_hash || "未提供"}</dd>
              </div>
            </dl>
            {canAdmin && pending && candidate.id ? (
              <div className="mt-4 flex flex-wrap gap-2">
                <Button
                  aria-label={`批准候选 ${value}`}
                  disabled={busyCandidateID === candidate.id}
                  onClick={() => onReview(candidate, "approved")}
                  size="sm"
                  type="button"
                >
                  <Check />
                  批准
                </Button>
                <Button
                  aria-label={`拒绝候选 ${value}`}
                  disabled={busyCandidateID === candidate.id}
                  onClick={() => onReview(candidate, "rejected")}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  <X />
                  拒绝
                </Button>
              </div>
            ) : null}
          </article>
          </Surface>
        );
      })}
    </div>
  );
});
