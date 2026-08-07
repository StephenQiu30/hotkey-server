import Link from "next/link";
import { FileText, Loader2 } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import {
  claimStatusLabel,
  confirmationLabel,
  evidenceStanceLabel,
  formatConfidence,
  formatRadarScore,
} from "@/lib/radarPresentation";

type EventIntelligencePanelProps = {
  event: HotKeyAPI.RadarEventResponse;
  intelligence?: HotKeyAPI.EventIntelligenceResponse;
  intelligenceLoading?: boolean;
  intelligenceError?: boolean;
  monitorSelected?: boolean;
};

function ScoreRow({ label, value }: { label: string; value?: number }) {
  const boundedValue = Math.max(0, Math.min(100, value ?? 0));

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium text-foreground">
          {formatRadarScore(value)}
        </span>
      </div>
      <Progress value={boundedValue} aria-label={`${label} ${formatRadarScore(value)}`} />
    </div>
  );
}

export function EventIntelligencePanel({
  event,
  intelligence,
  intelligenceLoading = false,
  intelligenceError = false,
  monitorSelected = false,
}: EventIntelligencePanelProps) {
  const claims = intelligence?.claims ?? [];
  const relevance = event.watch_relevance ?? event.watch_final_score;

  return (
    <div className="space-y-5 border-t pt-5">
      <section aria-labelledby="confirmation-title">
        <div className="flex items-center justify-between gap-3">
          <h3 id="confirmation-title" className="text-sm font-semibold text-foreground">
            证据确认，不等于真假裁决
          </h3>
          <Badge variant="outline" className="shrink-0 font-normal">
            {confirmationLabel(event.confirmation)}
          </Badge>
        </div>
        <p className="mt-2 text-xs leading-5 text-muted-foreground">
          证据确认强度 {formatRadarScore(event.confirmation_score)}。该分数仅表示当前证据状态，不是事实为真的概率。
        </p>
      </section>

      <section className="space-y-3 border-t pt-5" aria-labelledby="importance-title">
        <div className="flex items-center justify-between">
          <h3 id="importance-title" className="text-sm font-semibold text-foreground">
            重要性
          </h3>
          <span className="text-lg font-semibold tabular-nums text-foreground">
            {formatRadarScore(event.attention)}
          </span>
        </div>
        <ScoreRow label="传播动量" value={event.momentum} />
        <ScoreRow label="来源宽度" value={event.breadth} />
        <ScoreRow label="数据置信度" value={event.data_confidence} />
      </section>

      <section className="border-t pt-5" aria-labelledby="relevance-title">
        <div className="flex items-center justify-between gap-3">
          <h3 id="relevance-title" className="text-sm font-semibold text-foreground">
            监控相关性
          </h3>
          {relevance != null ? (
            <span className="text-sm font-semibold tabular-nums text-foreground">
              {formatRadarScore(relevance)}
            </span>
          ) : null}
        </div>
        <p className="mt-2 text-xs leading-5 text-muted-foreground">
          {!monitorSelected
            ? "选择监控后查看事件与监控规则的相关程度。"
            : relevance == null
              ? "相关性分数等待事件命中该监控后生成。"
              : "相关性只回答事件与所选监控是否匹配，不代表事实真伪。"}
        </p>
      </section>

      <section className="border-t pt-5" aria-labelledby="claims-title">
        <div className="flex items-center justify-between">
          <h3 id="claims-title" className="text-sm font-semibold text-foreground">
            可核查声明与证据
          </h3>
          {intelligenceLoading ? (
            <Loader2 className="h-4 w-4 animate-spin text-primary" aria-label="正在加载事件研判" />
          ) : null}
        </div>

        {intelligenceError ? (
          <Alert variant="destructive" className="mt-3">
            <AlertTitle>事件研判暂时不可用</AlertTitle>
            <AlertDescription>摘要与事件列表仍可继续使用，请稍后重试。</AlertDescription>
          </Alert>
        ) : claims.length ? (
          <ol className="mt-3 space-y-4">
            {claims.map((claim, claimIndex) => (
              <li key={claim.id ?? claim.claim_hash ?? claimIndex} className="rounded-lg border p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="secondary" className="font-normal">
                    {claimStatusLabel(claim.status)}
                  </Badge>
                  <span className="text-xs text-muted-foreground">
                    {formatConfidence(claim.confidence)}
                  </span>
                </div>
                <p className="mt-2 text-sm font-medium leading-6 text-foreground">
                  {claim.normalized_claim || "未命名声明"}
                </p>
                {claim.evidence?.length ? (
                  <ul className="mt-3 space-y-3 border-t pt-3">
                    {claim.evidence.map((evidence, evidenceIndex) => (
                      <li key={`${evidence.content_id ?? "evidence"}-${evidenceIndex}`}>
                        <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                          <span>{evidenceStanceLabel(evidence.stance)}证据</span>
                          {evidence.content_id != null ? (
                            <Link
                              href={`/dashboard/contents/${evidence.content_id}`}
                              className="inline-flex items-center gap-1 font-medium text-foreground underline-offset-4 hover:underline"
                            >
                              <FileText className="h-3.5 w-3.5" />
                              查看证据内容 {evidence.content_id}
                            </Link>
                          ) : null}
                        </div>
                        <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                          {evidence.excerpt || "暂无证据摘录。"}
                        </p>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="mt-2 text-xs text-muted-foreground">该声明暂无可展示证据。</p>
                )}
              </li>
            ))}
          </ol>
        ) : intelligenceLoading ? null : (
          <p className="mt-3 text-sm text-muted-foreground">暂无可核查声明。</p>
        )}
      </section>
    </div>
  );
}
