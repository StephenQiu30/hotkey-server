"use client";

import { useMemo, useState, type FormEvent } from "react";
import {
  ArrowUpRight,
  ChevronDown,
  Flame,
  Loader2,
  Search,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
import { PageHeader } from "@/components/dashboard/PageHeader";
import { postSearch } from "@/services/hotkey/hotkey-server/search";
import {
  instantSearchSourceOptions,
  sourceTypeLabel,
} from "@/lib/sourceLabels";

type SortMode =
  | "discovered"
  | "published"
  | "importance"
  | "relevance"
  | "heat";

const importanceOrder: Readonly<Record<string, number>> = {
  low: 1,
  medium: 2,
  high: 3,
  urgent: 4,
};

const statusLabels: Readonly<Record<string, string>> = {
  success: "成功",
  empty: "暂无结果",
  partial: "部分成功",
  failed: "失败",
  unavailable: "不可用",
};

const errorLabels: Readonly<Record<string, string>> = {
  not_configured: "未配置",
  rate_limited: "请求受限",
  unavailable: "来源不可用",
  request_failed: "请求失败",
  invalid_configuration: "配置无效",
};

const importanceLabels: Readonly<Record<string, string>> = {
  low: "低",
  medium: "中",
  high: "高",
  urgent: "紧急",
};

const qualityLabels: Readonly<Record<string, string>> = {
  credible: "可信",
  suspicious: "需复核",
  unavailable: "AI 未配置",
};

function timeValue(value: string | undefined) {
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function formatTime(value: string | undefined) {
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

function metrics(card: HotKeyAPI.HotspotCardResponse) {
  const values = [
    ["浏览", card.metrics?.view_count],
    ["点赞", card.metrics?.like_count],
    ["评论", card.metrics?.comment_count],
    ["分享", card.metrics?.share_count],
  ] as const;
  const result: Array<readonly [string, number]> = [];
  values.forEach(([label, value]) => {
    if (typeof value === "number" && Number.isFinite(value)) {
      result.push([label, value]);
    }
  });
  return result;
}

export default function SearchPage() {
  const allSources = instantSearchSourceOptions.map((source) => source.value);
  const [query, setQuery] = useState("");
  const [selectedSources, setSelectedSources] = useState(allSources);
  const [showSources, setShowSources] = useState(false);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [error, setError] = useState<string>();
  const [response, setResponse] = useState<HotKeyAPI.InstantSearchResponse>();
  const [sourceFilter, setSourceFilter] = useState("all");
  const [importanceFilter, setImportanceFilter] = useState("all");
  const [qualityFilter, setQualityFilter] = useState("all");
  const [publishedFrom, setPublishedFrom] = useState("");
  const [publishedTo, setPublishedTo] = useState("");
  const [sort, setSort] = useState<SortMode>("heat");

  const results = useMemo(() => {
    const from = publishedFrom ? Date.parse(`${publishedFrom}T00:00:00`) : 0;
    const to = publishedTo
      ? Date.parse(`${publishedTo}T23:59:59.999`)
      : Number.POSITIVE_INFINITY;
    return [...(response?.results ?? [])]
      .filter(
        (item) =>
          (sourceFilter === "all" || item.source_type === sourceFilter) &&
          (importanceFilter === "all" ||
            item.importance === importanceFilter) &&
          (qualityFilter === "all" || item.quality_state === qualityFilter) &&
          (!item.published_at ||
            (timeValue(item.published_at) >= from &&
              timeValue(item.published_at) <= to))
      )
      .sort((left, right) => {
        if (sort === "discovered")
          return timeValue(right.discovered_at) - timeValue(left.discovered_at);
        if (sort === "published")
          return timeValue(right.published_at) - timeValue(left.published_at);
        if (sort === "importance")
          return (
            (importanceOrder[right.importance ?? ""] ?? 0) -
            (importanceOrder[left.importance ?? ""] ?? 0)
          );
        if (sort === "relevance")
          return (right.relevance ?? 0) - (left.relevance ?? 0);
        return (right.heat_score ?? 0) - (left.heat_score ?? 0);
      });
  }, [
    importanceFilter,
    publishedFrom,
    publishedTo,
    qualityFilter,
    response,
    sort,
    sourceFilter,
  ]);

  const visibleSourceTypes = useMemo(
    () =>
      Array.from(
        new Set([
          ...(response?.source_statuses ?? []).map(
            (status) => status.source_type ?? ""
          ),
          ...(response?.results ?? []).map((item) => item.source_type ?? ""),
        ])
      ).filter(Boolean),
    [response]
  );

  async function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = query.trim();
    if (!normalized || selectedSources.length === 0) return;
    setLoading(true);
    setError(undefined);
    try {
      const result = await postSearch({
        query: normalized,
        limit: 50,
        ...(selectedSources.length === allSources.length
          ? {}
          : { source_types: selectedSources }),
      });
      setResponse(result.data);
      setSearched(true);
    } catch (reason) {
      setResponse(undefined);
      setSearched(true);
      setError(reason instanceof Error ? reason.message : "即时搜索失败");
    } finally {
      setLoading(false);
    }
  }

  function toggleSource(sourceType: string) {
    setSelectedSources((current) =>
      current.includes(sourceType)
        ? current.filter((value) => value !== sourceType)
        : [...current, sourceType]
    );
  }

  return (
    <main className="app-page">
      <PageHeader
        eyebrow="LIVE SEARCH"
        title="即时搜索"
        description="临时查询已配置的合规来源。结果不会创建监控、写入热点库或触发通知。"
      />

      <Card className="border border-border bg-card">
        <CardContent className="p-5 sm:p-6">
          <form className="space-y-4" onSubmit={search}>
            <div className="flex flex-col gap-3 sm:flex-row">
              <div className="min-w-0 flex-1">
                <Label htmlFor="instant-search-query" className="sr-only">
                  搜索词
                </Label>
                <Input
                  id="instant-search-query"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="输入公司、产品、人物或技术主题"
                  maxLength={200}
                />
              </div>
              <Button
                type="button"
                variant="outline"
                aria-label="选择来源"
                aria-expanded={showSources}
                onClick={() => setShowSources((value) => !value)}
              >
                选择来源
                <Badge variant="secondary">{selectedSources.length}</Badge>
                <ChevronDown className="h-4 w-4" />
              </Button>
              <Button
                type="submit"
                disabled={loading || !query.trim() || selectedSources.length === 0}
              >
                {loading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Search className="h-4 w-4" />
                )}
                立即搜索
              </Button>
            </div>

            {showSources ? (
              <fieldset className="rounded-lg border border-border p-4">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <legend className="text-sm font-medium">搜索来源</legend>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setSelectedSources([])}
                    >
                      清空
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setSelectedSources(allSources)}
                    >
                      全选
                    </Button>
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {instantSearchSourceOptions.map((source) => (
                    <label
                      key={source.value}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Checkbox
                        aria-label={source.label}
                        checked={selectedSources.includes(source.value)}
                        onCheckedChange={() => toggleSource(source.value)}
                      />
                      {source.label}
                    </label>
                  ))}
                </div>
              </fieldset>
            ) : null}
          </form>
        </CardContent>
      </Card>

      {error ? (
        <Alert variant="destructive" className="mt-6">
          <AlertTitle>搜索失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {response ? (
        <section aria-labelledby="source-status-heading" className="mt-8">
          <div className="mb-3 flex items-center justify-between gap-4">
            <h2 id="source-status-heading" className="text-sm font-medium">
              来源状态
            </h2>
            <p className="text-xs text-muted-foreground">
              共 {response.results?.length ?? 0} 条 · 搜索于 {formatTime(response.searched_at)}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {(response.source_statuses ?? []).map((status) => (
              <div
                key={`${status.source_type}-${status.source_name ?? ""}`}
                className="flex items-center gap-2 rounded-full border border-border px-3 py-1.5 text-xs"
              >
                <span className="font-medium">
                  {status.source_name || sourceTypeLabel(status.source_type)}
                </span>
                <span className="text-muted-foreground">
                  {statusLabels[status.state ?? ""] ?? status.state}
                  {status.state === "success" || status.state === "empty"
                    ? ` · ${status.result_count ?? 0} 条`
                    : ""}
                </span>
                {status.error_code ? (
                  <Badge
                    variant={
                      status.state === "failed" ? "destructive" : "secondary"
                    }
                  >
                    {errorLabels[status.error_code] ?? status.error_code}
                  </Badge>
                ) : null}
              </div>
            ))}
          </div>
        </section>
      ) : null}

      {response?.results?.length ? (
        <section aria-label="搜索结果筛选" className="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-6">
          <Select value={sourceFilter} onValueChange={setSourceFilter}>
            <SelectTrigger aria-label="来源筛选">
              <SelectValue placeholder="全部来源" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部来源</SelectItem>
              {visibleSourceTypes.map((sourceType) => (
                <SelectItem key={sourceType} value={sourceType}>
                  {sourceTypeLabel(sourceType)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={importanceFilter} onValueChange={setImportanceFilter}>
            <SelectTrigger aria-label="重要性筛选">
              <SelectValue placeholder="全部重要性" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部重要性</SelectItem>
              {Object.entries(importanceLabels).map(([value, label]) => (
                <SelectItem key={value} value={value}>{label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={qualityFilter} onValueChange={setQualityFilter}>
            <SelectTrigger aria-label="质量筛选">
              <SelectValue placeholder="全部质量状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部质量状态</SelectItem>
              {Object.entries(qualityLabels).map(([value, label]) => (
                <SelectItem key={value} value={value}>{label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            aria-label="发布时间从"
            type="date"
            value={publishedFrom}
            onChange={(event) => setPublishedFrom(event.target.value)}
          />
          <Input
            aria-label="发布时间到"
            type="date"
            value={publishedTo}
            onChange={(event) => setPublishedTo(event.target.value)}
          />
          <Select value={sort} onValueChange={(value) => setSort(value as SortMode)}>
            <SelectTrigger aria-label="排序">
              <SelectValue placeholder="排序" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="discovered">最新发现</SelectItem>
              <SelectItem value="published">最新发布</SelectItem>
              <SelectItem value="importance">重要性</SelectItem>
              <SelectItem value="relevance">相关性</SelectItem>
              <SelectItem value="heat">热度</SelectItem>
            </SelectContent>
          </Select>
        </section>
      ) : null}

      <section aria-live="polite" className="mt-6 space-y-4">
        {results.map((card) => (
          <Card
            key={`${card.source_type}-${card.external_id}`}
            className="border border-border bg-card"
          >
            <CardHeader className="gap-3 p-5 pb-3 sm:p-6 sm:pb-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">
                  {sourceTypeLabel(card.source_type)}
                </Badge>
                <Badge variant="secondary">
                  重要性 {importanceLabels[card.importance ?? ""] ?? "未知"}
                </Badge>
                <Badge variant="secondary">
                  {qualityLabels[card.quality_state ?? ""] ?? "质量未知"}
                </Badge>
                <span className="inline-flex items-center gap-1 text-xs font-medium">
                  <Flame className="h-3.5 w-3.5" />
                  热度 {card.heat_score ?? 0}
                </span>
                <span className="text-xs text-muted-foreground">
                  相关性 {card.relevance ?? 0}%
                </span>
              </div>
              <CardTitle className="text-xl leading-7">
                <h2>{card.title || "无标题"}</h2>
              </CardTitle>
              <CardDescription className="leading-6">
                {card.summary || "来源未提供摘要。"}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4 p-5 pt-0 sm:p-6 sm:pt-0">
              <div className="flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground">
                {card.author ? <span>作者 {card.author}</span> : null}
                <span>发布 {formatTime(card.published_at)}</span>
                <span>发现 {formatTime(card.discovered_at)}</span>
                {metrics(card).map(([label, value]) => (
                  <span key={label}>{label} {value.toLocaleString("zh-CN")}</span>
                ))}
              </div>
              <div className="flex flex-col gap-3 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
                <p className="text-xs text-muted-foreground">
                  {card.relevance_reason || "未提供判断理由"}
                </p>
                {card.canonical_url ? (
                  <Button asChild variant="outline" size="sm">
                    <a
                      href={card.canonical_url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      查看原文
                      <ArrowUpRight className="h-4 w-4" />
                    </a>
                  </Button>
                ) : null}
              </div>
            </CardContent>
          </Card>
        ))}

        {searched && !loading && !error && results.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border px-6 py-14 text-center">
            <Search className="mx-auto h-6 w-6 text-muted-foreground" />
            <h2 className="mt-4 font-medium">没有符合条件的结果</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              查看来源状态，或调整搜索词、来源和筛选条件。
            </p>
          </div>
        ) : null}
      </section>
    </main>
  );
}
