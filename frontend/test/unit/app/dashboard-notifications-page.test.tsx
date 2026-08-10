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
        version: 1,
        monitor_id: 2,
        event_type: "micro_event.review_requested",
        resource_type: "micro_event",
        resource_id: 8,
        resource_version: 2,
        occurred_at: "2026-08-08T00:00:00Z",
        created_at: "2026-08-08T00:00:00Z",
        title: "微事件归属需要复核",
        resource_status: "review_pending",
        deep_link: "/dashboard/events?event=8",
      }],
      lastEventID: 3,
      readThroughID: 0,
      unreadCount: 1,
      transport: "polling",
    });

    render(<NotificationsPage />);
    expect(screen.getByText("轮询中")).toBeInTheDocument();
    expect(screen.getByText("微事件归属需要复核")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看通知关联内容" })).toHaveAttribute("href", "/dashboard/events?event=8");
    expect(screen.queryByRole("heading", { name: "报告订阅" })).not.toBeInTheDocument();
    await waitFor(() => expect(useNotificationStore.getState().unreadCount).toBe(0));
    expect(useNotificationStore.getState().readThroughID).toBe(3);
  });
});
