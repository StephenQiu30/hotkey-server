import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SubscriptionsPage from "@/app/dashboard/notifications/page";

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
});
