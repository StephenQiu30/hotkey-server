import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import EventsPage from "@/app/dashboard/events/page";

const mocks = vi.hoisted(() => ({
  getRadarEvents: vi.fn(),
  getEventsIdUpdates: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(""),
}));
vi.mock("@/services/hotkey/hotkey-server/radar", () => ({
  getRadarEvents: mocks.getRadarEvents,
}));
vi.mock("@/services/hotkey/hotkey-server/events", () => ({
  getEventsIdUpdates: mocks.getEventsIdUpdates,
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
            momentum: 91,
            trend_status: "rising",
            confirmation: "corroborated",
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
  });

  it("filters Radar and opens a real event-change detail panel", async () => {
    render(<EventsPage />);

    expect(await screen.findByRole("heading", { name: "事件动态" })).toBeInTheDocument();
    expect(screen.getByText("华东沿海化工园区发生爆燃事故")).toBeInTheDocument();
    expect(await screen.findByText("新增 3 条独立来源")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "为什么值得关注" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "热点事件列表" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "事件" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /生成式 AI 产品/ }));

    expect(mocks.getEventsIdUpdates).toHaveBeenLastCalledWith({ id: 12, limit: 20 });
  });
});
