import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SearchPage from "@/app/dashboard/search/page";

const mocks = vi.hoisted(() => ({ postSearch: vi.fn() }));

vi.mock("@/services/hotkey/hotkey-server/search", () => ({
  postSearch: mocks.postSearch,
}));

describe("SearchPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.postSearch.mockResolvedValue({
      data: {
        query: "Claude",
        searched_at: "2026-08-13T12:00:00Z",
        results: [
          {
            source_type: "hacker_news",
            source_name: "Hacker News",
            external_id: "42",
            content_type: "article",
            title: "Claude 发布实时 API",
            summary: "Anthropic 公布了新的实时能力。",
            canonical_url: "https://example.com/claude",
            author: "alice",
            published_at: "2026-08-13T11:00:00Z",
            discovered_at: "2026-08-13T12:00:00Z",
            heat_score: 36.4,
            quality_state: "unavailable",
            relevance: 100,
            relevance_reason: "标题或摘要与搜索词直接匹配",
            keyword_mentioned: true,
            importance: "medium",
            metrics: { like_count: 12, comment_count: 5 },
          },
        ],
        source_statuses: [
          {
            source_type: "hacker_news",
            source_name: "Hacker News",
            state: "success",
            result_count: 1,
          },
          {
            source_type: "x",
            source_name: "X / Twitter",
            state: "failed",
            result_count: 0,
            error_code: "rate_limited",
          },
          {
            source_type: "duckduckgo",
            state: "unavailable",
            result_count: 0,
            error_code: "not_configured",
          },
        ],
      },
    });
  });

  it("searches configured sources and renders one flat hotspot card with explicit source states", async () => {
    const user = userEvent.setup();
    render(<SearchPage />);

    await user.type(screen.getByLabelText("搜索词"), "Claude");
    await user.click(screen.getByRole("button", { name: "立即搜索" }));

    await waitFor(() =>
      expect(mocks.postSearch).toHaveBeenCalledWith({
        query: "Claude",
        limit: 50,
      })
    );
    expect(await screen.findByRole("heading", { name: "Claude 发布实时 API" })).toBeInTheDocument();
    expect(screen.getByText("Anthropic 公布了新的实时能力。")).toBeInTheDocument();
    expect(screen.getByText("热度 36.4")).toBeInTheDocument();
    expect(screen.getByText("相关性 100%")).toBeInTheDocument();
    expect(screen.getByText("请求受限")).toBeInTheDocument();
    expect(screen.getByText("未配置")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看原文" })).toHaveAttribute(
      "href",
      "https://example.com/claude"
    );
  });

  it("supports selecting a bounded source subset without creating a monitor", async () => {
    const user = userEvent.setup();
    render(<SearchPage />);

    await user.type(screen.getByLabelText("搜索词"), "Claude");
    await user.click(screen.getByRole("button", { name: "选择来源" }));
    await user.click(screen.getByRole("button", { name: "清空" }));
    await user.click(screen.getByRole("checkbox", { name: "Hacker News" }));
    await user.click(screen.getByRole("button", { name: "立即搜索" }));

    await waitFor(() =>
      expect(mocks.postSearch).toHaveBeenCalledWith({
        query: "Claude",
        limit: 50,
        source_types: ["hacker_news"],
      })
    );
  });
});
