"use client";

import Link from "next/link";
import { BookOpen, ExternalLink, FileClock, GitBranch, PencilLine, Split } from "lucide-react";
import { SafeExternalLink } from "@/components/content/SafeExternalLink";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";

const relationLabels: Record<string, string> = {
  asserts: "直接陈述",
  attributes_to: "归因转述",
  mentions: "提及",
  contradicts: "相反表述",
  corrects: "更正",
  withdraws: "撤回",
  unknown: "关系未定",
};

const availabilityLabels: Record<string, string> = {
  ready: "引用可读",
  rights_unavailable: "引用权利已失效",
  retention_unavailable: "引用已停止保留",
  selector_unavailable: "引用位置不可用",
  document_unavailable: "正文版本不可用",
};

function formatDateTime(value?: string) {
  if (!value) return "时间未提供";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "时间未提供";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

export function MicroEventEvidenceCard({
  evidence,
  canReview,
  onCorrect,
  onReviewLineage,
}: {
  evidence: HotKeyAPI.ClaimEvidenceResponseDTO;
  canReview: boolean;
  onCorrect: (evidence: HotKeyAPI.ClaimEvidenceResponseDTO) => void;
  onReviewLineage: (evidence: HotKeyAPI.ClaimEvidenceResponseDTO) => void;
}) {
  const documentVersionID = evidence.document_version_id;
  const quoteReady = evidence.availability === "ready" && Boolean(evidence.exact_quote);

  return (
    <Card className="overflow-hidden [content-visibility:auto]">
      <CardHeader className="gap-3 border-b border-border/70 bg-muted/20">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{relationLabels[evidence.relation ?? ""] ?? "关系未定"}</Badge>
            <Badge variant={quoteReady ? "secondary" : "outline"}>
              {availabilityLabels[evidence.availability ?? ""] ?? "引用状态未知"}
            </Badge>
          </div>
          <span className="text-xs text-muted-foreground">{formatDateTime(evidence.published_at ?? evidence.captured_at)}</span>
        </div>
        <p className="text-sm font-medium leading-6">
          {evidence.subject || "未命名主体"} · {evidence.predicate || "未命名动作"} · {evidence.object || "未提供对象"}
        </p>
      </CardHeader>
      <CardContent className="space-y-4 pt-5">
        {quoteReady ? (
          <blockquote className="rounded-md border-l-4 border-primary bg-primary/5 px-4 py-3 text-sm leading-6">
            {evidence.exact_quote}
          </blockquote>
        ) : (
          <p className="rounded-md border border-dashed px-4 py-3 text-sm text-muted-foreground">
            当前只展示引用状态，不展示已撤权、过期或无法重新定位的摘录及哈希。
          </p>
        )}

        <dl className="grid gap-3 text-xs text-muted-foreground sm:grid-cols-2">
          <div>
            <dt className="font-medium text-foreground">出处主体</dt>
            <dd className="mt-1">{evidence.publisher || evidence.content_origin || "信息未提供"}</dd>
          </div>
          <div>
            <dt className="font-medium text-foreground">内容家族 / 独立起源</dt>
            <dd className="mt-1 inline-flex items-center gap-1">
              <GitBranch className="size-3" /> 家族 #{evidence.content_family_id ?? "—"} · 根版本 #{evidence.lineage_root_document_version_id ?? "—"}
            </dd>
          </div>
          <div>
            <dt className="font-medium text-foreground">引用版本</dt>
            <dd className="mt-1">DocumentVersion #{documentVersionID ?? "—"} · selector #{evidence.text_quote_selector_id ?? "—"}</dd>
          </div>
          <div>
            <dt className="font-medium text-foreground">决策来源</dt>
            <dd className="mt-1">{evidence.decision_origin === "manual" ? "人工复核" : "已审计模型运行"}</dd>
          </div>
        </dl>

        <div className="flex flex-wrap gap-2">
          {documentVersionID ? (
            <Button asChild size="sm" variant="outline">
              <Link href={`/dashboard/document-versions/${documentVersionID}?evidence=${evidence.id ?? ""}${evidence.markdown_anchor ? `#${encodeURIComponent(evidence.markdown_anchor)}` : ""}`}>
                <BookOpen /> 阅读归档
              </Link>
            </Button>
          ) : null}
          {evidence.canonical_url ? (
            <Button asChild size="sm" variant="outline">
              <SafeExternalLink href={evidence.canonical_url}><ExternalLink /> 访问原文</SafeExternalLink>
            </Button>
          ) : null}
          {evidence.source_record_url ? (
            <Button asChild size="sm" variant="ghost">
              <SafeExternalLink href={evidence.source_record_url}><FileClock /> 来源记录 / 讨论</SafeExternalLink>
            </Button>
          ) : null}
          {canReview && evidence.id && evidence.version ? (
            <Button onClick={() => onCorrect(evidence)} size="sm" type="button" variant="ghost">
              <PencilLine /> 纠正关系或摘录
            </Button>
          ) : null}
          {canReview && evidence.lineage_decision_id && evidence.content_family_member_version ? (
            <Button onClick={() => onReviewLineage(evidence)} size="sm" type="button" variant="ghost">
              <Split /> 复核正文谱系
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
