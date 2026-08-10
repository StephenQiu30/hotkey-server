import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PushSubscriptionDeviceCard } from "@/components/notifications/PushSubscriptionDeviceCard";
import {
  deleteNotificationsPushSubscriptionsId,
  putNotificationsPushSubscriptionsId,
} from "@/services/hotkey/hotkey-server/notifications";

vi.mock("@/services/hotkey/hotkey-server/notifications", () => ({
  putNotificationsPushSubscriptionsId: vi.fn(),
  deleteNotificationsPushSubscriptionsId: vi.fn(),
}));

const subscription: HotKeyAPI.PushSubscriptionResponseDTO = {
  id: 8,
  version: 3,
  device_label: "iPhone",
  timezone: "Asia/Shanghai",
  ttl_seconds: 3600,
  status: "active",
  monitor_ids: [4],
  created_at: "2026-08-10T00:00:00Z",
  updated_at: "2026-08-10T00:00:00Z",
};

describe("PushSubscriptionDeviceCard", () => {
  beforeEach(() => vi.clearAllMocks());

  it("updates the complete allow-list with a strong version ETag", async () => {
    vi.mocked(putNotificationsPushSubscriptionsId).mockResolvedValue({
      code: 0,
      message: "ok",
      data: { ...subscription, version: 4, monitor_ids: [4, 5] },
    });
    const onUpdated = vi.fn();
    const user = userEvent.setup();
    render(
      <PushSubscriptionDeviceCard
        subscription={subscription}
        monitors={[
          { id: 4, name: "AI 芯片", status: "active" },
          { id: 5, name: "开源模型", status: "active" },
        ]}
        currentBrowser
        onUpdated={onUpdated}
        onDisabled={vi.fn()}
      />,
    );
    await user.click(screen.getByText("开源模型"));
    await user.click(screen.getByRole("button", { name: "保存设备设置" }));
    await waitFor(() => expect(putNotificationsPushSubscriptionsId).toHaveBeenCalledTimes(1));
    expect(putNotificationsPushSubscriptionsId).toHaveBeenCalledWith(
      { id: 8 },
      expect.objectContaining({ monitor_ids: [4, 5], timezone: "Asia/Shanghai" }),
      { headers: { "If-Match": '"v3"' } },
    );
    expect(onUpdated).toHaveBeenCalledWith(expect.objectContaining({ version: 4 }));
  });

  it("disables a device with the same optimistic concurrency contract", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const disabled: HotKeyAPI.PushSubscriptionResponseDTO = {
      ...subscription,
      version: 4,
      status: "disabled",
    };
    vi.mocked(deleteNotificationsPushSubscriptionsId).mockResolvedValue({
      code: 0,
      message: "ok",
      data: disabled,
    });
    const onDisabled = vi.fn(() => Promise.resolve());
    const user = userEvent.setup();
    render(
      <PushSubscriptionDeviceCard
        subscription={subscription}
        monitors={[{ id: 4, name: "AI 芯片", status: "active" }]}
        currentBrowser={false}
        onUpdated={vi.fn()}
        onDisabled={onDisabled}
      />,
    );
    await user.click(screen.getByRole("button", { name: "停用此设备" }));
    await waitFor(() => expect(deleteNotificationsPushSubscriptionsId).toHaveBeenCalledTimes(1));
    expect(deleteNotificationsPushSubscriptionsId).toHaveBeenCalledWith(
      { id: 8 },
      { headers: { "If-Match": '"v3"' } },
    );
    expect(onDisabled).toHaveBeenCalledWith(disabled);
  });
});
