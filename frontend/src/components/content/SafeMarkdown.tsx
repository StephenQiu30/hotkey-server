import { memo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { cn } from "@/lib/utils";
import {
  SafeExternalLink,
  toSafeExternalURL,
} from "@/components/content/SafeExternalLink";

export const MAX_SAFE_MARKDOWN_LENGTH = 500_000;

type SafeMarkdownProps = {
  className?: string;
  markdown?: string | null;
  maxLength?: number;
};

function safeLengthLimit(requested?: number) {
  if (requested == null || !Number.isFinite(requested) || requested <= 0) {
    return MAX_SAFE_MARKDOWN_LENGTH;
  }
  return Math.min(Math.floor(requested), MAX_SAFE_MARKDOWN_LENGTH);
}

function truncateAtCodePoint(markdown: string, limit: number) {
  let visible = markdown.slice(0, limit);
  const lastCodeUnit = visible.charCodeAt(visible.length - 1);
  if (lastCodeUnit >= 0xd800 && lastCodeUnit <= 0xdbff) {
    visible = visible.slice(0, -1);
  }
  return visible;
}

function RemoteImagePlaceholder({
  alt,
  src,
}: {
  alt?: string;
  src?: string | Blob;
}) {
  const description = alt?.trim() || "未提供图片说明";
  const safeURL = toSafeExternalURL(
    typeof src === "string" ? src : undefined,
  );

  return (
    <span
      aria-label={`远程图片已阻止：${description}`}
      className="my-5 flex flex-col gap-2 rounded-lg border border-border bg-muted/35 px-4 py-3 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
      role="group"
    >
      <span>
        <span className="block font-medium text-foreground">远程图片已阻止</span>
        <span className="mt-1 block text-xs">{description}</span>
      </span>
      {safeURL ? (
        <SafeExternalLink
          aria-label={`在新标签页打开图片：${description}`}
          className="shrink-0 text-xs font-medium text-foreground underline underline-offset-4"
          href={safeURL}
        >
          显式打开图片
        </SafeExternalLink>
      ) : null}
    </span>
  );
}

const SAFE_MARKDOWN_COMPONENTS: Components = {
  a: ({ children, href, node: _node, ...props }) => (
    <SafeExternalLink {...props} href={href}>
      {children}
    </SafeExternalLink>
  ),
  img: ({ alt, node: _node, src }) => (
    <RemoteImagePlaceholder alt={alt} src={src} />
  ),
  table: ({ children, node: _node, ...props }) => (
    <div className="document-table-scroll">
      <table {...props}>{children}</table>
    </div>
  ),
};

const GFM_PLUGINS = [remarkGfm];
const safeURLTransform = (url: string) => toSafeExternalURL(url) ?? "";

export const SafeMarkdown = memo(function SafeMarkdown({
  className,
  markdown = "",
  maxLength,
}: SafeMarkdownProps) {
  const limit = safeLengthLimit(maxLength);
  const source = markdown ?? "";
  const truncated = source.length > limit;
  const visibleMarkdown = truncated
    ? truncateAtCodePoint(source, limit)
    : source;

  return (
    <div className={cn("document-markdown", className)}>
      <ReactMarkdown
        components={SAFE_MARKDOWN_COMPONENTS}
        remarkPlugins={GFM_PLUGINS}
        skipHtml
        urlTransform={safeURLTransform}
      >
        {visibleMarkdown}
      </ReactMarkdown>
      {truncated ? (
        <p
          className="mt-6 border-t border-border pt-4 text-xs text-muted-foreground"
          role="status"
        >
          正文过长，仅展示前 {limit.toLocaleString("zh-CN")} 个字符。请访问原文核对其余内容。
        </p>
      ) : null}
    </div>
  );
});
