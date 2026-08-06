import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import DashboardPage from "@/app/dashboard/page";

const mocks = vi.hoisted(() => ({
  getRadarEvents: vi.fn(),
  getAlerts: vi.fn(),
  getMonitors: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/radar", () => ({
  getRadarEvents: mocks.getRadarEvents,
}));
vi.mock("@/services/hotkey/hotkey-server/alerts", () => ({
  getAlerts: mocks.getAlerts,
}));
vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
}));

describe("DashboardPage", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getRadarEvents.mockResolvedValue({
      data: {
        as_of: "2026-08-04T14:30:00+08:00",
        items: [
          {
            event_id: 7,
            title_zh: "华东沿海化工园区发生爆燃事故",
            summary: "事故引发公众对化工园区风险与安全生产的持续关注。",
            momentum: 92,
            independent_source_count: 8,
            trend_status: "rising",
            confirmation: "corroborated",
            reason_codes: ["momentum_rising", "source_breadth_growing"],
            latest_update: { summary: "多家权威来源补充伤亡与救援进展" },
          },
          {
            event_id: 8,
            title_zh: "国际航线逐步恢复，暑期出行升温",
            summary: "航司运力增加，热门航线票价与预订热度回升。",
            momentum: 70,
            independent_source_count: 5,
            trend_status: "rising",
            confirmation: "corroborated",
          },
          {
            event_id: 9,
            title_zh: "生成式 AI 产品迎来新一轮功能更新",
            summary: "头部产品密集发布新功能，行业讨论度持续增长。",
            momentum: 66,
            independent_source_count: 4,
            trend_status: "stable",
            confirmation: "single_source",
          },
        ],
      },
    });
    mocks.getAlerts.mockResolvedValue({
      data: { items: [{ id: 1, state: "open" }] },
    });
    mocks.getMonitors.mockResolvedValue({
      data: {
        items: [
          { id: 2, name: "化工安全与监管", status: "active" },
          { id: 3, name: "航空出行与航空", status: "active" },
        ],
      },
    });
  });

  it("renders a radar-first overview from the new monitoring APIs", async () => {
    render(<DashboardPage />);

    expect(
      await screen.findByRole("heading", {
        name: /这是今日值得关注的变化/,
      }),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "今日 AI 摘要" })).toBeInTheDocument();
    expect(
      screen.getAllByText("华东沿海化工园区发生爆燃事故"),
    ).toHaveLength(2);
    expect(screen.getByRole("heading", { name: "重点事件" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "我的监控" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看全部事件" })).toHaveAttribute(
      "href",
      "/dashboard/events",
    );
    expect(mocks.getRadarEvents).toHaveBeenCalledWith({
      limit: 12,
      sort: "momentum",
      window: "24h",
    });
  });

  it("shows a truthful empty state when Radar has no events", async () => {
    mocks.getRadarEvents.mockResolvedValue({
      data: { as_of: "2026-08-04T14:30:00+08:00", items: [] },
    });

    render(<DashboardPage />);

    expect(await screen.findByText("当前窗口内还没有热点事件")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "创建监控" })).toHaveAttribute(
      "href",
      "/dashboard/settings",
    );
  });
});
