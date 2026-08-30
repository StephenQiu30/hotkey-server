import { ArrowUpRight, Eye, Flame, MessageCircle, Share2, ThumbsUp } from "lucide-react";
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

type Importance = NonNullable<HotKeyAPI.HotspotCardResponse["importance"]>;
type QualityState = NonNullable<HotKeyAPI.HotspotCardResponse["quality_state"]>;

export const importanceLabels: Readonly<Record<Importance, string>> = {
  low: "低",
  medium: "中",
  high: "高",
  urgent: "紧急",
};

export const qualityLabels: Readonly<Record<QualityState, string>> = {
  credible: "证据已覆盖",
  suspicious: "证据待补充",
  unavailable: "等待分析",
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
  const metrics: Array<[string, number | undefined, typeof Eye]> = [
    ["浏览", card.metrics?.view_count, Eye],
    ["点赞", card.metrics?.like_count, ThumbsUp],
    ["评论", card.metrics?.comment_count, MessageCircle],
    ["分享", card.metrics?.share_count, Share2],
  ];
  return metrics.filter(
    (item): item is [string, number, typeof Eye] =>
      typeof item[1] === "number" && Number.isFinite(item[1])
  );
}

function importanceAccent(value: string | undefined) {
  if (value === "urgent") return "bg-foreground/75";
  if (value === "high") return "bg-foreground/50";
  return "bg-muted-foreground/30";
}

export function HotspotCard({
  card,
  headingLevel = "h2",
}: {
  card: HotKeyAPI.HotspotCardResponse;
  headingLevel?: "h2" | "h3";
}) {
  const Heading = headingLevel;
  const heat =
    typeof card.heat_score === "number"
      ? Math.max(0, Math.min(100, card.heat_score))
      : undefined;
  const relevance =
    typeof card.relevance === "number"
      ? Math.max(0, Math.min(100, card.relevance))
      : undefined;
  const heatText = heat == null ? "—" : heat.toFixed(1);
  const relevanceText = relevance == null ? "—" : `${relevance.toFixed(0)}%`;

  return (
    <Card className="hotspot-card relative overflow-hidden bg-card/90" data-slot="hotspot-card">
      <div aria-hidden="true" className={`absolute inset-y-0 left-0 w-1 ${importanceAccent(card.importance)}`} />
      <CardHeader className="gap-4 p-5 pb-3 pl-6 sm:p-6 sm:pb-3 sm:pl-7">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline" className="bg-secondary/55">
                {card.source_name || sourceTypeLabel(card.source_type)}
              </Badge>
              <Badge variant={importanceVariant(card.importance)}>
                {card.importance ? importanceLabels[card.importance] : "未知"}优先级
              </Badge>
              <Badge variant={qualityVariant(card.quality_state)}>
                {card.quality_state
                  ? qualityLabels[card.quality_state]
                  : "证据状态未知"}
              </Badge>
            </div>
            <CardTitle className="mt-4 text-xl leading-7 sm:text-2xl">
              <Heading className="text-balance">{card.title || "无标题"}</Heading>
            </CardTitle>
            <CardDescription className="mt-3 max-w-3xl leading-6">
              {card.summary || "来源未提供摘要。"}
            </CardDescription>
          </div>
          <div className="grid shrink-0 grid-cols-2 gap-2 sm:w-[174px]">
            <div
              aria-label={heat == null ? "热度待分析" : `热度 ${heatText}`}
              className="rounded-xl bg-muted p-3 text-foreground"
            >
              <p className="flex items-center gap-1 text-[10px] font-semibold uppercase"><Flame className="h-3 w-3" /> Heat</p>
              <p className="mono mt-2 text-2xl font-semibold leading-none">{heatText}</p>
            </div>
            <div
              aria-label={
                relevance == null ? "相关性待分析" : `相关性 ${relevanceText}`
              }
              className="rounded-xl bg-muted p-3 text-foreground"
            >
              <p className="text-[10px] font-semibold uppercase">相关性</p>
              <p className="mono mt-2 text-2xl font-semibold leading-none">
                {relevance == null ? (
                  "—"
                ) : (
                  <>{relevance.toFixed(0)}<span className="text-xs">%</span></>
                )}
              </p>
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 p-5 pt-1 pl-6 sm:p-6 sm:pt-1 sm:pl-7">
        <div className="flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
          {card.author ? <span>作者 {card.author}</span> : null}
          <span>发布 {formatHotspotTime(card.published_at)}</span>
          <span>发现 {formatHotspotTime(card.discovered_at)}</span>
          {availableMetrics(card).map(([label, value, Icon]) => (
            <span className="inline-flex items-center gap-1" key={label}>
              <Icon aria-hidden="true" className="h-3 w-3" />
              <span className="sr-only">{label}</span>
              {value.toLocaleString("zh-CN")}
            </span>
          ))}
        </div>
        <div className="flex flex-col gap-4 pt-3 sm:flex-row sm:items-end sm:justify-between">
          <div className="max-w-3xl">
            <p className="text-[10px] font-semibold uppercase text-muted-foreground">为什么进入观察</p>
            <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
              {card.relevance_reason || "当前信号命中已发布监控目标，等待更多来源与证据补充。"}
            </p>
          </div>
          {card.canonical_url ? (
            <Button asChild size="sm" variant="outline">
              <a href={card.canonical_url} rel="noreferrer" target="_blank">
                查看原始信号
                <ArrowUpRight className="h-4 w-4" />
              </a>
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
