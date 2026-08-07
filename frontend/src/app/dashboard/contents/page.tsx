"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { FilterX, Loader2, RefreshCw, Search } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import { PageHeader } from "@/components/dashboard/PageHeader";
import {
  CollectionWorkspace,
  type CollectionWorkspacePagination,
} from "@/components/dashboard/CollectionWorkspace";
import { DEFAULT_PAGE_SIZE } from "@/components/dashboard/CursorPagination";
import {
  getCollectionRuns,
  postCollectionRunsIdRetry,
} from "@/services/hotkey/hotkey-server/collectionRuns";
import {
  deleteContentsId,
  getContents,
} from "@/services/hotkey/hotkey-server/contents";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { useAuthStore } from "@/stores/authStore";
import { UserRole } from "@/lib/domainEnums";

type ContentSort = NonNullable<HotKeyAPI.getContentsParams["sort"]>;
type MatchDecision = NonNullable<HotKeyAPI.getContentsParams["decision"]>;

function positiveNumber(value: string | null) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function ContentsWorkspace() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const serializedSearch = searchParams.toString();
  const initialKeyword = searchParams.get("q")?.trim() ?? "";
  const initialCursor = searchParams.get("cursor") || undefined;
  const initialPage = positiveNumber(searchParams.get("page")) ?? 1;
  const [keyword, setKeyword] = useState(initialKeyword);
  const [searchInput, setSearchInput] = useState(initialKeyword);
  const [sourceId, setSourceId] = useState(
    positiveNumber(searchParams.get("source"))
  );
  const [monitorId, setMonitorId] = useState(
    positiveNumber(searchParams.get("monitor"))
  );
  const [publishedFrom, setPublishedFrom] = useState(
    searchParams.get("from") ?? ""
  );
  const [publishedTo, setPublishedTo] = useState(searchParams.get("to") ?? "");
  const [decision, setDecision] = useState<MatchDecision | undefined>(
    (searchParams.get("decision") as MatchDecision) || undefined
  );
  const [sort, setSort] = useState<ContentSort>(
    (searchParams.get("sort") as ContentSort) || "latest"
  );
  const [pageSize, setPageSize] = useState(
    positiveNumber(searchParams.get("limit")) ?? DEFAULT_PAGE_SIZE
  );
  const user = useAuthStore((state) => state.user);
  const canManage =
    user?.role === UserRole.Editor || user?.role === UserRole.Admin;
  const canViewRuns = canManage;
  const canRetry = user?.role === UserRole.Admin;
  const [runs, setRuns] = useState<HotKeyAPI.CollectionRunResponse[]>([]);
  const [contents, setContents] = useState<HotKeyAPI.ContentResponse[]>([]);
  const [sources, setSources] = useState<HotKeyAPI.SourceReadResponse[]>([]);
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [contentError, setContentError] = useState<string>();
  const [runPage, setRunPage] = useState(1);
  const [runCursors, setRunCursors] = useState<(string | undefined)[]>([
    undefined,
  ]);
  const [runNextCursor, setRunNextCursor] = useState<string>();
  const [contentPage, setContentPage] = useState(initialPage);
  const [currentContentCursor, setCurrentContentCursor] =
    useState(initialCursor);
  const [contentCursors, setContentCursors] = useState<(string | undefined)[]>(
    () => {
      const history = Array<string | undefined>(initialPage).fill(undefined);
      history[initialPage - 1] = initialCursor;
      return history;
    }
  );
  const [contentNextCursor, setContentNextCursor] = useState<string>();
  const [deleteTarget, setDeleteTarget] = useState<HotKeyAPI.ContentResponse>();
  const [deletingContentID, setDeletingContentID] = useState<number>();
  const [retryingRunID, setRetryingRunID] = useState<number>();

  const replaceURL = useCallback(
    (changes: Record<string, string | undefined>) => {
      const next = new URLSearchParams(searchParams.toString());
      Object.entries(changes).forEach(([key, value]) =>
        value ? next.set(key, value) : next.delete(key)
      );
      router.replace(
        `/dashboard/contents${next.size ? `?${next.toString()}` : ""}`,
        { scroll: false }
      );
    },
    [router, searchParams]
  );

  useEffect(() => {
    const params = new URLSearchParams(serializedSearch);
    const nextKeyword = params.get("q")?.trim() ?? "";
    const nextMonitor = positiveNumber(params.get("monitor"));
    const nextSort = (params.get("sort") as ContentSort) || "latest";
    const nextPage = positiveNumber(params.get("page")) ?? 1;
    const nextCursor = params.get("cursor") || undefined;
    setKeyword(nextKeyword);
    setSearchInput(nextKeyword);
    setSourceId(positiveNumber(params.get("source")));
    setMonitorId(nextMonitor);
    setPublishedFrom(params.get("from") ?? "");
    setPublishedTo(params.get("to") ?? "");
    setDecision(
      nextMonitor
        ? (params.get("decision") as MatchDecision) || undefined
        : undefined
    );
    setSort(nextMonitor || nextSort !== "relevance" ? nextSort : "latest");
    setPageSize(positiveNumber(params.get("limit")) ?? DEFAULT_PAGE_SIZE);
    setContentPage(nextPage);
    setCurrentContentCursor(nextCursor);
  }, [serializedSearch]);

  const contentParams = useCallback(
    (cursor?: string): HotKeyAPI.getContentsParams => ({
      limit: pageSize,
      ...(cursor ? { cursor } : {}),
      ...(keyword ? { q: keyword } : {}),
      ...(sourceId ? { source_connection_id: sourceId } : {}),
      ...(publishedFrom
        ? { published_from: `${publishedFrom}T00:00:00Z` }
        : {}),
      ...(publishedTo ? { published_to: `${publishedTo}T23:59:59Z` } : {}),
      ...(monitorId ? { monitor_id: monitorId } : {}),
      ...(decision ? { decision } : {}),
      ...(sort !== "latest" ? { sort } : {}),
    }),
    [
      decision,
      keyword,
      monitorId,
      pageSize,
      publishedFrom,
      publishedTo,
      sort,
      sourceId,
    ]
  );

  const loadContentsPage = useCallback(
    async (cursor: string | undefined, page: number) => {
      setLoading(true);
      setContentError(undefined);
      try {
        const result = await getContents(contentParams(cursor));
        setContents(result.data?.items ?? []);
        setContentNextCursor(result.data?.next_cursor);
        setContentPage(page);
      } catch (reason) {
        const message =
          reason instanceof Error ? reason.message : "内容加载失败";
        setContentError(message);
        setContents([]);
      } finally {
        setLoading(false);
      }
    },
    [contentParams]
  );

  const loadRunsPage = useCallback(
    async (cursor: string | undefined, page: number) => {
      if (!canViewRuns) return;
      setLoading(true);
      try {
        const result = await getCollectionRuns({
          limit: pageSize,
          ...(cursor ? { cursor } : {}),
        });
        setRuns(result.data?.items ?? []);
        setRunNextCursor(result.data?.next_cursor);
        setRunPage(page);
      } catch (reason) {
        toast.error(
          reason instanceof Error ? reason.message : "采集批次加载失败"
        );
      } finally {
        setLoading(false);
      }
    },
    [canViewRuns, pageSize]
  );

  const load = useCallback(async () => {
    setContentPage(1);
    setContentCursors([undefined]);
    setCurrentContentCursor(undefined);
    await Promise.all([
      loadContentsPage(undefined, 1),
      canViewRuns ? loadRunsPage(undefined, 1) : Promise.resolve(),
    ]);
    if (!canViewRuns) {
      setRuns([]);
      setRunNextCursor(undefined);
    }
    setRunPage(1);
    setRunCursors([undefined]);
  }, [canViewRuns, loadContentsPage, loadRunsPage]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const next = searchInput.trim();
      if (next === keyword) return;
      setKeyword(next);
      setCurrentContentCursor(undefined);
      setContentPage(1);
      setContentCursors([undefined]);
      replaceURL({ q: next || undefined, cursor: undefined, page: undefined });
    }, 300);
    return () => window.clearTimeout(timer);
  }, [keyword, replaceURL, searchInput]);

  useEffect(() => {
    void loadContentsPage(currentContentCursor, contentPage);
  }, [contentPage, currentContentCursor, loadContentsPage]);

  useEffect(() => {
    setRunPage(1);
    setRunCursors([undefined]);
    if (canViewRuns) {
      void loadRunsPage(undefined, 1);
    } else {
      setRuns([]);
      setRunNextCursor(undefined);
    }
  }, [canViewRuns, loadRunsPage]);

  useEffect(() => {
    let active = true;
    Promise.allSettled([
      getSourceConnections({ limit: 100 }),
      getMonitors({ limit: 100 }),
    ]).then(([sourceResult, monitorResult]) => {
      if (!active) return;
      setSources(
        sourceResult.status === "fulfilled"
          ? sourceResult.value.data?.items ?? []
          : []
      );
      setMonitors(
        monitorResult.status === "fulfilled"
          ? (monitorResult.value.data?.items ?? []).filter(
              (item) => item.status === "active" || item.status === "paused"
            )
          : []
      );
    });
    return () => {
      active = false;
    };
  }, []);

  const resetContentPage = (urlChanges: Record<string, string | undefined>) => {
    setContentPage(1);
    setContentCursors([undefined]);
    setCurrentContentCursor(undefined);
    replaceURL({ ...urlChanges, cursor: undefined, page: undefined });
  };

  const clearFilters = () => {
    setSearchInput("");
    setKeyword("");
    setSourceId(undefined);
    setMonitorId(undefined);
    setPublishedFrom("");
    setPublishedTo("");
    setDecision(undefined);
    setSort("latest");
    setContentPage(1);
    setContentCursors([undefined]);
    setCurrentContentCursor(undefined);
    replaceURL({
      q: undefined,
      source: undefined,
      monitor: undefined,
      from: undefined,
      to: undefined,
      decision: undefined,
      sort: undefined,
      cursor: undefined,
      page: undefined,
    });
  };

  const nextContentPage = () => {
    if (!contentNextCursor) return;
    const page = contentPage + 1;
    setContentCursors((history) => [
      ...history.slice(0, contentPage),
      contentNextCursor,
    ]);
    setCurrentContentCursor(contentNextCursor);
    setContentPage(page);
    replaceURL({ cursor: contentNextCursor, page: String(page) });
  };
  const previousContentPage = () => {
    if (contentPage <= 1) return;
    const page = contentPage - 1;
    const cursor = contentCursors[page - 1];
    setCurrentContentCursor(cursor);
    setContentPage(page);
    replaceURL({ cursor, page: page === 1 ? undefined : String(page) });
  };
  const nextRunPage = () => {
    if (!runNextCursor) return;
    const page = runPage + 1;
    setRunCursors((history) => [...history.slice(0, runPage), runNextCursor]);
    void loadRunsPage(runNextCursor, page);
  };
  const previousRunPage = () => {
    if (runPage > 1) void loadRunsPage(runCursors[runPage - 2], runPage - 1);
  };

  const deleteContent = async () => {
    const id = deleteTarget?.id;
    if (!canManage || id == null) return;
    setDeletingContentID(id);
    try {
      await deleteContentsId({ id });
      setDeleteTarget(undefined);
      await loadContentsPage(contentCursors[contentPage - 1], contentPage);
      toast.success("内容已删除，归档证据将按生命周期清理");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "内容删除失败");
    } finally {
      setDeletingContentID(undefined);
    }
  };

  const retryRun = async (run: HotKeyAPI.CollectionRunResponse) => {
    if (!canRetry || run.id == null) return;
    setRetryingRunID(run.id);
    try {
      await postCollectionRunsIdRetry({ id: run.id });
      await loadRunsPage(runCursors[runPage - 1], runPage);
      toast.success(`采集批次 #${run.id} 已重新进入队列`);
    } catch (reason) {
      toast.error(
        reason instanceof Error ? reason.message : "采集批次重试失败"
      );
    } finally {
      setRetryingRunID(undefined);
    }
  };

  const changePageSize = (value: number) => {
    setPageSize(value);
    setContentPage(1);
    setContentCursors([undefined]);
    setCurrentContentCursor(undefined);
    replaceURL({
      limit: value === DEFAULT_PAGE_SIZE ? undefined : String(value),
      cursor: undefined,
      page: undefined,
    });
  };
  const runsPagination: CollectionWorkspacePagination = {
    page: runPage,
    hasNext: Boolean(runNextCursor),
    loading,
    onPageSizeChange: changePageSize,
    onPrevious: previousRunPage,
    onNext: nextRunPage,
    pageSize,
  };
  const contentsPagination: CollectionWorkspacePagination = {
    page: contentPage,
    hasNext: Boolean(contentNextCursor),
    loading,
    onPageSizeChange: changePageSize,
    onPrevious: previousContentPage,
    onNext: nextContentPage,
    pageSize,
  };
  const filtered = Boolean(
    keyword ||
      sourceId ||
      monitorId ||
      publishedFrom ||
      publishedTo ||
      decision ||
      sort !== "latest"
  );

  return (
    <div className="app-page">
      <PageHeader
        action={
          <Button className="gap-2" onClick={load} variant="outline">
            <RefreshCw className={loading ? "animate-spin" : ""} />
            刷新数据
          </Button>
        }
        description="从服务端检索已标准化内容，并核对采集与事件归属。"
        eyebrow="Ingestion"
        title="采集内容"
      />
      <Card className="mt-6 shadow-none">
        <CardContent className="grid gap-4 p-4 lg:grid-cols-4">
          <div className="space-y-2 lg:col-span-2">
            <Label htmlFor="content-search">搜索内容</Label>
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="content-search"
                className="pl-9"
                maxLength={100}
                placeholder="搜索标题或摘要"
                type="search"
                value={searchInput}
                onChange={(event) => setSearchInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    const next = searchInput.trim();
                    setKeyword(next);
                    resetContentPage({ q: next || undefined });
                  }
                }}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label>来源</Label>
            <Select
              value={sourceId?.toString() ?? "all"}
              onValueChange={(value) => {
                const next = value === "all" ? undefined : Number(value);
                setSourceId(next);
                resetContentPage({ source: next?.toString() });
              }}
            >
              <SelectTrigger aria-label="内容来源">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部来源</SelectItem>
                {sources
                  .filter((item) => item.id != null && !item.deleted)
                  .map((item) => (
                    <SelectItem key={item.id} value={String(item.id)}>
                      {item.name || `来源 #${item.id}`}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>监控</Label>
            <Select
              value={monitorId?.toString() ?? "all"}
              onValueChange={(value) => {
                const next = value === "all" ? undefined : Number(value);
                setMonitorId(next);
                if (!next) {
                  setDecision(undefined);
                  if (sort === "relevance") setSort("latest");
                }
                resetContentPage({
                  monitor: next?.toString(),
                  decision: next ? decision : undefined,
                  sort: next || sort !== "relevance" ? sort : undefined,
                });
              }}
            >
              <SelectTrigger aria-label="内容监控">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部监控</SelectItem>
                {monitors
                  .filter((item) => item.id != null)
                  .map((item) => (
                    <SelectItem key={item.id} value={String(item.id)}>
                      {item.name || `监控 #${item.id}`}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="published-from">开始日期</Label>
            <Input
              id="published-from"
              type="date"
              value={publishedFrom}
              onChange={(event) => {
                setPublishedFrom(event.target.value);
                resetContentPage({ from: event.target.value });
              }}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="published-to">结束日期</Label>
            <Input
              id="published-to"
              type="date"
              value={publishedTo}
              onChange={(event) => {
                setPublishedTo(event.target.value);
                resetContentPage({ to: event.target.value });
              }}
            />
          </div>
          <div className="space-y-2">
            <Label>匹配决策</Label>
            <Select
              disabled={!monitorId}
              value={decision ?? "all"}
              onValueChange={(value) => {
                const next =
                  value === "all" ? undefined : (value as MatchDecision);
                setDecision(next);
                resetContentPage({ decision: next });
              }}
            >
              <SelectTrigger aria-label="匹配决策">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部决策</SelectItem>
                <SelectItem value="accepted">已接受</SelectItem>
                <SelectItem value="review">待复核</SelectItem>
                <SelectItem value="rejected">已拒绝</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>排序</Label>
            <Select
              value={sort}
              onValueChange={(value) => {
                const next = value as ContentSort;
                setSort(next);
                resetContentPage({
                  sort: next === "latest" ? undefined : next,
                });
              }}
            >
              <SelectTrigger aria-label="内容排序">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="latest">最新发布</SelectItem>
                <SelectItem value="relevance" disabled={!monitorId}>
                  监控相关性
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-wrap items-end gap-2 lg:col-span-4">
            <Button
              disabled={!filtered}
              onClick={clearFilters}
              variant="outline"
            >
              <FilterX />
              清除筛选
            </Button>
            {keyword ? (
              <Button asChild variant="ghost">
                <Link
                  href={`/dashboard/events?q=${encodeURIComponent(keyword)}`}
                >
                  在事件中搜索同一关键词
                </Link>
              </Button>
            ) : null}
          </div>
        </CardContent>
      </Card>
      {contentError ? (
        <Alert className="mt-6" variant="destructive">
          <AlertTitle>内容检索失败</AlertTitle>
          <AlertDescription>{contentError}</AlertDescription>
        </Alert>
      ) : null}
      {loading && !runs.length && !contents.length ? (
        <div className="flex min-h-80 items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <CollectionWorkspace
          canManage={canManage}
          canRetry={canRetry}
          contentEmptyDescription={
            filtered
              ? "没有符合当前检索条件的内容，请调整或清除筛选。"
              : undefined
          }
          contents={contents}
          contentsPagination={contentsPagination}
          deletingContentID={deletingContentID}
          onDelete={setDeleteTarget}
          onRetry={retryRun}
          retryingRunID={retryingRunID}
          runs={runs}
          runsPagination={runsPagination}
        />
      )}
      <ConfirmDeleteDialog
        description="内容会从采集列表和后续候选中移除；系统保留生命周期墓碑，并清理已归档的 Markdown 证据。"
        loading={deletingContentID === deleteTarget?.id}
        onConfirm={deleteContent}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        open={deleteTarget != null}
        resourceName={deleteTarget?.title || `内容 #${deleteTarget?.id ?? ""}`}
        title="删除采集内容"
      />
    </div>
  );
}

export default function ContentsPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-80 items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <ContentsWorkspace />
    </Suspense>
  );
}
