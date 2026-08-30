import { memo, useMemo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { cn } from "@/lib/utils";
import {
  SafeExternalLink,
  toSafeExternalURL,
} from "@/components/content/SafeExternalLink";
import { Surface } from "@/components/ui/surface";

export const MAX_SAFE_MARKDOWN_LENGTH = 500_000;

type SafeMarkdownProps = {
  anchors?: readonly SafeMarkdownAnchor[];
  className?: string;
  markdown?: string | null;
  maxLength?: number;
};

export type SafeMarkdownAnchor = {
  ordinal: number;
  markdownAnchor: string;
};

type MarkdownSyntaxNode = {
  type?: string;
  children?: MarkdownSyntaxNode[];
  data?: {
    hProperties?: Record<string, unknown>;
  };
};

const SERVER_MARKDOWN_ANCHOR_PATTERN = /^body-[0-9]{4,5}-[0-9a-f]{12}$/;
const MAX_SERVER_MARKDOWN_ANCHORS = 20_000;

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
    <Surface asChild variant="subtle">
    <span
      aria-label={`远程图片已阻止：${description}`}
      className="my-5 flex flex-col gap-2 px-4 py-3 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
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
    </Surface>
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

function safeServerMarkdownAnchors(
  anchors?: readonly SafeMarkdownAnchor[],
): readonly SafeMarkdownAnchor[] | undefined {
  if (!anchors?.length || anchors.length > MAX_SERVER_MARKDOWN_ANCHORS) {
    return undefined;
  }
  const seen = new Set<string>();
  for (let index = 0; index < anchors.length; index += 1) {
    const anchor = anchors[index];
    if (
      anchor.ordinal !== index ||
      !SERVER_MARKDOWN_ANCHOR_PATTERN.test(anchor.markdownAnchor) ||
      seen.has(anchor.markdownAnchor)
    ) {
      return undefined;
    }
    seen.add(anchor.markdownAnchor);
  }
  return anchors;
}

function collectMarkdownAnchorTargets(
  node: MarkdownSyntaxNode,
  targets: MarkdownSyntaxNode[],
) {
  for (const child of node.children ?? []) {
    switch (child.type) {
      case "heading":
      case "paragraph":
      case "code":
        targets.push(child);
        break;
      case "list":
        collectMarkdownListAnchorTargets(child, targets);
        break;
      case "table":
        for (const row of child.children ?? []) {
          if (row.type === "tableRow") targets.push(row);
        }
        break;
      case "blockquote":
      case "root":
        collectMarkdownAnchorTargets(child, targets);
        break;
      default:
        break;
    }
  }
}

function collectMarkdownListAnchorTargets(
  list: MarkdownSyntaxNode,
  targets: MarkdownSyntaxNode[],
) {
  for (const item of list.children ?? []) {
    if (item.type !== "listItem") continue;
    targets.push(item);
    for (const child of item.children ?? []) {
      if (child.type === "list") collectMarkdownListAnchorTargets(child, targets);
    }
  }
}

function createServerMarkdownAnchorPlugin(
  anchors: readonly SafeMarkdownAnchor[],
) {
  return function serverMarkdownAnchorPlugin() {
    return function applyServerMarkdownAnchors(tree: MarkdownSyntaxNode) {
      const targets: MarkdownSyntaxNode[] = [];
      collectMarkdownAnchorTargets(tree, targets);
      if (targets.length !== anchors.length) return;
      for (let index = 0; index < targets.length; index += 1) {
        const target = targets[index];
        target.data ??= {};
        target.data.hProperties = {
          ...target.data.hProperties,
          id: anchors[index].markdownAnchor,
        };
      }
    };
  };
}

export const SafeMarkdown = memo(function SafeMarkdown({
  anchors,
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
  const remarkPlugins = useMemo(() => {
    const safeAnchors = truncated ? undefined : safeServerMarkdownAnchors(anchors);
    return safeAnchors
      ? [remarkGfm, createServerMarkdownAnchorPlugin(safeAnchors)]
      : GFM_PLUGINS;
  }, [anchors, truncated]);

  return (
    <div
      className={cn(
        "document-markdown [&_:target]:scroll-mt-24 [&_:target]:rounded-md [&_:target]:bg-primary/10 [&_:target]:ring-2 [&_:target]:ring-primary/45 [&_:target]:ring-offset-4 [&_:target]:ring-offset-background",
        className,
      )}
    >
      <ReactMarkdown
        components={SAFE_MARKDOWN_COMPONENTS}
        remarkPlugins={remarkPlugins}
        skipHtml
        urlTransform={safeURLTransform}
      >
        {visibleMarkdown}
      </ReactMarkdown>
      {truncated ? (
        <p
          className="mt-6 pt-4 text-xs text-muted-foreground"
          role="status"
        >
          正文过长，仅展示前 {limit.toLocaleString("zh-CN")} 个字符。请访问原文核对其余内容。
        </p>
      ) : null}
    </div>
  );
});
