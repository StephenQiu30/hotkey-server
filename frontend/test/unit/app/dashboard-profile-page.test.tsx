import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ProfilePage from "@/app/dashboard/profile/page";

const mocks = vi.hoisted(() => ({
  getMonitors: vi.fn().mockResolvedValue({ data: { items: [] } }),
  getReports: vi.fn().mockResolvedValue({ data: { items: [] } }),
  getSourceConnections: vi.fn().mockResolvedValue({ data: { items: [] } }),
  getOperationsOverview: vi.fn().mockResolvedValue({ data: { running_jobs: 2 } }),
}));

vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({ getMonitors: mocks.getMonitors }));
vi.mock("@/services/hotkey/hotkey-server/reports", () => ({ getReports: mocks.getReports }));
vi.mock("@/services/hotkey/hotkey-server/sources", () => ({ getSourceConnections: mocks.getSourceConnections }));
vi.mock("@/services/hotkey/hotkey-server/operations", () => ({ getOperationsOverview: mocks.getOperationsOverview }));
vi.mock("@/stores/authStore", () => ({
  useAuthStore: (selector: (state: { user: Record<string, unknown> }) => unknown) =>
    selector({ user: { role: "viewer", display_name: "Viewer QA", email: "viewer@example.test", status: "active" } }),
}));

describe("ProfilePage", () => {
  it("shows viewer profile statistics without requesting the editor-only runtime overview", async () => {
    render(<ProfilePage />);

    expect(await screen.findByRole("heading", { name: "Viewer QA" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "运行任务 0" })).toBeInTheDocument();
    expect(mocks.getOperationsOverview).not.toHaveBeenCalled();
  });
});
