"use client";

import { FormEvent, useState } from "react";
import { ExternalLink, Search } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { api, formatDate, SearchResponse, statusTone } from "@/lib/api";

const sourceOptions = [
  { value: "all", label: "全部来源" },
  { value: "rss", label: "RSS" },
  { value: "hacker_news", label: "Hacker News" },
  { value: "x_twitter", label: "X/Twitter" },
  { value: "bing", label: "Bing" },
  { value: "bilibili", label: "Bilibili" },
  { value: "sogou", label: "Sogou-style" },
];

export function SearchClient() {
  const [query, setQuery] = useState("");
  const [sourceType, setSourceType] = useState("all");
  const [limit, setLimit] = useState(20);
  const [result, setResult] = useState<SearchResponse | null>(null);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submitSearch(event: FormEvent) {
    event.preventDefault();
    setSearching(true);
    setError(null);
    setResult(null);
    try {
      const payload = {
        query,
        limit,
        source_types: sourceType === "all" ? null : [sourceType],
      };
      setResult(await api<SearchResponse>("/api/search", { method: "POST", body: JSON.stringify(payload) }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "搜索失败");
    } finally {
      setSearching(false);
    }
  }

  return (
    <div className="grid gap-5">
      <Card>
        <CardHeader>
          <CardTitle>搜索主题</CardTitle>
          <CardDescription>用于快速判断一个主题是否值得纳入关键词监控。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_200px_140px_max-content]" onSubmit={submitSearch}>
            <div className="grid gap-2">
              <Label htmlFor="search-query">查询</Label>
              <Input id="search-query" onChange={(event) => setQuery(event.target.value)} placeholder="例如 AI agent workflow" required value={query} />
            </div>
            <div className="grid gap-2">
              <Label>来源</Label>
              <Select onValueChange={setSourceType} value={sourceType}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {sourceOptions.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="search-limit">数量</Label>
              <Input id="search-limit" max={100} min={1} onChange={(event) => setLimit(Number(event.target.value))} type="number" value={limit} />
            </div>
            <div className="flex items-end">
              <Button className="w-full xl:w-auto" disabled={searching} type="submit">
                {searching ? "搜索中" : "搜索"}
                <Search className="h-4 w-4" />
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {error ? <p className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}

      {result ? (
        <div className="grid gap-4">
          {result.errors.length > 0 ? (
            <div className="ios-card-muted rounded-lg border border-amber-200 bg-amber-50 p-4" role="alert">
              <p className="font-semibold text-amber-900">部分来源返回错误</p>
              <ul className="mt-2 grid gap-1 text-sm text-amber-800">
                {result.errors.map((item) => <li key={item}>{item}</li>)}
              </ul>
            </div>
          ) : null}
          {result.items.length === 0 ? <p className="rounded-lg border border-dashed border-border bg-muted/40 p-6 text-sm text-muted-foreground">没有搜索结果。</p> : null}
          {result.items.map((item) => (
            <Card className="ios-card-muted" key={`${item.source_id}-${item.url}`}>
              <CardHeader>
                <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                  <div className="min-w-0">
                    <CardTitle className="text-lg leading-7">{item.title}</CardTitle>
                    <CardDescription className="mt-2 line-clamp-2">{item.summary || item.snippet || item.url}</CardDescription>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2">
                    <Badge variant={statusTone(item.status)}>{item.status}</Badge>
                    <Badge variant={statusTone(item.importance)}>{item.importance}</Badge>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div className="flex flex-wrap gap-3 text-sm text-muted-foreground">
                  <span>{item.source_name}</span>
                  <span>{formatDate(item.published_at)}</span>
                  <span>相关性 {item.relevance_score}</span>
                </div>
                <Button asChild size="sm" variant="secondary">
                  <a href={item.url} rel="noreferrer" target="_blank">
                    打开
                    <ExternalLink className="h-4 w-4" />
                  </a>
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}
    </div>
  );
}
