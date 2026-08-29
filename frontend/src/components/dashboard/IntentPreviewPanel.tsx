"use client";

import { memo } from "react";
import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Surface } from "@/components/ui/surface";

type IntentPreviewPanelProps = {
  preview?: HotKeyAPI.IntentPreviewResponseDTO;
  status?: string;
};

export const IntentPreviewPanel = memo(function IntentPreviewPanel({
  preview,
  status,
}: IntentPreviewPanelProps) {
  if (!preview) {
    return (
      <Surface asChild variant="subtle">
        <p className="p-4 text-sm text-muted-foreground">
          {status === "queued" || status === "running"
            ? "正在用历史样本执行多通道召回…"
            : "尚未运行预览。预览结果只解释相关性召回，不表示内容真实或来源可靠。"}
        </p>
      </Surface>
    );
  }

  return (
    <div className="space-y-4">
      <Surface className="flex flex-wrap items-center justify-between gap-3 p-4" variant="ring">
          <div>
            <p className="text-sm font-medium">预计告警量</p>
            <p className="mt-1 text-xs text-muted-foreground">基于当前时间隔离样本，不是线上承诺值。</p>
          </div>
          <Badge variant="secondary">{preview.estimated_alert_count ?? 0}</Badge>
      </Surface>
      {(preview.warnings ?? []).length > 0 ? (
        <div className="flex flex-wrap gap-2" role="status">
          {(preview.warnings ?? []).map((warning) => (
            <Badge key={warning} variant="outline">
              {warning}
            </Badge>
          ))}
        </div>
      ) : null}
      <div className="space-y-3">
        {(preview.samples ?? []).map((sample) => (
          <Surface
            asChild
            key={sample.document_version_id ?? sample.title}
            variant="ring"
          >
          <article className="p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                {sample.document_version_id ? (
                  <Link
                    className="text-sm font-semibold underline-offset-4 hover:underline"
                    href={`/dashboard/document-versions/${sample.document_version_id}`}
                  >
                    {sample.title || "未命名正文版本"}
                  </Link>
                ) : (
                  <p className="text-sm font-semibold">{sample.title || "未命名正文版本"}</p>
                )}
                <p className="mono mt-1 text-xs text-muted-foreground">
                  DocumentVersion #{sample.document_version_id ?? "—"}
                </p>
              </div>
              <Badge variant="outline">{sample.decision || "未决"}</Badge>
            </div>
            <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
              {(sample.recall_signals ?? []).map((signal) => (
                <span className="rounded bg-muted px-2 py-1" key={`${signal.channel}-${signal.rank}`}>
                  {signal.channel || "unknown"} · 排名 {signal.rank ?? "—"} · 原始信号{" "}
                  {typeof signal.raw_score === "number" ? signal.raw_score.toFixed(4) : "—"}
                </span>
              ))}
            </div>
            {(sample.reasons ?? []).length > 0 ? (
              <p className="mt-3 text-xs text-muted-foreground">召回原因：{sample.reasons?.join("；")}</p>
            ) : null}
            {(sample.exclusion_reasons ?? []).length > 0 ? (
              <p className="mt-2 text-xs text-muted-foreground">
                排除原因：{sample.exclusion_reasons?.join("；")}
              </p>
            ) : null}
          </article>
          </Surface>
        ))}
      </div>
    </div>
  );
});
