import { ExternalLink, FileText, RefreshCw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

type EventEvidenceTimelineProps = {
  contents: HotKeyAPI.ContentResponse[];
  totalCount: number;
  failedCount: number;
  unavailable?: boolean;
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

const contentTitle = (content: HotKeyAPI.ContentResponse) =>
  content.title || content.external_id || `内容 #${content.id ?? "—"}`;

export function EventEvidenceTimeline({
  contents,
  totalCount,
  failedCount,
  unavailable = false,
}: EventEvidenceTimelineProps) {
  const countDescription = failedCount > 0
    ? `已读取 ${contents.length} 条，${failedCount} 条暂不可读`
    : `已读取 ${contents.length} 条，共 ${totalCount} 条`;

  return (
    <section aria-labelledby="event-evidence-heading">
      <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-6 sm:px-7">
        <div>
          <div className="flex items-baseline gap-2">
            <h2 id="event-evidence-heading" className="text-base font-semibold text-slate-950">
              证据验证
            </h2>
            <span className="text-xs text-muted-foreground">（{totalCount} 条）</span>
          </div>
          <p className="mt-2 text-xs leading-5 text-muted-foreground">
            按发布时间呈现公开来源，帮助核对事件依据
          </p>
          <p className={`mt-1 text-xs ${failedCount > 0 ? "text-amber-700" : "text-emerald-700"}`}>
            {countDescription}
          </p>
        </div>
        <Button asChild variant="ghost" size="sm" className="h-auto shrink-0 px-2 text-xs text-blue-700 hover:bg-blue-50 hover:text-blue-800">
          <a href="#event-evidence-list">全部证据</a>
        </Button>
      </div>

      {unavailable ? (
        <div className="flex min-h-56 flex-col items-center justify-center px-8 text-center">
          <FileText className="mb-3 h-5 w-5 text-muted-foreground" />
          <p className="text-sm font-medium">证据成员暂时无法加载</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">事件本身仍可查看，稍后刷新可重试证据列表。</p>
        </div>
      ) : contents.length ? (
        <div id="event-evidence-list" className="divide-y divide-border px-5 sm:px-7">
          {contents.map((content, index) => {
            const title = contentTitle(content);
            return (
              <article
                key={content.id ?? `${content.external_id ?? title}-${index}`}
                className="grid grid-cols-[28px_minmax(0,1fr)] gap-3 py-5"
              >
                <Badge variant="secondary" className="mono flex h-6 w-6 items-center justify-center rounded-full border-0 bg-blue-50 p-0 text-[10px] font-semibold text-blue-700">
                  {index + 1}
                </Badge>
                <div className="min-w-0">
                  <div className="flex min-w-0 items-start gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="truncate text-sm font-medium text-slate-900">
                          {content.source_name || content.source_type || "公开来源"}
                        </span>
                        <span className="mono text-[10px] text-muted-foreground">
                          {formatDateTime(content.published_at)}
                        </span>
                      </div>
                    </div>
                    <Badge variant="secondary" className="shrink-0 rounded-md border-0 bg-emerald-50 px-2 py-1 text-[10px] font-medium text-emerald-700">
                      可读
                    </Badge>
                  </div>
                  <p className="mt-2 line-clamp-2 text-xs leading-5 text-slate-600">{title}</p>
                  <div className="mt-3 flex flex-wrap items-center gap-4 text-[11px]">
                    {content.id != null ? (
                      <Button asChild variant="link" className="h-auto p-0 text-[11px] font-medium text-blue-700 no-underline hover:text-blue-800">
                        <a aria-label={`阅读归档：${title}`} href={`/dashboard/contents/${content.id}`}>阅读归档</a>
                      </Button>
                    ) : null}
                    {content.canonical_url ? (
                      <Button asChild variant="link" className="h-auto gap-1 p-0 text-[11px] text-muted-foreground no-underline hover:text-foreground">
                        <a aria-label={`访问原站：${title}`} href={content.canonical_url} rel="noreferrer" target="_blank">
                          访问原站 <ExternalLink className="h-3 w-3" />
                        </a>
                      </Button>
                    ) : null}
                    <span className="ml-auto text-muted-foreground">
                      互动 {(content.metrics?.like_count ?? 0) + (content.metrics?.comment_count ?? 0) + (content.metrics?.share_count ?? 0)}
                    </span>
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      ) : (
        <div className="flex min-h-56 flex-col items-center justify-center px-8 text-center">
          <FileText className="mb-3 h-5 w-5 text-muted-foreground" />
          <p className="text-sm font-medium">
            {totalCount > 0 ? "该事件有证据成员，但详情暂不可读。" : "该事件暂时没有内容证据。"}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">采集完成后，证据会按时间显示在这里。</p>
        </div>
      )}

      <div className="flex items-center justify-between border-t border-border px-5 py-4 text-[11px] text-muted-foreground sm:px-7">
        <span>显示 {contents.length} 条，共 {totalCount} 条</span>
        <span className="inline-flex items-center gap-1.5"><RefreshCw className="h-3 w-3" />随工作台刷新</span>
      </div>
    </section>
  );
}
