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

const mocks = vi.hoisted(() => ({
  getRadarEvents: vi.fn(),
  getEventsIdUpdates: vi.fn(),
  getEventsIdIntelligence: vi.fn(),
  getMonitors: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(""),
}));
vi.mock("@/services/hotkey/hotkey-server/radar", () => ({
  getRadarEvents: mocks.getRadarEvents,
}));
vi.mock("@/services/hotkey/hotkey-server/events", () => ({
  getEventsIdUpdates: mocks.getEventsIdUpdates,
  getEventsIdIntelligence: mocks.getEventsIdIntelligence,
}));
vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
}));

describe("EventsPage", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getRadarEvents.mockResolvedValue({
      data: {
        as_of: "2026-08-04T14:30:00+08:00",
        items: [
          {
            event_id: 11,
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
    mocks.getMonitors.mockResolvedValue({
      data: {
        items: [
          { id: 7, name: "安全事故", status: "active" },
          { id: 8, name: "已归档监控", status: "archived" },
        ],
      },
    });
  });

  it("filters Radar and opens a real event-change detail panel", async () => {
    render(<EventsPage />);

    expect(await screen.findByRole("heading", { name: "事件动态" })).toBeInTheDocument();
    expect(screen.getByText("华东沿海化工园区发生爆燃事故")).toBeInTheDocument();
    expect(await screen.findByText("新增 3 条独立来源")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "为什么值得关注" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "热点事件列表" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "事件" })).toBeInTheDocument();
    expect(screen.getByText("证据确认，不等于真假裁决")).toBeInTheDocument();
    expect(screen.getByText("重要性")).toBeInTheDocument();
    expect(screen.getByText("园区已启动应急响应")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /查看证据内容 31/ })).toHaveAttribute("href", "/dashboard/contents/31");
    expect(screen.getByText("46 · 低置信度")).toBeInTheDocument();
    expect(mocks.getRadarEvents).toHaveBeenCalledWith({ window: "24h", sort: "momentum", limit: 50 });
    expect(mocks.getEventsIdIntelligence).toHaveBeenCalledWith({ id: 11 });

    fireEvent.click(screen.getByRole("button", { name: /生成式 AI 产品/ }));

    expect(mocks.getEventsIdUpdates).toHaveBeenLastCalledWith({ id: 12, limit: 20 });
    expect(mocks.getEventsIdIntelligence).toHaveBeenLastCalledWith({ id: 12 });
  });

  it("binds relevance ranking to a selected monitor", async () => {
    const user = userEvent.setup();
    render(<EventsPage />);

    await screen.findByRole("heading", { name: "事件动态" });
    const sortSelect = screen.getByRole("combobox", { name: "排序方式" });
    await user.click(sortSelect);
    expect(screen.getByRole("option", { name: "监控相关性" })).toHaveAttribute("data-disabled");
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("combobox", { name: "监控上下文" }));
    await user.click(screen.getByRole("option", { name: "安全事故" }));

    expect(await screen.findByText("相关性分数等待事件命中该监控后生成。")).toBeInTheDocument();
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
      }),
    );
  });

  it("contains auxiliary failures without clearing the Radar list", async () => {
    mocks.getEventsIdUpdates.mockRejectedValueOnce(new Error("updates unavailable"));
    mocks.getEventsIdIntelligence.mockRejectedValueOnce(
      new Error("intelligence unavailable"),
    );

    render(<EventsPage />);

    expect(
      await screen.findByText("华东沿海化工园区发生爆燃事故"),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("最新变化暂时不可用，请稍后重试。"),
    ).toBeInTheDocument();
    expect(screen.getByText("事件研判暂时不可用")).toBeInTheDocument();
  });
});
