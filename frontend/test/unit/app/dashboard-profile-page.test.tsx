import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProfilePage from "@/app/dashboard/profile/page";

const mocks = vi.hoisted(() => ({
  getMonitors: vi.fn().mockResolvedValue({ data: { items: [] } }),
  getSourceConnections: vi.fn().mockResolvedValue({ data: { items: [] } }),
  getOperationsOverview: vi
    .fn()
    .mockResolvedValue({ data: { running_jobs: 2 } }),
}));

vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
}));
vi.mock("@/services/hotkey/hotkey-server/sources", () => ({
  getSourceConnections: mocks.getSourceConnections,
}));
vi.mock("@/services/hotkey/hotkey-server/operations", () => ({
  getOperationsOverview: mocks.getOperationsOverview,
}));
vi.mock("@/stores/authStore", () => ({
  useAuthStore: (
    selector: (state: { user: Record<string, unknown> }) => unknown
  ) =>
    selector({
      user: {
        role: "viewer",
        display_name: "Viewer QA",
        email: "viewer@example.test",
        status: "active",
      },
    }),
}));

describe("ProfilePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getMonitors.mockResolvedValue({ data: { items: [] } });
    mocks.getSourceConnections.mockResolvedValue({ data: { items: [] } });
    mocks.getOperationsOverview.mockResolvedValue({
      data: { running_jobs: 2 },
    });
  });

  it("shows viewer profile statistics without requesting the editor-only runtime overview", async () => {
    render(<ProfilePage />);

    expect(
      await screen.findByRole("heading", { name: "Viewer QA" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "运行任务 0" })
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看监控" })).toHaveAttribute(
      "href",
      "/dashboard/settings"
    );
    expect(
      screen.queryByRole("link", { name: "管理监控" })
    ).not.toBeInTheDocument();
    expect(mocks.getOperationsOverview).not.toHaveBeenCalled();
    expect(screen.queryByText(/报告记录|Agent Token/)).not.toBeInTheDocument();
  });

  it("shows a recoverable error instead of misleading zero statistics", async () => {
    mocks.getMonitors.mockRejectedValueOnce(new Error("profile unavailable"));
    render(<ProfilePage />);

    expect(await screen.findByText("无法加载工作区统计")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "运行任务 0" })
    ).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "重试" }));
    expect(
      await screen.findByRole("link", { name: "运行任务 0" })
    ).toBeInTheDocument();
  });
});
