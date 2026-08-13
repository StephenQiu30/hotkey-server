import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ContentsPage from "@/app/dashboard/contents/page";

const mocks = vi.hoisted(() => ({
  getHotspots: vi.fn(),
  getSourceConnections: vi.fn(),
  getMonitors: vi.fn(),
  routerReplace: vi.fn(),
  navigationQuery: "",
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(mocks.navigationQuery),
  useRouter: () => ({ replace: mocks.routerReplace }),
}));
vi.mock("@/services/hotkey/hotkey-server/hotspots", () => ({
  getHotspots: mocks.getHotspots,
}));
vi.mock("@/services/hotkey/hotkey-server/sources", () => ({
  getSourceConnections: mocks.getSourceConnections,
}));
vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
}));

const card = {
  id: 7,
  source_type: "rss",
  source_name: "官方动态",
  external_id: "release-7",
  content_type: "article",
  title: "Claude 发布实时 API",
  summary: "Anthropic 公布了新的实时能力。",
  canonical_url: "https://example.test/release",
  author: "Alice",
  published_at: "2026-08-13T11:00:00Z",
  discovered_at: "2026-08-13T12:00:00Z",
  heat_score: 36.4,
  quality_state: "unavailable" as const,
  relevance: 88,
  relevance_reason: "当前监控的最近一次相关性判断",
  importance: "medium" as const,
  metrics: { like_count: 12 },
};

describe("HotspotRadar", () => {
  beforeEach(() => {
    mocks.navigationQuery = "";
    mocks.getSourceConnections.mockResolvedValue({
      data: { items: [{ id: 3, name: "官方动态", deleted: false }] },
    });
    mocks.getMonitors.mockResolvedValue({
      data: {
        items: [
          { id: 5, name: "Claude", status: "active" },
          { id: 6, name: "暂停项", status: "paused" },
        ],
      },
    });
    mocks.getHotspots.mockResolvedValue({
      data: {
        items: [card],
        summary: { total: 12, today: 3, urgent: 1 },
        next_cursor: "next-1",
      },
    });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders server statistics and the same flat hotspot card as instant search", async () => {
    render(<ContentsPage />);

    expect(
      await screen.findByRole("heading", { name: card.title })
    ).toBeInTheDocument();
    expect(mocks.getHotspots).toHaveBeenCalledWith({
      limit: 20,
      sort: "discovered",
    });
    expect(screen.getByText("总热点").nextElementSibling).toHaveTextContent(
      "12"
    );
    expect(screen.getByText("今日新增").nextElementSibling).toHaveTextContent(
      "3"
    );
    expect(screen.getByText("紧急热点").nextElementSibling).toHaveTextContent(
      "1"
    );
    await waitFor(() =>
      expect(screen.getByText("启用监控").nextElementSibling).toHaveTextContent(
        "1"
      )
    );
    expect(screen.getByText("热度 36.4")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看原文" })).toHaveAttribute(
      "href",
      card.canonical_url
    );
  });

  it("restores combined filters and delegates relevance ordering to the server", async () => {
    mocks.navigationQuery =
      "q=Claude&source=3&monitor=5&from=2026-08-01&to=2026-08-13&sort=relevance&limit=50";
    render(<ContentsPage />);

    await waitFor(() =>
      expect(mocks.getHotspots).toHaveBeenCalledWith({
        limit: 50,
        q: "Claude",
        source_connection_id: 3,
        monitor_id: 5,
        published_from: "2026-08-01T00:00:00Z",
        published_to: "2026-08-13T23:59:59Z",
        sort: "relevance",
      })
    );
  });

  it("debounces keyword search and resets pagination in the URL", async () => {
    render(<ContentsPage />);
    const search = screen.getByRole("searchbox", { name: "搜索热点" });
    await userEvent.setup().type(search, "更新");

    await waitFor(() =>
      expect(mocks.getHotspots).toHaveBeenLastCalledWith({
        limit: 20,
        q: "更新",
        sort: "discovered",
      })
    );
    expect(mocks.routerReplace).toHaveBeenCalledWith(
      "/dashboard/contents?q=%E6%9B%B4%E6%96%B0",
      { scroll: false }
    );
  });

  it("uses the opaque server cursor for the next page", async () => {
    render(<ContentsPage />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "下一页" }));

    await waitFor(() =>
      expect(mocks.getHotspots).toHaveBeenLastCalledWith({
        cursor: "next-1",
        limit: 20,
        sort: "discovered",
      })
    );
  });

  it("shows request failures separately from a valid empty result and retries", async () => {
    mocks.getHotspots
      .mockRejectedValueOnce(new Error("hotspot unavailable"))
      .mockResolvedValueOnce({ data: { items: [], summary: {} } });
    render(<ContentsPage />);

    expect(await screen.findByText("热点加载失败")).toBeInTheDocument();
    expect(screen.queryByText("暂时没有热点")).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() => expect(mocks.getHotspots).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("暂时没有热点")).toBeInTheDocument();
  });
});
