import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NotificationsPage from "@/app/dashboard/notifications/page";
import { useNotificationStore } from "@/stores/notificationStore";

describe("NotificationsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useNotificationStore.getState().reset();
  });

  it("shows polling degradation and marks displayed notifications as read", async () => {
    useNotificationStore.setState({
      items: [{
        id: 3,
        event_type: "report.failed",
        resource_type: "report",
        resource_id: 8,
        occurred_at: "2026-08-08T00:00:00Z",
        payload: { title: "报告生成失败" },
      }],
      lastEventID: 3,
      readThroughID: 0,
      unreadCount: 1,
      transport: "polling",
    });

    render(<NotificationsPage />);
    expect(screen.getByText("轮询中")).toBeInTheDocument();
    expect(screen.getByText("报告生成失败")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看通知关联内容" })).toHaveAttribute("href", "/dashboard/reports");
    expect(screen.queryByRole("heading", { name: "报告订阅" })).not.toBeInTheDocument();
    await waitFor(() => expect(useNotificationStore.getState().unreadCount).toBe(0));
    expect(useNotificationStore.getState().readThroughID).toBe(3);
  });
});
