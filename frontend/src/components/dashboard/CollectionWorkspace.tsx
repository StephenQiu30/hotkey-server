import { ExternalLink, FileSearch, RadioTower, RotateCcw, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CollectionRunStatus } from "@/lib/domainEnums";
import { collectionRunPresentation } from "@/lib/domainPresentation";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
} from "@/components/dashboard/CursorPagination";

export type CollectionWorkspacePagination = {
  page: number;
  hasNext: boolean;
  loading?: boolean;
  pageSize?: number;
  onPageSizeChange?: (pageSize: number) => void;
  onPrevious: () => void;
  onNext: () => void;
};

type CollectionWorkspaceProps = {
  runs: HotKeyAPI.CollectionRunResponse[];
  contents: HotKeyAPI.ContentResponse[];
  canManage?: boolean;
  deletingContentID?: number;
  retryingRunID?: number;
  onDelete?: (content: HotKeyAPI.ContentResponse) => void;
  onRetry?: (run: HotKeyAPI.CollectionRunResponse) => void;
  runsPagination?: CollectionWorkspacePagination;
  contentsPagination?: CollectionWorkspacePagination;
};

const formatDateTime = (value?: string) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date(value))
    : "—";

export function CollectionWorkspace({
  runs,
  contents,
  canManage = false,
  deletingContentID,
  retryingRunID,
  onDelete,
  onRetry,
  runsPagination,
  contentsPagination,
}: CollectionWorkspaceProps) {
  const succeeded = runs.filter(
    (run) => run.status === CollectionRunStatus.Succeeded,
  ).length;
  const failed = runs.filter(
    (run) => run.status === CollectionRunStatus.Failed,
  ).length;
  const active = runs.filter(
    (run) =>
      run.status === CollectionRunStatus.Queued ||
      run.status === CollectionRunStatus.Running,
  ).length;

  return (
    <div className="mt-6 space-y-5">
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {[
          ["当前页采集批次", runs.length],
          ["处理中", active],
          ["采集成功", succeeded],
          ["最近入库内容", contents.length],
        ].map(([label, value]) => (
          <div className="panel p-4" key={label}>
            <p className="text-xs text-muted-foreground">{label}</p>
            <p className="mono mt-3 text-2xl font-medium">{value}</p>
          </div>
        ))}
      </section>

      <div data-testid="collection-pipeline" className="grid items-stretch gap-5 lg:grid-cols-2">
        <section className="panel flex h-full min-w-0 flex-col overflow-hidden">
        <div className="flex min-h-[84px] items-center justify-between border-b border-border px-5 py-4">
          <div>
            <h2 className="text-sm font-medium">采集批次（当前页）</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              按批次编号展示调度器与来源连接的真实执行结果，每页 {runsPagination?.pageSize ?? DEFAULT_PAGE_SIZE} 条。
            </p>
          </div>
          <RadioTower className="h-4 w-4 text-muted-foreground" />
        </div>
        {runs.length ? (
          <div className="flex-1 divide-y divide-border">
            {runs.map((run) => {
              const status = collectionRunPresentation(run.status);
              return (
                <article
                  className="grid gap-3 px-5 py-4 sm:grid-cols-[80px_minmax(0,1fr)_120px] sm:items-center"
                  key={run.id}
                >
                  <span className="mono text-xs text-muted-foreground">
                    #{run.id}
                  </span>
                  <span className="min-w-0">
                    <span className="block text-xs text-muted-foreground">
                      候选 {run.candidate_count ?? 0} · 接受 {run.accepted_count ?? 0} ·
                      拒绝 {run.rejected_count ?? 0}
                    </span>
                    {run.error_code ? (
                      <span className="mono mt-1 block truncate text-xs text-red-400">
                        {run.error_code}
                      </span>
                    ) : (
                      <span className="mt-1 block text-xs text-muted-foreground">
                        完成于 {formatDateTime(run.finished_at ?? run.started_at)}
                      </span>
                    )}
                  </span>
                  <span className="flex items-center gap-2 sm:ml-auto">
                    <Badge
                      className={`w-fit ${status.className}`}
                      variant="outline"
                    >
                      {status.label}
                    </Badge>
                    {canManage &&
                    run.id != null &&
                    onRetry &&
                    (run.status === CollectionRunStatus.Failed ||
                      run.status === CollectionRunStatus.Cancelled) ? (
                      <Button
                        aria-label={`重试采集批次 #${run.id}`}
                        className="h-7 gap-1 px-2 text-xs"
                        disabled={retryingRunID === run.id}
                        onClick={() => onRetry(run)}
                        size="sm"
                        variant="outline"
                      >
                        <RotateCcw className={retryingRunID === run.id ? "animate-spin" : ""} />
                        重试
                      </Button>
                    ) : null}
                  </span>
                </article>
              );
            })}
          </div>
        ) : (
          <div className="flex min-h-48 flex-1 flex-col items-center justify-center px-5 text-center">
            <RadioTower className="mb-3 h-5 w-5 text-muted-foreground" />
            <p className="text-sm font-medium">尚未产生采集批次</p>
            <p className="mt-1 max-w-md text-xs leading-5 text-muted-foreground">
              发布监控后仍长期停留在这里，通常表示后台调度器未创建任务。
            </p>
          </div>
        )}
        {runsPagination ? (
          <CursorPagination {...runsPagination} />
        ) : null}
        </section>

        <section className="panel flex h-full min-w-0 flex-col overflow-hidden">
        <div className="flex min-h-[84px] items-center justify-between border-b border-border px-5 py-4">
          <div>
            <h2 className="text-sm font-medium">最近入库内容</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              采集成功后完成标准化的真实内容，每页 {contentsPagination?.pageSize ?? DEFAULT_PAGE_SIZE} 条。
            </p>
          </div>
          <FileSearch className="h-4 w-4 text-muted-foreground" />
        </div>
        {contents.length ? (
          <div className="flex-1 divide-y divide-border">
            {contents.map((content, index) => {
              const title = content.title || content.external_id || `内容 #${content.id ?? "—"}`;
              return (
                <article
                  className="px-5 py-4"
                  key={content.id ?? `${content.external_id ?? title}-${index}`}
                >
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <span>{content.source_name || content.source_type || "来源"}</span>
                  <span>·</span>
                  <span>{formatDateTime(content.published_at ?? content.fetched_at)}</span>
                  <span className="mono ml-auto">{content.language || "—"}</span>
                </div>
                <div className="mt-2 min-w-0">
                  {content.id != null ? (
                    <a
                      className="block text-sm font-medium leading-6 text-foreground no-underline hover:text-blue-300"
                      href={`/dashboard/contents/${content.id}`}
                    >
                      {title}
                    </a>
                  ) : (
                    <p className="text-sm font-medium leading-6">{title}</p>
                  )}
                  <div className="mt-3 flex flex-wrap items-center gap-4 text-xs">
                    {content.id != null ? (
                      <a
                        aria-label={`阅读归档：${title}`}
                        className="text-blue-400 no-underline"
                        href={`/dashboard/contents/${content.id}`}
                      >
                        阅读归档
                      </a>
                    ) : null}
                    {content.canonical_url ? (
                      <a
                        aria-label="访问原站"
                        className="flex shrink-0 items-center gap-1 text-muted-foreground no-underline hover:text-foreground"
                        href={content.canonical_url}
                        rel="noreferrer"
                        target="_blank"
                      >
                        访问原站 <ExternalLink className="h-3 w-3" />
                      </a>
                    ) : null}
                    {canManage && content.id != null && onDelete ? (
                      <Button
                        aria-label={`删除内容：${title}`}
                        className="h-auto gap-1 px-0 py-0 text-destructive hover:bg-transparent hover:text-destructive"
                        disabled={deletingContentID === content.id}
                        onClick={() => onDelete(content)}
                        variant="ghost"
                      >
                        <Trash2 className="h-3 w-3" />
                        删除
                      </Button>
                    ) : null}
                  </div>
                </div>
                </article>
              );
            })}
          </div>
        ) : (
          <div className="flex min-h-48 flex-1 flex-col items-center justify-center px-5 text-center">
            <FileSearch className="mb-3 h-5 w-5 text-muted-foreground" />
            <p className="text-sm font-medium">暂时没有已入库内容</p>
            <p className="mt-1 max-w-md text-xs leading-5 text-muted-foreground">
              {failed > 0
                ? "已有采集失败批次，请先根据上方错误码检查来源。"
                : runs.length > 0
                  ? "采集任务已创建，内容会在标准化完成后出现在这里。"
                  : "等待监控发布并生成第一条采集批次。"}
            </p>
          </div>
        )}
        {contentsPagination ? (
          <CursorPagination {...contentsPagination} />
        ) : null}
        </section>
      </div>
    </div>
  );
}
