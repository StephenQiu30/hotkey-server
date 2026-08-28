"use client";

import Link from "next/link";
import { useState, type FormEvent, type ReactNode } from "react";
import { Clock3, Loader2, Search, SlidersHorizontal } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { HotKeyAPIError } from "@/lib/request";
import { getSearch } from "@/services/hotkey/hotkey-server/search";

const resourceTypes = [
  { value: "content", label: "内容" },
  { value: "event", label: "事件" },
  { value: "knowledge", label: "知识" },
] as const;

const resourceLabels: Readonly<Record<string, string>> = {
  content: "内容",
  event: "事件",
  knowledge: "知识",
};

const statusLabels: Readonly<Record<string, string>> = {
  active: "有效",
  review_pending: "待复核",
  closed: "已关闭",
  merged: "已合并",
};

const highlightTokenPattern = /(<mark>|<\/mark>)/;

function optionalPositiveInteger(value: string): number | undefined {
  if (!value) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function dayBoundary(value: string, end: boolean): string | undefined {
  if (!value) return undefined;
  return new Date(`${value}T${end ? "23:59:59.999" : "00:00:00.000"}Z`).toISOString();
}

function decodeHighlightText(value: string): string {
  return value
    .replaceAll("&#34;", '"')
    .replaceAll("&#39;", "'")
    .replaceAll("&gt;", ">")
    .replaceAll("&lt;", "<")
    .replaceAll("&amp;", "&");
}

// The server emits escaped text plus the two controlled mark tokens. Parsing
// them into React nodes keeps every untrusted byte in a text node and avoids
// dangerouslySetInnerHTML entirely.
function SafeHighlight({ value, fallback }: { value?: string; fallback?: string }) {
  const source = value || fallback || "";
  const parts = source.split(highlightTokenPattern);
  let marked = false;
  const nodes: ReactNode[] = [];
  for (const [index, part] of parts.entries()) {
    if (part === "<mark>") {
      marked = true;
      continue;
    }
    if (part === "</mark>") {
      marked = false;
      continue;
    }
    const text = decodeHighlightText(part);
    nodes.push(marked ? <mark key={index}>{text}</mark> : text);
  }
  return nodes;
}

function resultPath(item: HotKeyAPI.SearchItemResponseDTO): string | undefined {
  if (!item.id) return undefined;
  if (item.type === "content") return `/dashboard/contents/${item.id}`;
  if (item.type === "event") return `/dashboard/events/${item.id}`;
  return undefined;
}

function formatTime(value?: string): string {
  if (!value) return "时间未知";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "时间未知";
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(parsed);
}

export default function SearchPage() {
  const [query, setQuery] = useState("");
  const [selectedTypes, setSelectedTypes] = useState<string[]>(resourceTypes.map((item) => item.value));
  const [showFilters, setShowFilters] = useState(false);
  const [sourceID, setSourceID] = useState("");
  const [monitorID, setMonitorID] = useState("");
  const [entity, setEntity] = useState("");
  const [status, setStatus] = useState("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [sort, setSort] = useState<"relevance" | "latest">("relevance");
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [permissionDenied, setPermissionDenied] = useState(false);
  const [error, setError] = useState<string>();
  const [response, setResponse] = useState<HotKeyAPI.SearchPageResponseDTO>();

  function toggleType(value: string) {
    setSelectedTypes((current) =>
      current.includes(value) ? current.filter((item) => item !== value) : [...current, value],
    );
  }

  async function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = query.trim();
    if (!normalized || selectedTypes.length === 0) return;
    setLoading(true);
    setError(undefined);
    setPermissionDenied(false);
    try {
      const parsedSourceID = optionalPositiveInteger(sourceID);
      const parsedMonitorID = optionalPositiveInteger(monitorID);
      const fromBoundary = dayBoundary(from, false);
      const toBoundary = dayBoundary(to, true);
      const result = await getSearch({
        q: normalized,
        limit: 50,
        sort,
        ...(selectedTypes.length === resourceTypes.length ? {} : { types: selectedTypes.join(",") }),
        ...(parsedSourceID ? { source_connection_id: parsedSourceID } : {}),
        ...(parsedMonitorID ? { monitor_id: parsedMonitorID } : {}),
        ...(entity.trim() ? { entity: entity.trim() } : {}),
        ...(status === "all" ? {} : { status }),
        ...(fromBoundary ? { from: fromBoundary } : {}),
        ...(toBoundary ? { to: toBoundary } : {}),
      });
      setResponse(result.data ?? { items: [] });
      setSearched(true);
    } catch (reason) {
      setResponse(undefined);
      setSearched(true);
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "全文检索失败");
    } finally {
      setLoading(false);
    }
  }

  const items = response?.items ?? [];

  return (
    <div className="app-page">
      <PageHeader
        eyebrow="POSTGRES SEARCH"
        title="全文检索"
        description="检索当前权限内的内容、事件与知识。结果来自 PostgreSQL 词法索引，不生成回答，也不调用向量或外部搜索。"
      />

      <Card className="border border-border bg-card">
        <CardContent className="p-5 sm:p-6">
          <form className="space-y-4" onSubmit={search}>
            <div className="flex flex-col gap-3 sm:flex-row">
              <div className="min-w-0 flex-1">
                <Label htmlFor="knowledge-search-query" className="sr-only">搜索词</Label>
                <Input
                  id="knowledge-search-query"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="输入关键词、实体、产品或技术主题"
                  maxLength={100}
                />
              </div>
              <Button
                type="button"
                variant="outline"
                aria-label="高级筛选"
                aria-expanded={showFilters}
                onClick={() => setShowFilters((value) => !value)}
              >
                <SlidersHorizontal className="h-4 w-4" />
                筛选
              </Button>
              <Button type="submit" disabled={loading || !query.trim() || selectedTypes.length === 0}>
                {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Search className="h-4 w-4" />}
                搜索
              </Button>
            </div>

            <fieldset>
              <legend className="mb-2 text-sm font-medium">资源类型</legend>
              <div className="flex flex-wrap gap-4">
                {resourceTypes.map((resource) => (
                  <label key={resource.value} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      aria-label={resource.label}
                      checked={selectedTypes.includes(resource.value)}
                      onCheckedChange={() => toggleType(resource.value)}
                    />
                    {resource.label}
                  </label>
                ))}
              </div>
            </fieldset>

            {showFilters ? (
              <fieldset className="grid gap-4 rounded-lg border border-border p-4 sm:grid-cols-2 lg:grid-cols-4">
                <legend className="px-1 text-sm font-medium">高级筛选</legend>
                <div className="space-y-2">
                  <Label htmlFor="search-source-id">来源 ID</Label>
                  <Input id="search-source-id" type="number" min={1} value={sourceID} onChange={(event) => setSourceID(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="search-monitor-id">Monitor ID</Label>
                  <Input id="search-monitor-id" type="number" min={1} value={monitorID} onChange={(event) => setMonitorID(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="search-entity">实体</Label>
                  <Input id="search-entity" maxLength={128} value={entity} onChange={(event) => setEntity(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="search-status">状态</Label>
                  <Select value={status} onValueChange={setStatus}>
                    <SelectTrigger id="search-status"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">全部状态</SelectItem>
                      <SelectItem value="active">有效</SelectItem>
                      <SelectItem value="review_pending">待复核</SelectItem>
                      <SelectItem value="closed">已关闭</SelectItem>
                      <SelectItem value="merged">已合并</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="search-from">开始日期</Label>
                  <Input id="search-from" type="date" value={from} onChange={(event) => setFrom(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="search-to">结束日期</Label>
                  <Input id="search-to" type="date" value={to} onChange={(event) => setTo(event.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="search-sort">排序</Label>
                  <Select value={sort} onValueChange={(value) => setSort(value as "relevance" | "latest")}>
                    <SelectTrigger id="search-sort"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="relevance">相关性</SelectItem>
                      <SelectItem value="latest">最新时间</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </fieldset>
            ) : null}
          </form>
        </CardContent>
      </Card>

      {permissionDenied ? (
        <Alert variant="destructive" className="mt-6">
          <AlertTitle>没有检索权限</AlertTitle>
          <AlertDescription>当前账号不能读取该检索范围，请联系管理员确认角色与对象权限。</AlertDescription>
        </Alert>
      ) : error ? (
        <Alert variant="destructive" className="mt-6">
          <AlertTitle>搜索失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {loading ? (
        <section aria-label="正在加载搜索结果" className="mt-6 space-y-3">
          {[0, 1, 2].map((item) => <div key={item} className="h-28 animate-pulse rounded-xl border border-border bg-muted/40" />)}
        </section>
      ) : null}

      {!loading && items.length > 0 ? (
        <section aria-label="全文检索结果" className="mt-6 space-y-4">
          <p className="text-sm text-muted-foreground">共返回 {items.length} 条当前可见结果</p>
          {items.map((item) => {
            const path = resultPath(item);
            const title = <SafeHighlight value={item.title_highlight} fallback={item.title} />;
            return (
              <Card key={`${item.type}-${item.id}`} className="border border-border bg-card">
                <CardContent className="space-y-3 p-5">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">{resourceLabels[item.type ?? ""] ?? item.type}</Badge>
                    {item.status ? <Badge variant="outline">{statusLabels[item.status] ?? item.status}</Badge> : null}
                    <span className="ml-auto flex items-center gap-1 text-xs text-muted-foreground">
                      <Clock3 className="h-3.5 w-3.5" />{formatTime(item.occurred_at)}
                    </span>
                  </div>
                  <h2 className="text-lg font-semibold">
                    {path ? <Link className="hover:underline" href={path}>{title}</Link> : title}
                  </h2>
                  {item.snippet || item.snippet_highlight ? (
                    <p className="text-sm leading-6 text-muted-foreground">
                      <SafeHighlight value={item.snippet_highlight} fallback={item.snippet} />
                    </p>
                  ) : null}
                  <p className="text-xs text-muted-foreground">相关性 {(item.score ?? 0).toFixed(3)}</p>
                </CardContent>
              </Card>
            );
          })}
        </section>
      ) : null}

      {searched && !loading && !error && items.length === 0 ? (
        <div className="mt-6 rounded-xl border border-dashed border-border px-6 py-14 text-center">
          <Search className="mx-auto h-6 w-6 text-muted-foreground" />
          <h2 className="mt-4 font-medium">没有符合条件的结果</h2>
          <p className="mt-2 text-sm text-muted-foreground">调整关键词、资源类型或高级筛选后重试。</p>
        </div>
      ) : null}
    </div>
  );
}
