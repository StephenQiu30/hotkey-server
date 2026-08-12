"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  Database,
  Eye,
  FileText,
  Loader2,
  Power,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  getSourceConnections,
  patchSourceConnectionsId,
  postSourceConnections,
  postSourceConnectionsIdDisable,
  postSourceConnectionsIdEnable,
  postSourceConnectionsIdHealth,
  postSourceConnectionsIdArchive,
} from "@/services/hotkey/hotkey-server/sources";
import { getSourceHealthMessage } from "@/lib/sourceHealthMessages";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { useAuthStore } from "@/stores/authStore";
import { SourceAction, UserRole } from "@/lib/domainEnums";
import { sourceHealthPresentation } from "@/lib/domainPresentation";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import { SourceConnectionDialog } from "@/components/dashboard/SourceConnectionDialog";
import { SourceCredentialDialog } from "@/components/dashboard/SourceCredentialDialog";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";

function sourceStatus(source: HotKeyAPI.SourceReadResponse) {
  if (source.deleted) {
    return { label: "已归档", className: "text-muted-foreground" };
  }
  if (!source.enabled) {
    return { label: "已停用", className: "text-muted-foreground" };
  }
  return sourceHealthPresentation(source.health_status);
}

export default function SourcesPage() {
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const user = useAuthStore((state) => state.user);
  const canManage = user?.role === UserRole.Admin;
  const [sources, setSources] = useState<HotKeyAPI.SourceReadResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const [creating, setCreating] = useState(false);
  const [action, setAction] = useState<number>();
  const [deleteTarget, setDeleteTarget] =
    useState<HotKeyAPI.SourceReadResponse>();
  const [bodyStorageTarget, setBodyStorageTarget] =
    useState<HotKeyAPI.SourceReadResponse>();
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<string>();

  const loadPage = useCallback(
    async (cursor: string | undefined, pageNumber: number) => {
      setLoading(true);
      setLoadError(undefined);
      try {
        const result = await getSourceConnections({
          limit: pageSize,
          ...(cursor ? { cursor } : {}),
        });
        setSources(
          (result.data?.items ?? []).filter((source) => !source.deleted)
        );
        setPage(pageNumber);
        setNextCursor(result.data?.next_cursor);
      } catch (reason) {
        const message =
          reason instanceof Error ? reason.message : "来源加载失败";
        setLoadError(message);
        toast.error(message);
      } finally {
        setLoading(false);
      }
    },
    [pageSize]
  );

  const load = useCallback(async () => {
    setCursors([undefined]);
    await loadPage(undefined, 1);
  }, [loadPage]);
  useEffect(() => {
    load();
  }, [load]);

  const nextPage = () => {
    if (!nextCursor) return;
    const nextPageNumber = page + 1;
    setCursors((history) => [...history.slice(0, page), nextCursor]);
    void loadPage(nextCursor, nextPageNumber);
  };

  const previousPage = () => {
    if (page <= 1) return;
    void loadPage(cursors[page - 2], page - 1);
  };

  const changePageSize = (nextPageSize: number) => {
    setPageSize(nextPageSize);
  };

  const create = async (request: HotKeyAPI.CreateSourceRequest) => {
    if (!canManage) return false;
    setCreating(true);
    try {
      await postSourceConnections(request);
      await load();
      toast.success("来源连接已创建");
      return true;
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "创建失败");
      return false;
    } finally {
      setCreating(false);
    }
  };

  const operate = async (
    source: HotKeyAPI.SourceReadResponse,
    kind: SourceAction
  ) => {
    if (!canManage || source.id == null) return;
    setAction(source.id);
    try {
      if (kind === SourceAction.Health) {
        const result = await postSourceConnectionsIdHealth({ id: source.id });
        toast[result.data?.healthy ? "success" : "error"](
          result.data?.healthy
            ? "来源健康"
            : getSourceHealthMessage(result.data?.error_code)
        );
      } else if (source.enabled)
        await postSourceConnectionsIdDisable(
          { id: source.id },
          { expected_source_version: source.version ?? 0 }
        );
      else
        await postSourceConnectionsIdEnable(
          { id: source.id },
          { expected_source_version: source.version ?? 0 }
        );
      await load();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "操作失败");
    } finally {
      setAction(undefined);
    }
  };

  const deleteSource = async () => {
    if (!canManage || deleteTarget?.id == null || deleteTarget.enabled) return;
    setAction(deleteTarget.id);
    try {
      await postSourceConnectionsIdArchive(
        { id: deleteTarget.id },
        { expected_source_version: deleteTarget.version ?? 0 }
      );
      setDeleteTarget(undefined);
      await load();
      toast.success("来源已删除，已采集内容仍保留");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "来源删除失败");
    } finally {
      setAction(undefined);
    }
  };

  const enableBodyStorage = async () => {
    const source = bodyStorageTarget;
    if (!canManage || source?.id == null || source.config?.allow_body_storage)
      return;
    setAction(source.id);
    try {
      await patchSourceConnectionsId(
        { id: source.id },
        {
          expected_source_version: source.version ?? 0,
          config: { allow_body_storage: true },
        }
      );
      setBodyStorageTarget(undefined);
      await load();
      toast.success("已开启正文/摘要归档，下一次采集将更新内容");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "开启归档失败");
    } finally {
      setAction(undefined);
    }
  };

  const replaceCredential = async (
    source: HotKeyAPI.SourceReadResponse,
    credential: string
  ) => {
    if (!canManage || source.id == null) return false;
    setAction(source.id);
    try {
      await patchSourceConnectionsId(
        { id: source.id },
        {
          expected_source_version: source.version ?? 0,
          credential,
        }
      );
      await load();
      toast.success("来源凭据已替换，请重新执行健康探测");
      return true;
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "凭据替换失败");
      return false;
    } finally {
      setAction(undefined);
    }
  };

  return (
    <div className="app-page">
      <PageHeader
        eyebrow="Sources"
        title={canManage ? "来源管理" : "来源目录"}
        description={
          canManage
            ? "连接、探测并管理七类正式来源；单个来源失败不会阻塞其他来源，也不会触发隐藏回退。"
            : "查看工作区的七类来源连接、健康状态与采集边界。"
        }
        action={
          canManage ? (
            <SourceConnectionDialog busy={creating} onSubmit={create} />
          ) : undefined
        }
      />
      {!canManage && (
        <Alert className="mt-6">
          <Eye />
          <div className="mb-1 font-medium leading-none tracking-tight">
            只读来源目录
          </div>
          <AlertDescription>
            当前 {user?.role ?? UserRole.Viewer}{" "}
            角色可以查看来源状态；新增、探测和启停来源仅对管理员开放。
          </AlertDescription>
        </Alert>
      )}
      {loadError && (
        <Alert variant="destructive" className="mt-6">
          <div className="mb-1 font-medium leading-none tracking-tight">
            来源加载失败
          </div>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{loadError}</span>
            <Button variant="outline" size="sm" onClick={load}>
              重新加载
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {loading ? (
        <div className="flex h-72 items-center justify-center">
          <Loader2 className="animate-spin text-muted-foreground" />
        </div>
      ) : !sources.length && !loadError ? (
        <Card className="mt-6 gap-0 overflow-hidden py-0">
          <Empty className="h-72">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Database />
              </EmptyMedia>
              <EmptyTitle>还没有来源连接</EmptyTitle>
              <EmptyDescription>
                添加来源后即可查看连接状态和采集健康度。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPageSizeChange={changePageSize}
            onPrevious={previousPage}
            page={page}
            pageSize={pageSize}
          />
        </Card>
      ) : sources.length ? (
        <Card className="mt-6 gap-0 overflow-hidden py-0">
          <div
            className={`hidden gap-4 border-b border-border px-5 py-3 text-xs text-muted-foreground md:grid ${
              canManage
                ? "grid-cols-[minmax(0,1.5fr)_120px_120px_320px]"
                : "grid-cols-[minmax(0,1.5fr)_120px_120px]"
            }`}
          >
            <span>来源</span>
            <span>类型</span>
            <span>状态</span>
            {canManage && <span className="text-right">操作</span>}
          </div>
          <div className="divide-y divide-border">
            {sources.map((source) => {
              const status = sourceStatus(source);
              return (
                <div
                  key={source.id}
                  className={`grid gap-3 px-4 py-4 md:items-center md:gap-4 md:px-5 ${
                    canManage
                      ? "md:grid-cols-[minmax(0,1.5fr)_120px_120px_320px]"
                      : "md:grid-cols-[minmax(0,1.5fr)_120px_120px]"
                  }`}
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">
                      {source.name}
                    </p>
                    {source.endpoint && (
                      <p className="mt-1 break-all text-xs text-muted-foreground md:truncate">
                        {source.endpoint}
                      </p>
                    )}
                    <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      {source.terms_policy_url && (
                        <a
                          className="underline underline-offset-4 hover:text-foreground"
                          href={source.terms_policy_url}
                          target="_blank"
                          rel="noreferrer"
                        >
                          条款与政策
                        </a>
                      )}
                      {source.credential_configured && <span>凭据已配置</span>}
                      {source.source_type === "bing_grounding" && (
                        <span>模型生成的派生证据 · 保留引用</span>
                      )}
                      {source.config?.rate_limit_per_minute != null &&
                        source.config?.content_retention_days != null && (
                          <span>
                            {source.config.rate_limit_per_minute} req/min · 保留{" "}
                            {source.config.content_retention_days} 天
                          </span>
                        )}
                    </div>
                  </div>
                  <span className="mono text-xs text-muted-foreground">
                    {source.source_type}
                  </span>
                  <span className={`text-xs ${status.className}`}>
                    {status.label}
                  </span>
                  {canManage && (
                    <div className="flex flex-wrap justify-start gap-2 md:justify-end">
                      {source.credential_configured && (
                        <SourceCredentialDialog
                          busy={action === source.id}
                          sourceName={source.name ?? `来源 #${source.id}`}
                          onSubmit={(credential) =>
                            replaceCredential(source, credential)
                          }
                        />
                      )}
                      {!source.config?.allow_body_storage && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setBodyStorageTarget(source)}
                          disabled={action === source.id || source.deleted}
                          className="gap-1.5"
                        >
                          <FileText />
                          开启归档
                        </Button>
                      )}
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => operate(source, SourceAction.Health)}
                        disabled={action === source.id}
                        className="gap-1.5"
                      >
                        <Activity />
                        探测
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => operate(source, SourceAction.Toggle)}
                        disabled={action === source.id || source.deleted}
                        className="gap-1.5"
                      >
                        <Power />
                        {source.enabled ? "停用" : "启用"}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setDeleteTarget(source)}
                        disabled={action === source.id || source.enabled}
                        title={source.enabled ? "请先停用来源" : "删除来源"}
                        className="gap-1.5 text-destructive hover:text-destructive"
                      >
                        <Trash2 />
                        删除
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPrevious={previousPage}
            page={page}
          />
        </Card>
      ) : null}
      <div className="mt-4 flex justify-end">
        <Button
          variant="ghost"
          onClick={load}
          className="gap-2 text-muted-foreground"
        >
          <RefreshCw />
          刷新来源
        </Button>
      </div>
      <ConfirmDeleteDialog
        open={deleteTarget != null}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title="删除来源"
        description="来源配置将从列表中移除；已采集内容、证据和历史报告不会被删除。"
        resourceName={deleteTarget?.name || `来源 #${deleteTarget?.id ?? ""}`}
        onConfirm={deleteSource}
        loading={action === deleteTarget?.id}
      />
      <AlertDialog
        open={bodyStorageTarget != null}
        onOpenChange={(open) => !open && setBodyStorageTarget(undefined)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>开启正文与摘要归档？</AlertDialogTitle>
            <AlertDialogDescription>
              只保存该来源 Feed
              实际提供的正文或摘要，不抓取原网页。开启后将在后续采集时归档，请先确认来源条款允许保存。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={action === bodyStorageTarget?.id}>
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              aria-label="确认开启"
              disabled={action === bodyStorageTarget?.id}
              onClick={() => void enableBodyStorage()}
            >
              确认开启
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
