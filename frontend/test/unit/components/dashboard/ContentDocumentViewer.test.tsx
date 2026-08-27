import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ContentDocumentViewer } from "@/components/dashboard/ContentDocumentViewer";

const readyDocument = {
  availability: "ready",
  content_id: 7,
  title: "Archived item title",
  source_name: "arXiv · Artificial Intelligence",
  canonical_url: "https://example.test/papers/7",
  published_at: "2026-07-18T04:00:00Z",
  captured_at: "2026-07-18T04:01:00Z",
  language: "en",
  sha256: "a".repeat(64),
  markdown: [
    "# A durable agent workflow",
    "",
    "- first fact",
    "- second fact",
    "",
    "| Signal | Value |",
    "| --- | ---: |",
    "| Heat | 42 |",
    "",
    "```ts",
    "const safe = true;",
    "```",
    "",
    "<script>window.__unsafeMarkdown = true</script>",
  ].join("\n"),
} as const;

describe("ContentDocumentViewer", () => {
  beforeEach(() => {
    window.print = vi.fn();
  });

  it("renders archived GFM safely with source provenance and print action", () => {
    const { container } = render(
      <ContentDocumentViewer document={readyDocument} />,
    );

    expect(
      screen.getByRole("heading", { name: "A durable agent workflow" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Archived item title" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByText("const safe = true;")).toBeInTheDocument();
    expect(container.querySelector("script")).toBeNull();
    expect((window as typeof window & { __unsafeMarkdown?: boolean }).__unsafeMarkdown).toBeUndefined();
    expect(
      screen.getByText(/仅包含来源 Feed 实际提供并获准归档的正文或摘要/),
    ).toBeInTheDocument();
    const sourceLink = screen.getByRole("link", { name: "访问原站" });
    expect(sourceLink).toHaveAttribute(
      "href",
      "https://example.test/papers/7",
    );
    expect(sourceLink).toHaveAttribute(
      "rel",
      "noopener noreferrer",
    );
    expect(sourceLink.querySelector("button")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "打印 / 保存 PDF" }));
    expect(window.print).toHaveBeenCalledTimes(1);
  });

  it("uses the shared URL policy for document provenance", () => {
    render(
      <ContentDocumentViewer
        document={{
          ...readyDocument,
          canonical_url: "javascript:alert(1)",
        }}
      />,
    );

    expect(
      screen.queryByRole("link", { name: "访问原站" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/javascript:alert/)).not.toBeInTheDocument();
  });

  it("keeps not-captured content honest and disables printing", () => {
    render(
      <ContentDocumentViewer
        document={{
          availability: "not_captured",
          content_id: 8,
          source_name: "Science · Current Issue",
          canonical_url: "https://example.test/papers/8",
        }}
      />,
    );

    expect(screen.getByText("本条未归档正文/摘要")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "打印 / 保存 PDF" })).toBeDisabled();
    expect(screen.getByRole("link", { name: "访问原站" })).toHaveAttribute(
      "href",
      "https://example.test/papers/8",
    );
  });

  it("labels an absent publication time without substituting capture time", () => {
    render(
      <ContentDocumentViewer
        document={{
          ...readyDocument,
          published_at: undefined,
        }}
      />,
    );

    expect(screen.getByText(/发布时间未知/)).toBeInTheDocument();
    expect(screen.queryByText(/发布于 —/)).not.toBeInTheDocument();
  });

  it.each([
    ["missing", "归档证据缺失"],
    ["deleting", "归档证据正在清理"],
    ["read_failed", "归档证据暂时无法读取"],
    ["integrity_failed", "归档证据完整性校验失败"],
  ] as const)("shows unavailable reason %s without rendering fallback body", (reason, copy) => {
    render(
      <ContentDocumentViewer
        document={{
          availability: "unavailable",
          unavailable_reason: reason,
          content_id: 9,
          title: "Only metadata",
          markdown: "must not render",
        }}
      />,
    );

    expect(screen.getByText(copy)).toBeInTheDocument();
    expect(screen.queryByText("must not render")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "打印 / 保存 PDF" })).toBeDisabled();
  });

  it("shows the management action only when the caller has permission", async () => {
    const onDelete = vi.fn();
    const { rerender } = render(
      <ContentDocumentViewer
        canManage
        document={readyDocument}
        onDelete={onDelete}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "删除内容" }));
    expect(onDelete).toHaveBeenCalledTimes(1);

    rerender(<ContentDocumentViewer document={readyDocument} />);
    expect(screen.queryByRole("button", { name: "删除内容" })).not.toBeInTheDocument();
  });
});
