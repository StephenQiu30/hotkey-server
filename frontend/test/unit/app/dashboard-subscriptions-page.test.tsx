import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SubscriptionsPage from "@/app/dashboard/subscriptions/page";

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
    expect(screen.queryByRole("heading", { name: "站内通知" })).not.toBeInTheDocument();
  });

  it("keeps subscription failures distinct from the empty state and retries", async () => {
    mocks.getSubscriptions
      .mockRejectedValueOnce(new Error("subscription unavailable"))
      .mockResolvedValueOnce({ data: { items: [] } });

    render(<SubscriptionsPage />);
    expect(await screen.findByText("无法加载报告订阅")).toBeInTheDocument();
    expect(screen.queryByText("暂时没有发布订阅")).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "重试订阅" }));
    expect(await screen.findByText("暂时没有发布订阅")).toBeInTheDocument();
  });
});
