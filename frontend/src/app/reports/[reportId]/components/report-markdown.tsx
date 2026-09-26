import type { ReactNode } from "react";

const INLINE_LINK =
  /\[((?:\\.|[^\[\]])+|\[[^\]]+\])\]\(<([^>\n]+)>\)|\[(c[1-9][0-9]*)\]/g;

function unescapeMarkdown(value: string): string {
  return value.replace(/\\([\\`*_\[\]<>])/g, "$1");
}

function safeHttpUrl(url: string | null | undefined): string | null {
  if (!url) return null;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" || parsed.protocol === "http:"
      ? parsed.href
      : null;
  } catch {
    return null;
  }
}

function inlineContent(
  source: string,
  citations: Map<string, string | null>,
): ReactNode[] {
  const parts: ReactNode[] = [];
  let offset = 0;
  for (const match of source.matchAll(INLINE_LINK)) {
    const index = match.index ?? 0;
    if (index > offset)
      parts.push(unescapeMarkdown(source.slice(offset, index)));
    const label = match[1];
    const citation = match[3];
    const url = safeHttpUrl(label ? match[2] : citations.get(citation));
    const text = label ? unescapeMarkdown(label) : `[${citation}]`;
    parts.push(
      url ? (
        <a
          key={index}
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="break-all underline underline-offset-4"
        >
          {text}
        </a>
      ) : (
        <span key={index}>{text}</span>
      ),
    );
    offset = index + match[0].length;
  }
  if (offset < source.length)
    parts.push(unescapeMarkdown(source.slice(offset)));
  return parts;
}

export function ReportMarkdown({
  report,
}: {
  report: HotKeyAPI.ReportDetailView;
}) {
  const citations = new Map(
    report.citations.map((citation) => [
      citation.citation,
      safeHttpUrl(citation.url),
    ]),
  );
  return (
    <article className="mt-10 min-w-0 space-y-3 break-words">
      {report.body_markdown.split("\n").map((line, index) => {
        const text = line.trim();
        if (!text) return null;
        if (text.startsWith("# ")) {
          return (
            <h2 key={index} className="pt-4 text-2xl font-semibold">
              {inlineContent(text.slice(2), citations)}
            </h2>
          );
        }
        if (text.startsWith("## ")) {
          return (
            <h3 key={index} className="pt-4 text-xl font-semibold">
              {inlineContent(text.slice(3), citations)}
            </h3>
          );
        }
        if (text.startsWith("> ")) {
          return (
            <blockquote
              key={index}
              className="text-muted-foreground border-l-2 pl-4 text-sm leading-6"
            >
              {inlineContent(text.slice(2), citations)}
            </blockquote>
          );
        }
        const ordered = /^(\d+)\.\s+(.+)$/.exec(text);
        if (ordered) {
          return (
            <p key={index} className="pl-2 leading-7">
              <span className="mr-2">{ordered[1]}.</span>
              {inlineContent(ordered[2], citations)}
            </p>
          );
        }
        if (text.startsWith("- ")) {
          return (
            <p
              key={index}
              className={`leading-7 ${line.startsWith("   ") ? "pl-8" : "pl-2"}`}
            >
              <span className="mr-2">•</span>
              {inlineContent(text.slice(2), citations)}
            </p>
          );
        }
        return (
          <p key={index} className="leading-7">
            {inlineContent(text, citations)}
          </p>
        );
      })}
    </article>
  );
}
