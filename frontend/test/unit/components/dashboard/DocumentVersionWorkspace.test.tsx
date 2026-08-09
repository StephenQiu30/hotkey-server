import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HotKeyAPIError } from "@/lib/request";

const mocks = vi.hoisted(() => ({
  getCitation: vi.fn(),
  getDocument: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/documentVersions", () => ({
  getDocumentVersionsIdCitation: mocks.getCitation,
  getDocumentVersionsIdDocument: mocks.getDocument,
}));

import { DocumentVersionWorkspace } from "@/components/dashboard/DocumentVersionWorkspace";

const citation = {
  document_id: 4,
  document_version_id: 9,
  title: "OpenAI 发布新模型",
  source_name: "OpenAI Feed",
  source_type: "rss",
  author: "OpenAI",
  availability: "full_archive",
  body_origin: "feed_content",
  completeness: "full",
  canonical_url: "https://openai.example/release",
  source_record_url: "https://feed.example/item/9",
  discussion_url: "https://forum.example/t/9",
  published_at: "2026-08-09T08:00:00Z",
  captured_at: "2026-08-09T08:05:00Z",
  locator_availability: "unavailable",
  locator_unavailable_reason: "anchor_map_unavailable",
  artifact: {
    artifact_type: "markdown",
    mime_type: "text/markdown",
    sha256: "b".repeat(64),
    etag: '"' + "b".repeat(64) + '"',
    size_bytes: 40,
    transformer_profile_sha256: "c".repeat(64),
  },
} satisfies HotKeyAPI.CitationResponseDTO;

describe("DocumentVersionWorkspace", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getCitation.mockResolvedValue({ data: citation });
    mocks.getDocument.mockResolvedValue({
      data: { citation, etag: citation.artifact?.etag, markdown: "# 正式发布\n\n正文内容" },
    });
  });

  it("renders an exact immutable Markdown version with explicit provenance", async () => {
    render(<DocumentVersionWorkspace documentVersionID={9} />);

    expect(await screen.findByRole("heading", { name: "正式发布" })).toBeInTheDocument();
    expect(screen.getByText("正文内容")).toBeInTheDocument();
    expect(screen.getByText("OpenAI Feed")).toBeInTheDocument();
    expect(screen.getByText("feed_content · full")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "发布页" })).toHaveAttribute(
      "href",
      "https://openai.example/release",
    );
    expect(screen.getByRole("link", { name: "来源记录" })).toHaveAttribute(
      "href",
      "https://feed.example/item/9",
    );
    expect(screen.getByRole("link", { name: "讨论页" })).toHaveAttribute(
      "href",
      "https://forum.example/t/9",
    );
    expect(screen.getByText("anchor_map_unavailable")).toBeInTheDocument();
    expect(screen.queryByText(/可信|已证实|真实性评分/)).not.toBeInTheDocument();
  });

  it("keeps safe citation facts visible when the body is policy blocked", async () => {
    mocks.getCitation.mockResolvedValueOnce({
      data: {
        ...citation,
        availability: "policy_blocked",
        unavailable_reason: "display_private_denied",
        artifact: undefined,
        content_sha256: undefined,
      },
    });
    mocks.getDocument.mockRejectedValueOnce(new HotKeyAPIError(403, "正文不可读取"));
    render(<DocumentVersionWorkspace documentVersionID={9} />);

    expect(await screen.findByText("OpenAI 发布新模型")).toBeInTheDocument();
    expect(screen.getByText("正文当前不可读取")).toBeInTheDocument();
    expect(screen.getByText("display_private_denied")).toBeInTheDocument();
    expect(screen.queryByText("正文内容")).not.toBeInTheDocument();
    expect(screen.queryByText("b".repeat(64))).not.toBeInTheDocument();
  });

  it("refuses to render Markdown returned for another document version", async () => {
    mocks.getDocument.mockResolvedValueOnce({
      data: {
        citation: { ...citation, document_version_id: 10 },
        markdown: "# 不应显示",
      },
    });
    render(<DocumentVersionWorkspace documentVersionID={9} />);

    expect(await screen.findByText("正文响应与请求版本不一致")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "不应显示" })).not.toBeInTheDocument();
  });
});
