import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import EventsPage from "@/app/dashboard/events/page";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getRadarEvents: vi.fn(),
  getEventsIdUpdates: vi.fn(),
  getEventsIdIntelligence: vi.fn(),
  getEventsIdHeat: vi.fn(),
  getEventsIdContents: vi.fn(),
  postEventsIdContentsContentIdLock: vi.fn(),
  postEventsIdLifecycle: vi.fn(),
  postEventsIdMerge: vi.fn(),
  postEventsIdSplit: vi.fn(),
  getMonitors: vi.fn(),
  routerReplace: vi.fn(),
  navigationQuery: "",
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(mocks.navigationQuery),
  useRouter: () => ({ replace: mocks.routerReplace }),
}));
vi.mock("@/services/hotkey/hotkey-server/radar", () => ({
  getRadarEvents: mocks.getRadarEvents,
}));
vi.mock("@/services/hotkey/hotkey-server/events", () => ({
  getEventsIdUpdates: mocks.getEventsIdUpdates,
  getEventsIdIntelligence: mocks.getEventsIdIntelligence,
  getEventsIdHeat: mocks.getEventsIdHeat,
  getEventsIdContents: mocks.getEventsIdContents,
  postEventsIdContentsContentIdLock: mocks.postEventsIdContentsContentIdLock,
  postEventsIdLifecycle: mocks.postEventsIdLifecycle,
  postEventsIdMerge: mocks.postEventsIdMerge,
  postEventsIdSplit: mocks.postEventsIdSplit,
}));
vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
}));

