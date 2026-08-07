import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SubscriptionsPage from "@/app/dashboard/notifications/page";
import { useNotificationStore } from "@/stores/notificationStore";

const mocks = vi.hoisted(() => ({
  getSubscriptions: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/delivery", () => ({
  getReportSubscriptions: mocks.getSubscriptions,
  postReportSubscriptions: vi.fn(),
  patchReportSubscriptionsId: vi.fn(),
  deleteReportSubscriptionsId: vi.fn(),
  postReportSubscriptionsIdRssTokenRotate: vi.fn(),
}));

describe("SubscriptionsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useNotificationStore.getState().reset();
  });

  it("labels a subscription without monitor_id as covering all enabled monitors", async () => {
    mocks.getSubscriptions.mockResolvedValue({
      data: {
        items: [
          {
            id: 1,
            channel: "email",
            report_type: "daily",
            recipient: "reader@example.com",
            schedule: "0 9 * * *",
            enabled: true,
            version: 1,
          },
        ],
      },
    });

    render(<SubscriptionsPage />);

    expect(
      await screen.findByText("全部已启用监控 · reader@example.com"),
    ).toBeInTheDocument();
  });

  it("shows polling degradation and marks displayed notifications as read", async () => {
    mocks.getSubscriptions.mockResolvedValue({ data: { items: [] } });
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

    render(<SubscriptionsPage />);
    expect(screen.getByText("轮询中")).toBeInTheDocument();
    expect(screen.getByText("报告生成失败")).toBeInTheDocument();
    await waitFor(() => expect(useNotificationStore.getState().unreadCount).toBe(0));
    expect(useNotificationStore.getState().readThroughID).toBe(3);
  });
});
