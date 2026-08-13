import { ArrowUpRight, Flame } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { sourceTypeLabel } from "@/lib/sourceLabels";

export const importanceLabels: Readonly<Record<string, string>> = {
  low: "低",
  medium: "中",
  high: "高",
  urgent: "紧急",
};

export const qualityLabels: Readonly<Record<string, string>> = {
  credible: "可信",
  suspicious: "需复核",
  unavailable: "AI 未分析",
};

function importanceVariant(value: string | undefined) {
  if (value === "urgent") return "destructive" as const;
  if (value === "high") return "warning" as const;
  return "secondary" as const;
}

function qualityVariant(value: string | undefined) {
  if (value === "credible") return "success" as const;
  if (value === "suspicious") return "warning" as const;
  return "outline" as const;
}

export function formatHotspotTime(value: string | undefined) {
  if (!value) return "时间未知";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "时间未知";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(parsed);
}

function availableMetrics(card: HotKeyAPI.HotspotCardResponse) {
  const metrics: Array<[string, number | undefined]> = [
    ["浏览", card.metrics?.view_count],
    ["点赞", card.metrics?.like_count],
    ["评论", card.metrics?.comment_count],
    ["分享", card.metrics?.share_count],
  ];
  return metrics.filter(
    (item): item is [string, number] =>
      typeof item[1] === "number" && Number.isFinite(item[1])
  );
}

export function HotspotCard({ card }: { card: HotKeyAPI.HotspotCardResponse }) {
  return (
    <Card className="overflow-hidden">
      <CardHeader className="gap-3 p-5 pb-3 sm:p-6 sm:pb-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline">
            {card.source_name || sourceTypeLabel(card.source_type)}
          </Badge>
          <Badge variant={importanceVariant(card.importance)}>
            重要性 {importanceLabels[card.importance ?? ""] ?? "未知"}
          </Badge>
          <Badge variant={qualityVariant(card.quality_state)}>
            {qualityLabels[card.quality_state ?? ""] ?? "质量未知"}
          </Badge>
          <span className="mono inline-flex items-center gap-1 text-xs font-medium">
            <Flame className="h-3.5 w-3.5" />
            热度 {card.heat_score ?? 0}
          </span>
          <span className="text-xs text-muted-foreground">
            相关性 {card.relevance ?? 0}%
          </span>
        </div>
        <CardTitle className="text-xl leading-7">
          <h2 className="text-balance">{card.title || "无标题"}</h2>
        </CardTitle>
        <CardDescription className="leading-6">
          {card.summary || "来源未提供摘要。"}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 p-5 pt-0 sm:p-6 sm:pt-0">
        <div className="flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
          {card.author ? <span>作者 {card.author}</span> : null}
          <span>发布 {formatHotspotTime(card.published_at)}</span>
          <span>发现 {formatHotspotTime(card.discovered_at)}</span>
          {availableMetrics(card).map(([label, value]) => (
            <span key={label}>
              {label} {value.toLocaleString("zh-CN")}
            </span>
          ))}
        </div>
        <div className="flex flex-col gap-3 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
          <p className="max-w-3xl text-xs leading-5 text-muted-foreground">
            {card.relevance_reason || "未提供判断理由"}
          </p>
          {card.canonical_url ? (
            <Button asChild size="sm" variant="outline">
              <a href={card.canonical_url} rel="noreferrer" target="_blank">
                查看原文
                <ArrowUpRight className="h-4 w-4" />
              </a>
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
