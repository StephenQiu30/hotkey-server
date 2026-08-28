import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NotificationsPage from "@/app/dashboard/notifications/page";
import { AuthStatus } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";
import { useNotificationStore } from "@/stores/notificationStore";

const mocks = vi.hoisted(() => ({
  getNotifications: vi.fn(),
  postNotificationsReadReceipts: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/notifications", () => ({
  getNotifications: mocks.getNotifications,
  postNotificationsReadReceipts: mocks.postNotificationsReadReceipts,
}));

describe("NotificationsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useNotificationStore.getState().reset();
    mocks.getNotifications.mockResolvedValue({
      data: { items: [], next_after_id: 0, read_through_id: 0 },
    });
    mocks.postNotificationsReadReceipts.mockResolvedValue({
      data: {
        receipt_id: 1,
        read_through_id: 3,
        advanced: true,
        recorded_at: "2026-08-08T00:01:00Z",
      },
    });
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: {
        id: 7,
        email: "viewer@example.test",
        display_name: "Viewer",
        role: "viewer",
        status: "active",
      },
      error: null,
    });
  });

  it("shows polling degradation and marks displayed notifications as read", async () => {
    useNotificationStore.setState({
      items: [{
        id: 3,
        version: 1,
        monitor_id: 2,
        event_type: "hotspot.discovered",
        resource_type: "hotspot",
        resource_id: 8,
        resource_version: 2,
        occurred_at: "2026-08-08T00:00:00Z",
        created_at: "2026-08-08T00:00:00Z",
        title: "新热点已发现",
        resource_status: "high",
        deep_link: "/dashboard/contents/8",
      }],
      lastEventID: 3,
      readThroughID: 0,
      unreadCount: 1,
      transport: "polling",
    });

    render(<NotificationsPage />);
    expect(screen.getByText("轮询中")).toBeInTheDocument();
    expect(screen.getByText("新热点已发现")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看通知关联内容" })).toHaveAttribute("href", "/dashboard/contents/8");
    expect(screen.getByRole("heading", { name: "站内实时通知" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "邮件通知" })).toBeInTheDocument();
    expect(screen.getByText("viewer@example.test")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "管理邮件提醒" })).toHaveAttribute("href", "/dashboard/settings");
    expect(screen.queryByText(/Web Push|手机与浏览器通知/)).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "报告订阅" })).not.toBeInTheDocument();
    await waitFor(() =>
      expect(mocks.postNotificationsReadReceipts).toHaveBeenCalledWith({ read_through_id: 3 }),
    );
    await waitFor(() => expect(useNotificationStore.getState().unreadCount).toBe(0));
    expect(useNotificationStore.getState().readThroughID).toBe(3);
  });
});
