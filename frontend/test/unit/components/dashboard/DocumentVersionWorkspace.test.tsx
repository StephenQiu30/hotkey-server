import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HotKeyAPIError } from "@/lib/request";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getCitation: vi.fn(),
  getDocument: vi.fn(),
  postTextQuoteSelector: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/documentVersions", () => ({
  getDocumentVersionsIdCitation: mocks.getCitation,
  getDocumentVersionsIdDocument: mocks.getDocument,
  postDocumentVersionsIdTextQuoteSelectors: mocks.postTextQuoteSelector,
}));

import { DocumentVersionWorkspace } from "@/components/dashboard/DocumentVersionWorkspace";

const citation = {
  document_id: 4,
  document_version_id: 9,
  title: "OpenAI 发布新模型",
  source_name: "OpenAI Feed",
  source_type: "rss",
  author: "OpenAI",
  publisher: "OpenAI, Inc.",
  publisher_availability: "available",
  publisher_party: {
    display_name: "OpenAI, Inc.",
    external_id: "openai",
    homepage_url: "https://openai.example/about",
    identity_namespace: "publisher-id",
    kind: "organization",
    role: "publisher",
  },
  content_origin_availability: "available",
  content_origin: {
    display_name: "OpenAI Newsroom",
    external_id: "newsroom",
    identity_namespace: "publisher-id",
    kind: "organization",
    role: "content_origin",
  },
  distributors: [
    {
      display_name: "Example Wire",
      external_id: "example-wire",
      homepage_url: "https://wire.example/about",
      identity_namespace: "distributor-id",
      kind: "organization",
      role: "distributor",
    },
  ],
  availability: "full_archive",
  raw_evidence: {
    availability: "available",
    payload_sha256s: ["e".repeat(64)],
    retention_until: "2026-09-08T08:05:00Z",
    deletion_audited: false,
    exception_approved: false,
  },
  body_origin: "feed_content",
  completeness: "full",
  canonical_url: "https://openai.example/release",
  source_record_url: "https://feed.example/item/9",
  discussion_url: "https://forum.example/t/9",
  published_at: "2026-08-09T08:00:00Z",
  published_utc_offset_minutes: 480,
  captured_at: "2026-08-09T08:05:00Z",
  locator_availability: "unavailable",
  locator_unavailable_reason: "exact_quote_unavailable",
  content_sha256: "a".repeat(64),
  artifact: {
    artifact_type: "markdown",
    mime_type: "text/markdown",
    sha256: "b".repeat(64),
    etag: '"' + "b".repeat(64) + '"',
    size_bytes: 40,
    transformer_profile_sha256: "c".repeat(64),
    anchor_map: {
      normalization_version: "nfc-lf-collapse-space-v1",
      anchor_map_profile_version: "commonmark-gfm-visible-blocks-v1",
      anchor_map_sha256: "d".repeat(64),
      blocks: [
        { ordinal: 0, markdown_anchor: "body-0000-000000000001" },
        { ordinal: 1, markdown_anchor: "body-0001-000000000002" },
      ],
    },
  },
} satisfies HotKeyAPI.CitationResponseDTO;

