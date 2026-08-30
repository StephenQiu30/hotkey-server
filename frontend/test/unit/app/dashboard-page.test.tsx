import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import DashboardPage from "@/app/dashboard/page";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getHotspots: vi.fn(),
  getMonitors: vi.fn(),
}));
vi.mock("@/services/hotkey/hotkey-server/hotspots", () => ({
  getHotspots: mocks.getHotspots,
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
    mocks.getHotspots.mockResolvedValue({
      data: {
        items: [
          {
            id: 7,
            source_type: "rss",
            title: "Claude 发布实时 API",
            summary: "新的实时能力。",
            canonical_url: "https://example.test/release",
            discovered_at: "2026-08-13T12:00:00Z",
            heat_score: 36.4,
            quality_state: "unavailable",
            relevance: 88,
            importance: "medium",
          },
        ],
        summary: { total: 12, today: 3, urgent: 1 },
      },
    });
    mocks.getMonitors.mockResolvedValue({
      data: { items: [{ id: 2, name: "Claude", status: "active" }] },
    });
  });

  it("renders persisted hotspot statistics and cards without the legacy event model", async () => {
    render(<DashboardPage />);
    expect(
      await screen.findByRole("heading", { name: /今日信号态势/ })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "正在升温" })
    ).toBeInTheDocument();
    expect(screen.getByText("12 条信号进入观察")).toBeInTheDocument();
    expect(screen.getByText("1 条紧急信号待复核")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Claude 发布实时 API", level: 3 })
    ).toBeInTheDocument();
    expect(screen.getByText("等待分析")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看全部信号" })).toHaveAttribute(
      "href",
      "/dashboard/contents"
    );
    expect(screen.getByRole("link", { name: "查看通知" })).toHaveAttribute(
      "href",
      "/dashboard/notifications"
    );
    expect(mocks.getHotspots).toHaveBeenCalledWith({ limit: 3, sort: "heat" });
    expect(
      screen.queryByText(/Heat v2|微事件|内容起源/)
    ).not.toBeInTheDocument();
  });

  it("shows an explicit empty state", async () => {
    mocks.getHotspots.mockResolvedValue({ data: { items: [], summary: {} } });
    render(<DashboardPage />);
    expect(await screen.findByText("还没有进入观察的信号")).toBeInTheDocument();
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
    mocks.getHotspots.mockResolvedValue({ data: { items: [], summary: {} } });
    render(<DashboardPage />);
    expect(
      await screen.findByRole("link", { name: "查看监控" })
    ).toHaveAttribute("href", "/dashboard/settings");
    expect(
      screen.queryByRole("link", { name: "创建监控" })
    ).not.toBeInTheDocument();
  });

  it("offers monitor creation to analysts", async () => {
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: { id: 7, email: "analyst@example.test", role: UserRole.Analyst },
      error: null,
    });
    mocks.getHotspots.mockResolvedValue({ data: { items: [], summary: {} } });
    render(<DashboardPage />);

    expect(await screen.findByRole("link", { name: "创建监控" })).toHaveAttribute(
      "href",
      "/dashboard/settings"
    );
  });

  it("does not report missing analysis scores as zero", async () => {
    mocks.getHotspots.mockResolvedValue({
      data: {
        items: [
          {
            id: 8,
            source_type: "rss",
            title: "等待分析的信号",
            quality_state: "unavailable",
          },
        ],
        summary: { total: 1 },
      },
    });

    render(<DashboardPage />);

    expect(
      await screen.findByRole("heading", { name: "等待分析的信号", level: 3 })
    ).toBeInTheDocument();
    expect(screen.getByLabelText("热度待分析")).toBeInTheDocument();
    expect(screen.getByLabelText("相关性待分析")).toBeInTheDocument();
  });

  it("keeps monitor failures distinct from an empty monitor list", async () => {
    mocks.getMonitors.mockRejectedValueOnce(new Error("monitor unavailable"));

    render(<DashboardPage />);

    expect(
      await screen.findAllByText("监控状态暂时不可用")
    ).not.toHaveLength(0);
    expect(screen.queryByText("0 个任务在线")).not.toBeInTheDocument();
    expect(screen.queryByText("还没有可用监控")).not.toBeInTheDocument();
  });
});
