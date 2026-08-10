import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import DashboardPage from "@/app/dashboard/page";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getMicroEvents: vi.fn(),
  getAlerts: vi.fn(),
  getMonitors: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/microEvents", () => ({
  getMicroEvents: mocks.getMicroEvents,
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
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: { id: 1, email: "admin@example.test", role: UserRole.Admin },
      error: null,
    });
    mocks.getMicroEvents.mockResolvedValue({
      data: {
        items: [
          {
            id: 7,
            primary_subject_key: "华东沿海化工园区",
            primary_action_key: "发生爆燃事故",
            status: "active",
            storyline: { summary: "事故救援与后续调查持续更新。" },
            latest_heat: {
              acceleration: 2,
              independent_lineage_root_count: 8,
              reason_codes: ["acceleration_rising"],
              window_ended_at: "2026-08-04T14:30:00+08:00",
            },
          },
          {
            id: 8,
            primary_subject_key: "国际航线",
            primary_action_key: "逐步恢复",
            status: "active",
            latest_heat: { independent_lineage_root_count: 5 },
          },
          {
            id: 9,
            primary_subject_key: "生成式 AI 产品",
            primary_action_key: "发布功能更新",
            status: "review_pending",
            latest_heat: { independent_lineage_root_count: 4 },
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

  it("renders a Heat v2 overview from semantic micro-events", async () => {
    render(<DashboardPage />);

    expect(
      await screen.findByRole("heading", {
        name: /这是今日值得关注的变化/,
      })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "今日事件摘要" })
    ).toBeInTheDocument();
    expect(screen.getAllByText("华东沿海化工园区 · 发生爆燃事故")).toHaveLength(2);
    expect(
      screen.getByRole("heading", { name: "重点事件" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "我的监控" })
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看全部事件" })).toHaveAttribute(
      "href",
      "/dashboard/events"
    );
    expect(mocks.getMicroEvents).toHaveBeenCalledWith({ limit: 12 });
    expect(screen.getByText("Heat v2")).toBeInTheDocument();
    expect(screen.queryByText(/可信|已证实|置信度|Radar 综合/)).not.toBeInTheDocument();
  });

  it("shows an explicit empty state when there are no micro-events", async () => {
    mocks.getMicroEvents.mockResolvedValue({
      data: { items: [] },
    });

    render(<DashboardPage />);

    expect(
      await screen.findByText("当前窗口内还没有热点事件")
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "创建监控" })).toHaveAttribute(
      "href",
      "/dashboard/settings"
    );
  });

  it("keeps the empty-state action read-only for viewers", async () => {
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: { id: 2, email: "viewer@example.test", role: UserRole.Viewer },
      error: null,
    });
    mocks.getMicroEvents.mockResolvedValue({
      data: { items: [] },
    });

    render(<DashboardPage />);

    expect(
      await screen.findByRole("link", { name: "查看监控" })
    ).toHaveAttribute("href", "/dashboard/settings");
    expect(
      screen.queryByRole("link", { name: "创建监控" })
    ).not.toBeInTheDocument();
  });
});
