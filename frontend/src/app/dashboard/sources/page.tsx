"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  Database,
  Eye,
  FileText,
  Loader2,
  Plus,
  Power,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { SourceAction, SourceType, UserRole } from "@/lib/domainEnums";
import { sourceHealthPresentation } from "@/lib/domainPresentation";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import { Checkbox } from "@/components/ui/checkbox";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";

const emptySourceForm = () => ({
  name: "",
  source_type: SourceType.RSS,
  endpoint: "",
  auth_type: "none" as "none" | "api_key" | "oauth2" | "bearer",
  allow_body_storage: true,
});

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
  const [dialog, setDialog] = useState(false);
  const [action, setAction] = useState<number>();
  const [deleteTarget, setDeleteTarget] =
    useState<HotKeyAPI.SourceReadResponse>();
  const [bodyStorageTarget, setBodyStorageTarget] =
    useState<HotKeyAPI.SourceReadResponse>();
  const [form, setForm] = useState(emptySourceForm);
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<string>();

  const changeDialog = (open: boolean) => {
    setDialog(open);
    if (!open) setForm(emptySourceForm());
  };

  const loadPage = useCallback(
    async (cursor: string | undefined, pageNumber: number) => {
      setLoading(true);
      try {
        const result = await getSourceConnections({
          limit: pageSize,
          ...(cursor ? { cursor } : {}),
        });
        setSources(
          (result.data?.items ?? []).filter((source) => !source.deleted),
        );
        setPage(pageNumber);
        setNextCursor(result.data?.next_cursor);
      } catch (reason) {
        toast.error(reason instanceof Error ? reason.message : "来源加载失败");
      } finally {
        setLoading(false);
      }
    },
    [pageSize],
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

  const create = async () => {
    if (!canManage || !form.name || !form.endpoint) return;
    try {
      await postSourceConnections({
        name: form.name,
        source_type: form.source_type,
        endpoint: form.endpoint,
        auth_type: form.auth_type,
        enabled: true,
        config: {
          allow_body_storage: form.allow_body_storage,
        },
      });
      changeDialog(false);
      await load();
      toast.success("来源连接已创建");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "创建失败");
    }
  };

  const operate = async (
    source: HotKeyAPI.SourceReadResponse,
    kind: SourceAction,
  ) => {
    if (!canManage || source.id == null) return;
    setAction(source.id);
    try {
      if (kind === SourceAction.Health) {
        const result = await postSourceConnectionsIdHealth({ id: source.id });
        toast[result.data?.healthy ? "success" : "error"](
          result.data?.healthy
            ? "来源健康"
            : getSourceHealthMessage(result.data?.error_code),
        );
      } else if (source.enabled)
        await postSourceConnectionsIdDisable(
          { id: source.id },
          { expected_source_version: source.version ?? 0 },
        );
      else
        await postSourceConnectionsIdEnable(
          { id: source.id },
          { expected_source_version: source.version ?? 0 },
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
        { expected_source_version: deleteTarget.version ?? 0 },
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
        },
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

  return (
    <div className="app-page">
      <PageHeader
        eyebrow="Sources"
        title={canManage ? "来源管理" : "来源目录"}
        description={
          canManage
            ? "连接、探测并管理官方 RSS、Atom 与 Hacker News 数据源。"
            : "查看当前工作区已接入的 RSS、Atom 与 Hacker News 数据源。"
        }
        action={
          canManage ? (
            <Dialog open={dialog} onOpenChange={changeDialog}>
              <DialogTrigger asChild>
                <Button className="gap-2">
                  <Plus />
                  新增来源
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>新增来源连接</DialogTitle>
                </DialogHeader>
                <div className="space-y-4 py-2">
                  <div>
                    <Label htmlFor="source-name">名称</Label>
                    <Input
                      id="source-name"
                      value={form.name}
                      onChange={(event) =>
                        setForm({ ...form, name: event.target.value })
                      }
                      placeholder="OpenAI 官方博客"
                      className="mt-2"
                    />
                  </div>
                  <div>
                    <Label>来源类型</Label>
                    <Select
                      value={form.source_type}
                      onValueChange={(value) =>
                        setForm({
                          ...form,
                          source_type: value as SourceType,
                          allow_body_storage: true,
                        })
                      }
                    >
                      <SelectTrigger className="mt-2">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={SourceType.RSS}>
                          RSS / Atom
                        </SelectItem>
                        <SelectItem value={SourceType.HackerNews}>
                          Hacker News
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div>
                    <Label htmlFor="source-endpoint">接口地址</Label>
                    <Input
                      id="source-endpoint"
                      value={form.endpoint}
                      onChange={(event) =>
                        setForm({ ...form, endpoint: event.target.value })
                      }
                      placeholder="https://example.com/feed.xml"
                      className="mt-2"
                    />
                  </div>
                  <div className="rounded-md border border-border p-4">
                    <div className="flex items-start gap-3">
                      <Checkbox
                        aria-label="保存来源正文/摘要用于归档预览"
                        checked={form.allow_body_storage}
                        id="source-allow-body-storage"
                        onCheckedChange={(checked) =>
                          setForm({
                            ...form,
                            allow_body_storage: checked === true,
                          })
                        }
                      />
                      <div className="min-w-0">
                        <Label htmlFor="source-allow-body-storage">
                          保存来源正文/摘要用于归档预览
                        </Label>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">
                          只保存来源 Feed
                          实际提供的正文/摘要，不抓取原网页；启用前确认来源条款。
                        </p>
                      </div>
                    </div>
                  </div>
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => changeDialog(false)}>
                    取消
                  </Button>
                  <Button
                    onClick={create}
                    disabled={!form.name || !form.endpoint}
                  >
                    创建连接
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          ) : undefined
        }
      />
      {!canManage && (
        <div className="panel mt-6 flex items-start gap-3 px-4 py-3 text-sm">
          <Eye className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
          <div>
            <p className="font-medium">只读来源目录</p>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              当前 {user?.role ?? UserRole.Viewer}{" "}
              角色可以查看来源状态；新增、探测和启停来源仅对管理员开放。
            </p>
          </div>
        </div>
      )}
      {loading ? (
        <div className="flex h-72 items-center justify-center">
          <Loader2 className="animate-spin text-muted-foreground" />
        </div>
      ) : !sources.length ? (
        <div className="panel mt-6">
          <div className="flex h-72 flex-col items-center justify-center">
            <Database className="mb-3 h-6 w-6 text-muted-foreground" />
            <p className="text-sm">还没有来源连接</p>
          </div>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPageSizeChange={changePageSize}
            onPrevious={previousPage}
            page={page}
            pageSize={pageSize}
          />
        </div>
      ) : (
        <div className="panel mt-6 overflow-hidden">
          <div
            className={`hidden gap-4 border-b border-border px-5 py-3 text-xs text-muted-foreground md:grid ${
              canManage
                ? "grid-cols-[minmax(0,1.5fr)_120px_120px_250px]"
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
                      ? "md:grid-cols-[minmax(0,1.5fr)_120px_120px_250px]"
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
                  </div>
                  <span className="mono text-xs text-muted-foreground">
                    {source.source_type}
                  </span>
                  <span className={`text-xs ${status.className}`}>
                    {status.label}
                  </span>
                  {canManage && (
                    <div className="flex flex-wrap justify-start gap-2 md:justify-end">
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
        </div>
      )}
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
