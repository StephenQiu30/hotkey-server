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
      await screen.findByRole("heading", { name: /这是今日值得关注的热点/ })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "热门热点" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Claude 发布实时 API" })
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看全部热点" })).toHaveAttribute(
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
    expect(await screen.findByText("暂时没有热点")).toBeInTheDocument();
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
});
