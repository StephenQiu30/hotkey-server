"use client";

import { useCallback, useEffect, useState } from "react";
import { Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import { PageHeader } from "@/components/dashboard/PageHeader";
import {
  CollectionWorkspace,
  type CollectionWorkspacePagination,
} from "@/components/dashboard/CollectionWorkspace";
import {
  DEFAULT_PAGE_SIZE,
} from "@/components/dashboard/CursorPagination";
import {
  getCollectionRuns,
  postCollectionRunsIdRetry,
} from "@/services/hotkey/hotkey-server/collectionRuns";
import { deleteContentsId, getContents } from "@/services/hotkey/hotkey-server/contents";
import { useAuthStore } from "@/stores/authStore";
import { UserRole } from "@/lib/domainEnums";

export default function ContentsPage() {
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const user = useAuthStore((state) => state.user);
  const canManage = user?.role === UserRole.Editor || user?.role === UserRole.Admin;
  const [runs, setRuns] = useState<HotKeyAPI.CollectionRunResponse[]>([]);
  const [contents, setContents] = useState<HotKeyAPI.ContentResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [runPage, setRunPage] = useState(1);
  const [runCursors, setRunCursors] = useState<(string | undefined)[]>([
    undefined,
  ]);
  const [runNextCursor, setRunNextCursor] = useState<string>();
  const [contentPage, setContentPage] = useState(1);
  const [contentCursors, setContentCursors] = useState<(string | undefined)[]>([
    undefined,
  ]);
  const [contentNextCursor, setContentNextCursor] = useState<string>();
  const [deleteTarget, setDeleteTarget] = useState<HotKeyAPI.ContentResponse>();
  const [deletingContentID, setDeletingContentID] = useState<number>();
  const [retryingRunID, setRetryingRunID] = useState<number>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const runRequest = canManage
        ? getCollectionRuns({ limit: pageSize })
        : undefined;
      const contentResult = await getContents({ limit: pageSize });
      setContents(contentResult.data?.items ?? []);
      setContentPage(1);
      setContentCursors([undefined]);
      setContentNextCursor(contentResult.data?.next_cursor);
      if (runRequest) {
        const runResult = await runRequest;
        setRuns(runResult.data?.items ?? []);
        setRunNextCursor(runResult.data?.next_cursor);
      } else {
        setRuns([]);
        setRunNextCursor(undefined);
      }
      setRunPage(1);
      setRunCursors([undefined]);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "采集数据加载失败");
    } finally {
      setLoading(false);
    }
  }, [canManage, pageSize]);

  const loadRunsPage = async (cursor: string | undefined, page: number) => {
    if (!canManage) return;
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
      toast.error(reason instanceof Error ? reason.message : "采集批次加载失败");
    } finally {
      setLoading(false);
    }
  };

  const loadContentsPage = async (cursor: string | undefined, page: number) => {
    setLoading(true);
    try {
      const result = await getContents({
        limit: pageSize,
        ...(cursor ? { cursor } : {}),
      });
      setContents(result.data?.items ?? []);
      setContentNextCursor(result.data?.next_cursor);
      setContentPage(page);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "内容加载失败");
    } finally {
      setLoading(false);
    }
  };

  const nextRunPage = () => {
    if (!runNextCursor) return;
    const page = runPage + 1;
    const cursor = runNextCursor;
    setRunCursors((history) => [...history.slice(0, runPage), cursor]);
    void loadRunsPage(cursor, page);
  };

  const previousRunPage = () => {
    if (runPage <= 1) return;
    void loadRunsPage(runCursors[runPage - 2], runPage - 1);
  };

  const nextContentPage = () => {
    if (!contentNextCursor) return;
    const page = contentPage + 1;
    const cursor = contentNextCursor;
    setContentCursors((history) => [...history.slice(0, contentPage), cursor]);
    void loadContentsPage(cursor, page);
  };

  const previousContentPage = () => {
    if (contentPage <= 1) return;
    void loadContentsPage(contentCursors[contentPage - 2], contentPage - 1);
  };

  const changePageSize = (nextPageSize: number) => {
    setPageSize(nextPageSize);
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
    if (!canManage || run.id == null) return;
    setRetryingRunID(run.id);
    try {
      await postCollectionRunsIdRetry({ id: run.id });
      await loadRunsPage(runCursors[runPage - 1], runPage);
      toast.success(`采集批次 #${run.id} 已重新进入队列`);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "采集批次重试失败");
    } finally {
      setRetryingRunID(undefined);
    }
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

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="app-page">
      <PageHeader
        action={
          <Button className="gap-2" onClick={load} variant="outline">
            <RefreshCw className={loading ? "animate-spin" : ""} />
            刷新数据
          </Button>
        }
        description="核对来源采集、内容标准化与事件聚合之间的真实数据进度。"
        eyebrow="Ingestion"
        title="采集内容"
      />
      {loading && !runs.length && !contents.length ? (
        <div className="flex min-h-80 items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <CollectionWorkspace
          canManage={canManage}
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
