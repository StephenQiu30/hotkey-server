"use client";

import { useEffect, useState } from "react";
import { ExternalLink } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { api, formatDate, Hotspot, statusTone } from "@/lib/api";

export function HotspotDetailClient({ id }: { id: string }) {
  const [item, setItem] = useState<Hotspot | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<Hotspot>(`/api/hotspots/${id}`).then(setItem).catch((err) => setError(err.message));
  }, [id]);

  if (error) return <p className="ios-card-muted border-destructive/35 bg-destructive/10 border p-3 text-sm text-destructive" role="alert">{error}</p>;
  if (!item) return <Skeleton className="h-96 ios-shell-card" />;

  const statusMap: Record<string, string> = {
    active: "活动",
    filtered: "已过滤",
  };

  return (
    <div className="grid gap-5 xl:grid-cols-[1.2fr_.8fr]">
      <Card className="ios-shell-card">
        <CardHeader>
          <CardTitle className="text-2xl leading-tight">{item.title}</CardTitle>
          <CardDescription className="mt-2">{item.ai_analysis?.summary || item.snippet || "暂无摘要"}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-5">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={statusTone(item.status)}>{statusMap[item.status] || item.status}</Badge>
            <Badge variant={statusTone(item.ai_analysis?.importance || "default")}>{item.ai_analysis?.importance || "unknown"}</Badge>
            <span className="text-xs text-muted-foreground">热度 {item.ai_analysis?.relevance_score || "-"}</span>
          </div>
          <Separator />
          <div className="grid gap-3 md:grid-cols-2">
            <div className="grid gap-2 ios-card-muted p-3">
              <p className="text-xs text-muted-foreground">来源</p>
              <p className="font-semibold">{item.source?.name || item.source_id}</p>
            </div>
            <div className="grid gap-2 ios-card-muted p-3">
              <p className="text-xs text-muted-foreground">关键词</p>
              <p className="font-semibold">{item.keyword?.keyword || "-"}</p>
            </div>
            <div className="grid gap-2 ios-card-muted p-3">
              <p className="text-xs text-muted-foreground">发布时间</p>
              <p className="font-semibold">{formatDate(item.published_at || item.fetched_at)}</p>
            </div>
            <div className="grid gap-2 ios-card-muted p-3">
              <p className="text-xs text-muted-foreground">作者</p>
              <p className="font-semibold">{item.author || "-"}</p>
            </div>
          </div>
          <div className="ios-card-muted grid gap-2 p-3">
            <p className="text-xs text-muted-foreground">原始链接</p>
            <a className="text-primary underline-offset-4 hover:underline" href={item.url} rel="noreferrer" target="_blank">
              {item.url}
            </a>
          </div>
          <Button asChild className="w-fit" variant="secondary">
            <a href={item.url} rel="noreferrer" target="_blank">
              打开原文
              <ExternalLink className="h-4 w-4" />
            </a>
          </Button>
        </CardContent>
      </Card>

      <Card className="ios-shell-card">
        <CardHeader>
          <CardTitle>AI 分析</CardTitle>
          <CardDescription>真实性、相关性和报告入选依据。</CardDescription>
        </CardHeader>
        <CardContent>
          {item.ai_analysis ? (
            <dl className="grid gap-4">
              <div className="flex items-center justify-between gap-3">
                <dt className="font-semibold text-muted-foreground">真实性</dt>
                <dd><Badge variant={item.ai_analysis.is_real === false ? "destructive" : "success"}>{String(item.ai_analysis.is_real)}</Badge></dd>
              </div>
              <div className="flex items-center justify-between gap-3">
                <dt className="font-semibold text-muted-foreground">相关性</dt>
                <dd className="font-bold">{item.ai_analysis.relevance_score}</dd>
              </div>
              <div className="flex items-center justify-between gap-3">
                <dt className="font-semibold text-muted-foreground">重要性</dt>
                <dd><Badge variant={statusTone(item.ai_analysis.importance)}>{item.ai_analysis.importance}</Badge></dd>
              </div>
              <div className="grid gap-2">
                <dt className="font-semibold text-muted-foreground">理由</dt>
                <dd className="rounded-lg bg-muted p-3 text-sm leading-7">{item.ai_analysis.relevance_reason || "-"}</dd>
              </div>
            </dl>
          ) : (
            <p className="rounded-lg border border-dashed border-border bg-muted/40 p-4 text-sm text-muted-foreground">暂无 AI 分析。</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