describe("DocumentVersionWorkspace", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({ user: { role: "viewer" } as HotKeyAPI.UserResponse });
    mocks.getCitation.mockResolvedValue({ data: citation });
    mocks.getDocument.mockResolvedValue({
      data: { citation, etag: citation.artifact?.etag, markdown: "# 正式发布\n\n正文内容" },
    });
    mocks.postTextQuoteSelector.mockResolvedValue({ data: { id: 77, version: 1, document_version_id: 9, exact_quote: "正文内容", utf8_byte_start: 8, utf8_byte_end: 20, markdown_anchor: "body-0001-000000000002" } });
  });

  it("renders an exact immutable Markdown version with explicit provenance", async () => {
    render(<DocumentVersionWorkspace documentVersionID={9} />);

    expect(await screen.findByRole("heading", { name: "正式发布" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "正式发布" })).toHaveAttribute(
      "id",
      "body-0000-000000000001",
    );
    expect(screen.getByText("正文内容")).toBeInTheDocument();
    expect(screen.getByText("正文内容")).toHaveAttribute("id", "body-0001-000000000002");
    expect(screen.getByText("OpenAI Feed")).toBeInTheDocument();
    expect(screen.getByText("发布者")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "OpenAI, Inc." })).toHaveAttribute(
      "href",
      "https://openai.example/about",
    );
    expect(screen.getByText("OpenAI Newsroom")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Example Wire" })).toHaveAttribute(
      "href",
      "https://wire.example/about",
    );
    expect(screen.getByText("feed_content · full")).toBeInTheDocument();
    expect(screen.getByText(/原时区 UTC\+08:00/)).toBeInTheDocument();
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
    expect(screen.getByText("exact_quote_unavailable")).toBeInTheDocument();
    expect(screen.getByText(/commonmark-gfm-visible-blocks-v1 · 2 blocks/)).toBeInTheDocument();
    expect(screen.queryByText(/可信|已证实|真实性评分/)).not.toBeInTheDocument();
  });

  it("keeps an unknown publication time distinct from capture time", async () => {
    const unknownTimeCitation = {
      ...citation,
      published_at: undefined,
      published_utc_offset_minutes: undefined,
    };
    mocks.getCitation.mockResolvedValueOnce({ data: unknownTimeCitation });
    mocks.getDocument.mockResolvedValueOnce({
      data: { citation: unknownTimeCitation, etag: citation.artifact?.etag, markdown: "# 正式发布\n\n正文内容" },
    });

    render(<DocumentVersionWorkspace documentVersionID={9} />);

    expect(await screen.findByText(/发布时间未知/)).toBeInTheDocument();
    expect(screen.getByText(/采集于/)).toBeInTheDocument();
    expect(screen.queryByText(/发布于 .*采集于/)).not.toBeInTheDocument();
  });

  it("keeps missing publisher and content-origin facts explicitly unknown", async () => {
    const unknownCitation = {
      ...citation,
      publisher: undefined,
      publisher_party: undefined,
      publisher_availability: "unavailable" as const,
      publisher_unavailable_reason: "publisher_unavailable",
      content_origin: undefined,
      content_origin_availability: "unavailable" as const,
      content_origin_unavailable_reason: "content_origin_unavailable",
      distributors: [],
    };
    mocks.getCitation.mockResolvedValueOnce({ data: unknownCitation });
    mocks.getDocument.mockResolvedValueOnce({
      data: {
        citation: unknownCitation,
        etag: unknownCitation.artifact?.etag,
        markdown: "# 正式发布\n\n正文内容",
      },
    });

    render(<DocumentVersionWorkspace documentVersionID={9} />);

    expect(await screen.findByText("发布者信息未提供")).toBeInTheDocument();
    expect(screen.getByText("内容源主体未提供")).toBeInTheDocument();
    expect(screen.getByText("分发方信息未提供")).toBeInTheDocument();
    expect(screen.queryByText("OpenAI Feed", { selector: "a" })).not.toBeInTheDocument();
  });

  it("shows an explicit expired raw-object state while retaining hash metadata", async () => {
    const expiredCitation = {
      ...citation,
      raw_evidence: {
        availability: "expired" as const,
        payload_sha256s: ["f".repeat(64)],
        retention_until: "2026-08-10T08:05:00Z",
        deletion_audited: true,
        exception_approved: false,
      },
    };
    mocks.getCitation.mockResolvedValueOnce({ data: expiredCitation });
    mocks.getDocument.mockResolvedValueOnce({
      data: { citation: expiredCitation, etag: citation.artifact?.etag, markdown: "# 正式发布\n\n正文内容" },
    });

    render(<DocumentVersionWorkspace documentVersionID={9} />);

    expect(await screen.findByText("原始对象已过保留期")).toBeInTheDocument();
    expect(screen.getByText(/ffffffffff/)).toBeInTheDocument();
    expect(screen.getByText("删除审计已记录")).toBeInTheDocument();
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

  it("lets an editor create a server-located exact quote selector", async () => {
    useAuthStore.setState({ user: { role: "editor" } as HotKeyAPI.UserResponse });
    render(<DocumentVersionWorkspace documentVersionID={9} />);

    const input = await screen.findByLabelText("精确摘录");
    await userEvent.type(input, "正文内容");
    await userEvent.click(screen.getByRole("button", { name: "生成引用选择器" }));

    expect(mocks.postTextQuoteSelector).toHaveBeenCalledWith(
      { id: 9 },
      {
        exact_quote: "正文内容",
        plaintext_sha256: "a".repeat(64),
        normalization_version: "nfc-lf-collapse-space-v1",
      },
      expect.objectContaining({
        headers: expect.objectContaining({ "If-Match": `"${"a".repeat(64)}"` }),
      }),
    );
    expect(await screen.findByText(/引用选择器 #77 已创建/)).toBeInTheDocument();
  });
});