describe("EventsPage", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.navigationQuery = "";
    useAuthStore.setState({
      user: { role: "viewer" } as HotKeyAPI.UserResponse,
    });
    mocks.getRadarEvents.mockResolvedValue({
      data: {
        as_of: "2026-08-04T14:30:00+08:00",
        items: [
          {
            event_id: 11,
            version: 3,
            lifecycle_status: "active",
            title_zh: "华东沿海化工园区发生爆燃事故",
            summary: "相关讨论快速扩散，权威来源正在持续更新。",
            independent_source_count: 8,
            attention: 86,
            momentum: 91,
            breadth: 75,
            data_confidence: 82,
            trend_status: "rising",
            confirmation: "corroborated",
            confirmation_score: 100,
            reason_codes: ["source_breadth_growing"],
            first_seen_at: "2026-08-04T08:17:00+08:00",
          },
          {
            event_id: 12,
            version: 2,
            lifecycle_status: "active",
            title_zh: "生成式 AI 产品迎来新一轮功能更新",
            summary: "行业讨论度持续增长。",
            independent_source_count: 4,
            trend_status: "stable",
          },
        ],
      },
    });
    mocks.getEventsIdUpdates.mockResolvedValue({
      data: {
        items: [
          {
            id: 4,
            kind: "evidence_added",
            summary: "新增 3 条独立来源",
            observed_at: "2026-08-04T10:24:00+08:00",
          },
        ],
      },
    });
    mocks.getEventsIdIntelligence.mockResolvedValue({
      data: {
        event_id: 11,
        claims: [
          {
            id: 21,
            normalized_claim: "园区已启动应急响应",
            status: "single_source",
            confidence: 46,
            evidence: [
              {
                content_id: 31,
                excerpt: "当地应急部门通报已启动响应。",
                stance: "supports",
                confidence: 52,
              },
            ],
          },
        ],
      },
    });
    mocks.getEventsIdHeat.mockResolvedValue({
      data: {
        event_id: 11,
        heat_score: 86,
        trend_score: 24,
        trend_status: "rising",
        window_hours: 24,
        heat_version: "heat-v1",
        components: {
          independence: 75,
          content_velocity: 82,
          source_breadth: 70,
          recency: 96,
          credibility: 80,
        },
      },
    });
    mocks.getEventsIdContents.mockResolvedValue({
      data: {
        items: [
          {
            id: 41,
            version: 2,
            event_id: 11,
            content_id: 31,
            membership_score: 91,
            evidence_role: "primary",
            origin: "rule",
            manual_locked: false,
          },
        ],
      },
    });
    mocks.postEventsIdContentsContentIdLock.mockResolvedValue({
      data: {
        id: 41,
        version: 3,
        event_id: 11,
        content_id: 31,
        manual_locked: true,
      },
    });
    mocks.postEventsIdLifecycle.mockResolvedValue({
      data: { id: 11, version: 4, lifecycle_status: "cooling" },
    });
    mocks.postEventsIdMerge.mockResolvedValue({ data: {} });
    mocks.postEventsIdSplit.mockResolvedValue({ data: {} });
    mocks.getMonitors.mockResolvedValue({
      data: {
        items: [
          { id: 7, name: "安全事故", status: "active" },
          { id: 8, name: "已归档监控", status: "archived" },
        ],
      },
    });
  });

  it("sends URL search filters to Radar instead of filtering the current page", async () => {
    mocks.navigationQuery =
      "q=%E5%8C%96%E5%B7%A5&window=7d&monitor=7&sort=relevance&lifecycle=active&trend=rising&verification=corroborated&min_heat=70";
    render(<EventsPage />);

    await waitFor(() =>
      expect(mocks.getRadarEvents).toHaveBeenCalledWith({
        q: "化工",
        window: "7d",
        monitor_id: 7,
        lifecycle: ["active"],
        trend: ["rising"],
        verification: ["corroborated"],
        min_heat: 70,
        sort: "relevance",
        limit: 50,
      })
    );
    expect(screen.getByRole("link", { name: "在内容中搜索" })).toHaveAttribute(
      "href",
      "/dashboard/contents?q=%E5%8C%96%E5%B7%A5"
    );
  });

  it("debounces event search and persists it in the URL", async () => {
    render(<EventsPage />);
    const search = screen.getByRole("searchbox", { name: "搜索事件" });
    await userEvent.setup().type(search, "发布");

    await waitFor(() =>
      expect(mocks.getRadarEvents).toHaveBeenLastCalledWith({
        window: "24h",
        sort: "momentum",
        limit: 50,
        q: "发布",
      })
    );
    expect(mocks.routerReplace).toHaveBeenCalledWith(
      "/dashboard/events?q=%E5%8F%91%E5%B8%83",
      { scroll: false }
    );
  });

  it("filters Radar and opens a real event-change detail panel", async () => {
    render(<EventsPage />);

    expect(
      await screen.findByRole("heading", { name: "事件动态" })
    ).toBeInTheDocument();
    expect(
      screen.getByText("华东沿海化工园区发生爆燃事故")
    ).toBeInTheDocument();
    expect(await screen.findByText("新增 3 条独立来源")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "为什么值得关注" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: "热点事件列表" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "事件" })
    ).toBeInTheDocument();
    expect(screen.getByText("证据确认，不等于真假裁决")).toBeInTheDocument();
    expect(screen.getByText("重要性")).toBeInTheDocument();
    expect(screen.getByText("园区已启动应急响应")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /查看证据内容 31/ })
    ).toHaveAttribute("href", "/dashboard/contents/31");
    expect(screen.getByText("46 · 低置信度")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "热度与趋势" })
    ).toBeInTheDocument();
    expect(screen.getByText("互动表现不可用")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "聚类与治理" })
    ).toBeInTheDocument();
    expect(
      screen.getByText("当前为只读视图；治理操作仅向编辑者和管理员开放。")
    ).toBeInTheDocument();
    expect(mocks.getRadarEvents).toHaveBeenCalledWith({
      window: "24h",
      sort: "momentum",
      limit: 50,
    });
    expect(mocks.getEventsIdIntelligence).toHaveBeenCalledWith({ id: 11 });
    expect(mocks.getEventsIdHeat).toHaveBeenCalledWith({ id: 11 });
    expect(mocks.getEventsIdContents).toHaveBeenCalledWith({ id: 11 });

    fireEvent.click(screen.getByRole("button", { name: /生成式 AI 产品/ }));

    expect(mocks.getEventsIdUpdates).toHaveBeenLastCalledWith({
      id: 12,
      limit: 20,
    });
    expect(mocks.getEventsIdIntelligence).toHaveBeenLastCalledWith({ id: 12 });
  });

  it("lets an editor lock a member with its exact version", async () => {
    useAuthStore.setState({
      user: { role: "editor" } as HotKeyAPI.UserResponse,
    });
    const user = userEvent.setup();
    render(<EventsPage />);

    await user.click(
      await screen.findByRole("button", { name: "锁定内容 31" })
    );

    expect(mocks.postEventsIdContentsContentIdLock).toHaveBeenCalledWith(
      { id: 11, content_id: 31 },
      { expected_version: 2, locked: true, reason: "manual_member_lock" }
    );
    expect(
      await screen.findByRole("button", { name: "解锁内容 31" })
    ).toBeInTheDocument();
  });

  it("binds relevance ranking to a selected monitor", async () => {
    const user = userEvent.setup();
    render(<EventsPage />);

    await screen.findByRole("heading", { name: "事件动态" });
    const sortSelect = screen.getByRole("combobox", { name: "排序方式" });
    await user.click(sortSelect);
    expect(screen.getByRole("option", { name: "监控相关性" })).toHaveAttribute(
      "data-disabled"
    );
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("combobox", { name: "监控上下文" }));
    await user.click(screen.getByRole("option", { name: "安全事故" }));

    expect(
      await screen.findByText("相关性分数等待事件命中该监控后生成。")
    ).toBeInTheDocument();
    expect(mocks.getRadarEvents).toHaveBeenLastCalledWith({
      window: "24h",
      sort: "momentum",
      limit: 50,
      monitor_id: 7,
    });

    await user.click(sortSelect);
    await user.click(screen.getByRole("option", { name: "监控相关性" }));
    await waitFor(() =>
      expect(mocks.getRadarEvents).toHaveBeenLastCalledWith({
        window: "24h",
        sort: "relevance",
        limit: 50,
        monitor_id: 7,
      })
    );
  });

  it("contains auxiliary failures without clearing the Radar list", async () => {
    mocks.getEventsIdUpdates.mockRejectedValueOnce(
      new Error("updates unavailable")
    );
    mocks.getEventsIdIntelligence.mockRejectedValueOnce(
      new Error("intelligence unavailable")
    );
    mocks.getEventsIdHeat.mockRejectedValueOnce(new Error("heat unavailable"));
    mocks.getEventsIdContents.mockRejectedValueOnce(
      new Error("members unavailable")
    );

    render(<EventsPage />);

    expect(
      await screen.findByText("华东沿海化工园区发生爆燃事故")
    ).toBeInTheDocument();
    expect(
      await screen.findByText("最新变化暂时不可用，请稍后重试。")
    ).toBeInTheDocument();
    expect(screen.getByText("事件研判暂时不可用")).toBeInTheDocument();
    expect(screen.getByText("热度暂时不可用")).toBeInTheDocument();
    expect(screen.getByText("聚类成员暂时不可用")).toBeInTheDocument();
  });

  it("uses a non-heading alert title when the Radar request fails", async () => {
    mocks.getRadarEvents.mockRejectedValueOnce(new Error("radar unavailable"));
    render(<EventsPage />);

    expect(await screen.findByText("事件雷达加载失败")).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "事件雷达加载失败" })
    ).not.toBeInTheDocument();
  });
});
