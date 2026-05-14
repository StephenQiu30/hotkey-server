"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { ArrowUpDown, ExternalLink, Filter } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { api, formatDate, Hotspot, Page, statusTone } from "@/lib/api";

const importanceOptions = [
  { value: "all", label: "全部重要性" },
  { value: "high", label: "High" },
  { value: "medium", label: "Medium" },
  { value: "low", label: "Low" },
];

const sortOptions = [
  { value: "fetched_at_desc", label: "抓取时间" },
  { value: "published_at_desc", label: "发布时间" },
  { value: "relevance_desc", label: "相关性" },
  { value: "importance_desc", label: "重要性" },
];

export function HotspotsClient() {
  const [items, setItems] = useState<Hotspot[]>([]);
  const [importance, setImportance] = useState("all");
  const [sort, setSort] = useState("fetched_at_desc");
  const [loading, setLoading] = useState(true);
  const [filtering, setFiltering] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function load(path = `/api/hotspots?sort=${sort}`) {
    const page = await api<Page<Hotspot>>(path);
    setItems(page.items);
  }

  useEffect(() => {
    load().catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, []);

  async function applyFilters(event: FormEvent) {
    event.preventDefault();
    setFiltering(true);
    setError(null);
    const params = new URLSearchParams({ sort });
    if (importance !== "all") params.set("importance", importance);
    try {
      await load(`/api/hotspots?${params.toString()}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "筛选失败");
    } finally {
      setFiltering(false);
    }
  }

  if (loading) {
    return <Skeleton className="h-96" />;
  }

  return (
    <div className="grid gap-4">
      <Card>
        <CardContent className="pt-5">
          <form className="grid gap-3 md:grid-cols-[220px_220px_auto]" onSubmit={applyFilters}>
            <div className="grid gap-2">
              <Label>重要性</Label>
              <Select onValueChange={setImportance} value={importance}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {importanceOptions.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label>排序</Label>
              <Select onValueChange={setSort} value={sort}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {sortOptions.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end">
              <Button className="w-full md:w-auto" disabled={filtering} type="submit">
                {filtering ? "筛选中" : "应用筛选"}
                <Filter className="h-4 w-4" />
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      {error ? <p className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}
      {items.length === 0 ? <p className="rounded-lg border border-dashed border-border bg-muted/40 p-6 text-sm text-muted-foreground">暂无热点。请先配置关键词和来源，然后在任务页触发检测。</p> : null}
      <div className="grid gap-3">
        {items.map((item) => (
          <Card key={item.id} className="ios-card-muted">
            <CardHeader className="gap-3">
              <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                <div className="min-w-0">
                  <Link
                    className="text-lg font-extrabold leading-7 text-slate-950 transition-colors hover:text-primary ios-focus-ring"
                    href={`/app/hotspots/${item.id}`}
                  >
                    {item.title}
                  </Link>
                  <p className="mt-2 line-clamp-2 leading-7 text-muted-foreground">{item.ai_analysis?.summary || item.snippet || item.url}</p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <Badge variant={statusTone(item.status)}>{item.status}</Badge>
                  <Badge variant={statusTone(item.ai_analysis?.importance || "")}>{item.ai_analysis?.importance || "unknown"}</Badge>
                </div>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-3 text-sm text-muted-foreground md:flex-row md:items-center md:justify-between">
              <div className="flex flex-wrap gap-3">
                <span>{item.source?.name || `source-${item.source_id}`}</span>
                <span>{item.keyword?.keyword || "未关联关键词"}</span>
                <span>{formatDate(item.published_at || item.fetched_at)}</span>
                <span className="inline-flex items-center gap-1">
                  <ArrowUpDown className="h-3.5 w-3.5" />
                  {item.ai_analysis?.relevance_score || "-"}
                </span>
              </div>
              <Button asChild size="sm" variant="secondary">
                <a href={item.url} rel="noreferrer" target="_blank">
                  原文
                  <ExternalLink className="h-4 w-4" />
                </a>
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
