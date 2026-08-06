"use client";

import { ExternalLink, FileDown, FileText, Trash2 } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

type ContentDocumentViewerProps = {
  document: HotKeyAPI.ContentDocumentResponse;
  canManage?: boolean;
  deleting?: boolean;
  onDelete?: () => void;
};

const formatDateTime = (value?: string) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(value))
    : "—";

export function ContentDocumentViewer({ document, canManage = false, deleting = false, onDelete }: ContentDocumentViewerProps) {
  const ready = document.availability === "ready";

  return (
    <article className="document-print-root mx-auto w-full max-w-[920px]">
      <header className="document-header border-b border-border pb-7">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="eyebrow">Archived content</p>
              <Badge variant="outline" className={ready ? "success-text" : "text-muted-foreground"}>
                {ready ? "已归档" : "未归档"}
              </Badge>
            </div>
            <h1 className="mt-3 text-2xl font-semibold sm:text-3xl">
              {document.title || `归档内容 #${document.content_id ?? "—"}`}
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">
              {document.source_name || "未知来源"} · 发布于 {formatDateTime(document.published_at)}
            </p>
          </div>
          <div className="document-actions flex shrink-0 flex-wrap gap-2">
            {document.canonical_url ? (
              <Button asChild variant="outline" className="gap-2">
                <a
                  aria-label="访问原站"
                  href={document.canonical_url}
                  rel="noreferrer"
                  target="_blank"
                >
                  访问原站 <ExternalLink className="h-3.5 w-3.5" />
                </a>
              </Button>
            ) : null}
            <Button
              className="gap-2"
              disabled={!ready}
              onClick={() => window.print()}
            >
              <FileDown className="h-4 w-4" />
              打印 / 保存 PDF
            </Button>
            {canManage && document.content_id != null && onDelete ? (
              <Button
                aria-label="删除内容"
                className="gap-2"
                disabled={deleting}
                onClick={onDelete}
                variant="destructive"
              >
                <Trash2 className="h-4 w-4" />
                删除内容
              </Button>
            ) : null}
          </div>
        </div>

        <div className="document-scope mt-6 rounded-md border border-blue-500/25 bg-blue-500/[0.06] px-4 py-3 text-xs leading-5 text-blue-200/80">
          仅包含来源 Feed 实际提供并获准归档的正文或摘要；系统不会抓取原网页，也不代表完整论文或付费内容。
        </div>
        {document.canonical_url ? (
          <p className="document-canonical mono mt-3 break-all text-[11px] text-muted-foreground">
            原始地址：{document.canonical_url}
          </p>
        ) : null}
      </header>

      {ready ? (
        <div className="document-markdown py-8">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              a: ({ children, node: _node, ...props }) => (
                <a {...props} rel="noreferrer" target="_blank">
                  {children}
                </a>
              ),
              table: ({ children, node: _node, ...props }) => (
                <div className="document-table-scroll">
                  <table {...props}>{children}</table>
                </div>
              ),
            }}
          >
            {document.markdown || ""}
          </ReactMarkdown>
        </div>
      ) : (
        <div className="panel my-8 flex min-h-64 flex-col items-center justify-center px-6 text-center">
          <FileText className="mb-4 h-6 w-6 text-muted-foreground" />
          <h2 className="text-base font-medium">本条未归档正文/摘要</h2>
          <p className="mt-2 max-w-lg text-sm leading-6 text-muted-foreground">
            来源可能没有在 Feed 中提供正文，或采集时未开启正文归档授权。标题和原站地址不会被伪装成正文。
          </p>
        </div>
      )}

      <footer className="document-footer border-t border-border py-5 text-xs text-muted-foreground">
        <span>归档时间：{formatDateTime(document.captured_at)}</span>
        {document.sha256 ? <span className="mono ml-4">SHA-256 {document.sha256.slice(0, 12)}…</span> : null}
      </footer>
    </article>
  );
}
